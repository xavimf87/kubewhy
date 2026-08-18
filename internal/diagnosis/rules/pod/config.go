package pod

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func configRefRule() diagnosis.Rule[*snapshot.Pod] {
	return diagnosis.RuleFunc[*snapshot.Pod]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDMissingConfigMap,
			Title: "Referenced ConfigMap or Secret does not exist",
			Description: "Reports ConfigMaps and Secrets a Pod requires that the API server says do not " +
				"exist, and the container configuration errors they cause. Secret existence is checked " +
				"through a metadata-only request, so Secret contents are never read.",
			Emits: []string{IDMissingConfigMap, IDMissingSecret, IDCreateContainerConfigErr},
		},
		Fn: evaluateConfigRefs,
	}
}

func evaluateConfigRefs(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
	var out []diagnosis.Diagnosis
	missingConfigMaps := missingNames(snap.ConfigMaps)
	missingSecrets := missingNames(snap.Secrets)

	for _, name := range missingConfigMaps {
		out = append(out, missingObjectDiagnosis(snap, IDMissingConfigMap, "ConfigMap", name))
	}
	for _, name := range missingSecrets {
		out = append(out, missingObjectDiagnosis(snap, IDMissingSecret, "Secret", name))
	}

	for _, container := range snap.Containers() {
		waiting := container.Waiting()
		if waiting == nil || (waiting.Reason != "CreateContainerConfigError" && waiting.Reason != "CreateContainerError") {
			continue
		}
		d := diagnosis.Diagnosis{
			ID:         IDCreateContainerConfigErr,
			Subject:    snap.Ref(),
			Component:  container.Name,
			Severity:   diagnosis.SeverityCritical,
			Confidence: diagnosis.ConfidenceCertain,
			Summary:    fmt.Sprintf("%s %q cannot be created from its configuration", capitalize(container.Kind()), container.Name),
			Explanation: "The kubelet could not build the container because part of the configuration it " +
				"references is not usable. The message below is the kubelet's own.",
			Evidence: containerEvidence(container),
			Suggestions: []diagnosis.Suggestion{{
				Description: "Check the ConfigMaps, Secrets and keys the container references against what exists in the namespace.",
				Commands: []string{
					fmt.Sprintf("kubectl get configmap,secret -n %s", snap.Pod.Namespace),
				},
			}},
		}
		// When the missing object is known, the container error is a
		// consequence of it rather than a separate problem.
		switch {
		case referencesAny(waiting.Message, missingConfigMaps):
			d.CausedBy = IDMissingConfigMap
		case referencesAny(waiting.Message, missingSecrets):
			d.CausedBy = IDMissingSecret
		case len(missingConfigMaps) > 0:
			d.CausedBy = IDMissingConfigMap
		case len(missingSecrets) > 0:
			d.CausedBy = IDMissingSecret
		}
		out = append(out, d)
	}
	return out
}

func missingObjectDiagnosis(snap *snapshot.Pod, id, kind, name string) diagnosis.Diagnosis {
	return diagnosis.Diagnosis{
		ID:         id,
		Subject:    snap.Ref(),
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    fmt.Sprintf("%s %q does not exist but the Pod requires it", kind, name),
		Explanation: fmt.Sprintf(
			"The Pod references %s %q without marking it optional, so no container can start until it "+
				"exists. The API server reports that it is not present in namespace %q.",
			kind, name, snap.Pod.Namespace),
		Evidence: []diagnosis.Evidence{{
			Source: "api",
			Field:  strings.ToLower(kind) + "/" + name,
			Value:  "NotFound",
		}},
		Suggestions: []diagnosis.Suggestion{{
			Description: fmt.Sprintf("Create the %s, or fix the reference in the Pod template if the name changed.", kind),
			Commands:    []string{fmt.Sprintf("kubectl get %s -n %s", strings.ToLower(kind), snap.Pod.Namespace)},
		}},
	}
}

// missingNames returns the names the API server confirmed do not exist.
// Names whose state is Unknown are excluded: a denied read is not an absence.
func missingNames(refs map[string]snapshot.Existence) []string {
	var out []string
	for name, state := range refs {
		if state == snapshot.Missing {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

var quotedNameRe = regexp.MustCompile(`"([^"]+)"`)

// referencesAny reports whether a kubelet message quotes one of the names.
func referencesAny(message string, names []string) bool {
	for _, match := range quotedNameRe.FindAllStringSubmatch(message, -1) {
		for _, name := range names {
			if match[1] == name {
				return true
			}
		}
	}
	return false
}
