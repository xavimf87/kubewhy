package pod

import (
	"context"
	"fmt"
	"strings"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// imageWaitReasons are the waiting reasons the kubelet uses when it cannot
// obtain a container image.
var imageWaitReasons = map[string]bool{
	"ImagePullBackOff":    true,
	"ErrImagePull":        true,
	"InvalidImageName":    true,
	"ImageInspectError":   true,
	"ErrImageNeverPull":   true,
	"RegistryUnavailable": true,
}

func imagePullRule() diagnosis.Rule[*snapshot.Pod] {
	return diagnosis.RuleFunc[*snapshot.Pod]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDImagePullFailed,
			Title: "Container image could not be pulled",
			Description: "Reports containers waiting on an image pull and classifies the failure from the " +
				"registry message the kubelet recorded, without asserting a cause the message does not state.",
		},
		Fn: evaluateImagePull,
	}
}

func evaluateImagePull(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
	var out []diagnosis.Diagnosis

	for _, container := range snap.Containers() {
		waiting := container.Waiting()
		if waiting == nil || !imageWaitReasons[waiting.Reason] {
			continue
		}

		image := ""
		if container.Spec != nil {
			image = container.Spec.Image
		}
		// The waiting message is often truncated; the pull event carries the
		// registry's own words.
		detail := waiting.Message
		pullEvents := snap.Events.WithReason("Failed", "BackOff")
		for _, ev := range pullEvents {
			if ev.Container() != "" && ev.Container() != container.Name {
				continue
			}
			if len(ev.Message) > len(detail) {
				detail = ev.Message
			}
		}

		cause := classifyImageFailure(waiting.Reason, detail)
		d := diagnosis.Diagnosis{
			ID:         IDImagePullFailed,
			Subject:    snap.Ref(),
			Component:  container.Name,
			Severity:   diagnosis.SeverityCritical,
			Confidence: cause.confidence,
			Summary:    fmt.Sprintf("%s %q cannot start because its image could not be pulled", capitalize(container.Kind()), container.Name),
			Explanation: fmt.Sprintf("Kubernetes reports %s for image %q. %s",
				waiting.Reason, image, cause.explanation),
			PossibleCauses: cause.causes,
			Evidence: []diagnosis.Evidence{{
				Source: "podSpec",
				Field:  "image",
				Value:  image,
			}},
			Suggestions: cause.suggestions(snap, container.Name, image),
		}
		d.Evidence = append(d.Evidence, containerEvidence(container)...)
		if detail != "" && detail != waiting.Message {
			d.Evidence = append(d.Evidence, diagnosis.Evidence{
				Source:  "event",
				Field:   "reason",
				Value:   "Failed",
				Message: truncate(detail, 300),
			})
		}
		if cause.classification != "" {
			d.Evidence = append(d.Evidence, diagnosis.Evidence{
				Source: "kubewhy",
				Field:  "classification",
				Value:  cause.classification,
			})
		}
		out = append(out, d)
	}
	return out
}

// imageCause is the interpretation of a registry error message.
type imageCause struct {
	classification string
	explanation    string
	confidence     diagnosis.Confidence
	causes         []string
	commands       []string
	action         string
}

func (c imageCause) suggestions(snap *snapshot.Pod, container, image string) []diagnosis.Suggestion {
	action := c.action
	if action == "" {
		action = "Check that the image reference is correct and that the node can reach the registry."
	}
	commands := c.commands
	if len(commands) == 0 {
		commands = []string{fmt.Sprintf("kubectl describe pod %s -n %s", snap.Pod.Name, snap.Pod.Namespace)}
	}
	return []diagnosis.Suggestion{{Description: action, Commands: commands}}
}

// imageMatchers maps registry wording to a classification. The kubelet passes
// the registry's message through, so matching on it is the only evidence
// available for telling these failures apart.
var imageMatchers = []struct {
	needles        []string
	classification string
	explanation    string
	causes         []string
	action         string
}{
	{
		needles:        []string{"manifest unknown", "manifest for", "not found", "does not exist", "repository does not exist"},
		classification: "image or tag not found",
		explanation:    "The registry answered that the image or tag does not exist.",
		causes: []string{
			"the tag was never pushed, or was deleted from the registry",
			"the image name or tag contains a typo",
		},
		action: "Verify the image name and tag against the registry.",
	},
	{
		needles:        []string{"unauthorized", "authentication required", "pull access denied", "no basic auth credentials", "denied:", "forbidden"},
		classification: "registry rejected the credentials",
		explanation:    "The registry refused the request because it was not authenticated or not authorised for this repository.",
		causes: []string{
			"the Pod has no imagePullSecret for this registry",
			"the imagePullSecret exists but does not grant access to this repository",
			"the image is private and the node's credentials do not cover it",
		},
		action: "Check the Pod's imagePullSecrets and the permissions they grant. KubeWhy does not read their contents.",
	},
	{
		needles:        []string{"toomanyrequests", "rate limit", "too many requests"},
		classification: "registry rate limit",
		explanation:    "The registry rejected the pull because a rate limit was reached.",
		causes: []string{
			"the registry limits anonymous or per-account pulls and the quota is exhausted",
		},
		action: "Wait for the limit to reset, authenticate the pulls, or mirror the image.",
	},
	{
		needles:        []string{"no such host", "i/o timeout", "connection refused", "dial tcp", "network is unreachable", "server gave http response to https client", "x509", "certificate"},
		classification: "registry unreachable from the node",
		explanation:    "The node could not complete a connection to the registry.",
		causes: []string{
			"the node cannot resolve or reach the registry host",
			"a proxy, firewall or egress policy blocks the connection",
			"the registry uses a certificate the node does not trust",
		},
		action: "Check the node's network path to the registry, including DNS, egress rules and trusted certificates.",
	},
}

func classifyImageFailure(reason, message string) imageCause {
	if reason == "InvalidImageName" {
		return imageCause{
			classification: "invalid image reference",
			explanation:    "The image reference is not a valid name, so no pull was attempted.",
			confidence:     diagnosis.ConfidenceCertain,
			causes:         []string{"the image field contains a malformed reference"},
			action:         "Correct the image reference in the Pod template.",
		}
	}
	if reason == "ErrImageNeverPull" {
		return imageCause{
			classification: "image absent and pull policy Never",
			explanation:    "The image is not present on the node and imagePullPolicy is Never, so the kubelet did not try to pull it.",
			confidence:     diagnosis.ConfidenceCertain,
			causes:         []string{"the image was expected to be preloaded on the node but is not there"},
			action:         "Preload the image on the node or change imagePullPolicy.",
		}
	}

	lower := strings.ToLower(message)
	for _, matcher := range imageMatchers {
		for _, needle := range matcher.needles {
			if strings.Contains(lower, needle) {
				return imageCause{
					classification: matcher.classification,
					explanation:    matcher.explanation,
					confidence:     diagnosis.ConfidenceLikely,
					causes:         matcher.causes,
					action:         matcher.action,
				}
			}
		}
	}

	return imageCause{
		explanation: "The message the registry returned does not identify a more specific cause.",
		confidence:  diagnosis.ConfidenceCertain,
		causes: []string{
			"the image or tag does not exist",
			"the registry requires credentials the Pod does not have",
			"the node cannot reach the registry",
		},
	}
}
