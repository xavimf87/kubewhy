package pod

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func nodeRule() diagnosis.Rule[*snapshot.Pod] {
	return diagnosis.RuleFunc[*snapshot.Pod]{
		Metadata: diagnosis.RuleMeta{
			ID:    IDNodeNotReady,
			Title: "The Pod's node is not ready",
			Description: "Reports that the node a Pod is bound to does not report Ready, which explains a " +
				"Pod whose status has stopped changing.",
		},
		Fn: evaluateNode,
	}
}

func evaluateNode(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
	if snap.Node == nil || snap.IsTerminal() {
		return nil
	}
	var ready *corev1.NodeCondition
	for i := range snap.Node.Status.Conditions {
		if snap.Node.Status.Conditions[i].Type == corev1.NodeReady {
			ready = &snap.Node.Status.Conditions[i]
			break
		}
	}
	if ready == nil || ready.Status == corev1.ConditionTrue {
		return nil
	}

	d := diagnosis.Diagnosis{
		ID:         IDNodeNotReady,
		Subject:    diagnosis.ResourceRef{Kind: "Node", Name: snap.Node.Name},
		Severity:   diagnosis.SeverityCritical,
		Confidence: diagnosis.ConfidenceCertain,
		Summary:    fmt.Sprintf("Node %q is not Ready, and this Pod is bound to it", snap.Node.Name),
		Explanation: "When a node stops reporting Ready, the kubelet on it is no longer updating the Pods " +
			"it runs. Their status then reflects the last thing the control plane heard, not what is " +
			"happening now.",
		Evidence: []diagnosis.Evidence{
			{Source: "node", Field: "conditions.Ready", Value: string(ready.Status)},
			{Source: "node", Field: "conditions.Ready.reason", Value: ready.Reason, Message: truncate(ready.Message, 200)},
		},
		PossibleCauses: []string{
			"the kubelet on that node stopped reporting to the control plane",
			"the node lost network connectivity or was shut down",
		},
		Suggestions: []diagnosis.Suggestion{{
			Description: "Look at the node itself; a Pod on an unready node cannot be diagnosed further from the Pod's own status.",
			Commands:    []string{fmt.Sprintf("kubectl describe node %s", snap.Node.Name)},
		}},
	}
	return []diagnosis.Diagnosis{d}
}
