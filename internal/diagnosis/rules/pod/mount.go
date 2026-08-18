package pod

import (
	"context"
	"fmt"
	"strings"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func mountRule() diagnosis.Rule[*snapshot.Pod] {
	return diagnosis.RuleFunc[*snapshot.Pod]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDFailedMount,
			Title: "A volume could not be mounted",
			Description: "Reports FailedMount and FailedAttachVolume events and links them to the missing " +
				"object when KubeWhy could confirm one.",
		},
		Fn: evaluateMounts,
	}
}

func evaluateMounts(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
	if snap.IsTerminal() {
		return nil
	}
	ev, ok := snap.Events.Latest("FailedMount", "FailedAttachVolume")
	if !ok {
		return nil
	}
	// A mount failure blocks a container from starting. Once every container
	// has started, the mounts plainly succeeded, and the event is history
	// that would otherwise be reported as a current fault for as long as
	// Kubernetes keeps it.
	if !anyContainerWaiting(snap) {
		return nil
	}

	d := diagnosis.Diagnosis{
		ID:         IDFailedMount,
		Subject:    snap.Ref(),
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    "The kubelet could not mount a volume the Pod needs",
		Explanation: "A container cannot start until every volume it mounts is available. The message " +
			"below is what the kubelet reported.",
		Evidence: []diagnosis.Evidence{eventEvidence(ev)},
		Suggestions: []diagnosis.Suggestion{{
			Description: "Check that every object the Pod's volumes reference exists and is usable from this node.",
		}},
	}

	if volume := quotedAfter(ev.Message, "volume"); volume != "" {
		d.Evidence = append(d.Evidence, diagnosis.Evidence{
			Source: "event",
			Field:  "volume",
			Value:  volume,
		})
	}

	// Prefer a cause KubeWhy confirmed against the API over the message text.
	switch {
	case len(missingNames(snap.ConfigMaps)) > 0:
		d.CausedBy = IDMissingConfigMap
	case len(missingNames(snap.Secrets)) > 0:
		d.CausedBy = IDMissingSecret
	default:
		for _, claim := range snap.PVCs {
			if claim.Exists == snapshot.Missing {
				d.CausedBy = IDPVCNotFound
			}
		}
	}

	lower := strings.ToLower(ev.Message)
	switch {
	case strings.Contains(lower, "timed out waiting for the condition"), strings.Contains(lower, "timeout expired"):
		d.PossibleCauses = []string{
			"the volume is still attached to another node and has not been released",
			"the storage backend did not answer the attach or mount request in time",
		}
	case strings.Contains(lower, "not found"):
		d.PossibleCauses = []string{
			fmt.Sprintf("an object the volume references does not exist in namespace %q", snap.Pod.Namespace),
		}
	}
	return []diagnosis.Diagnosis{d}
}

// anyContainerWaiting reports whether some container has not started yet.
func anyContainerWaiting(snap *snapshot.Pod) bool {
	for _, container := range snap.Containers() {
		if container.Waiting() != nil {
			return true
		}
	}
	return false
}

// quotedAfter returns the quoted token that follows a keyword in a kubelet
// message, e.g. the volume name in `failed for volume "config"`.
func quotedAfter(message, keyword string) string {
	idx := strings.Index(strings.ToLower(message), strings.ToLower(keyword))
	if idx < 0 {
		return ""
	}
	rest := message[idx+len(keyword):]
	match := quotedNameRe.FindStringSubmatch(rest)
	if match == nil {
		return ""
	}
	return match[1]
}
