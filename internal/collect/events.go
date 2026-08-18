package collect

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// eventLimit bounds how many events are fetched for one object. Busy
// namespaces can hold thousands; a diagnosis never needs more than the recent
// ones for a single object.
const eventLimit = 200

// eventsFor fetches the events recorded against a single object.
//
// The field selector keeps the work on the API server, and the results are
// filtered again client-side so a server that ignores the selector cannot
// leak unrelated events into a diagnosis.
func eventsFor(ctx context.Context, c *kube.Client, ref diagnosis.ResourceRef, uid types.UID) (snapshot.Events, error) {
	selector := fields.Set{
		"involvedObject.name": ref.Name,
		"involvedObject.kind": ref.Kind,
	}
	if uid != "" {
		selector["involvedObject.uid"] = string(uid)
	}

	list, err := c.Clientset.CoreV1().Events(ref.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: selector.AsSelector().String(),
		Limit:         eventLimit,
	})
	if err != nil {
		return nil, err
	}

	out := make(snapshot.Events, 0, len(list.Items))
	for i := range list.Items {
		ev := &list.Items[i]
		if !matchesObject(ev, ref, uid) {
			continue
		}
		out = append(out, normalizeEvent(ev))
	}
	return out.Sort(), nil
}

func matchesObject(ev *corev1.Event, ref diagnosis.ResourceRef, uid types.UID) bool {
	if uid != "" && ev.InvolvedObject.UID != "" {
		return ev.InvolvedObject.UID == uid
	}
	return strings.EqualFold(ev.InvolvedObject.Kind, ref.Kind) && ev.InvolvedObject.Name == ref.Name
}

// normalizeEvent flattens the several timestamp fields Kubernetes uses across
// event API versions into one shape.
func normalizeEvent(ev *corev1.Event) snapshot.Event {
	out := snapshot.Event{
		Type:      ev.Type,
		Reason:    ev.Reason,
		Message:   strings.TrimSpace(ev.Message),
		Count:     ev.Count,
		FirstSeen: ev.FirstTimestamp.Time,
		LastSeen:  ev.LastTimestamp.Time,
		FieldPath: ev.InvolvedObject.FieldPath,
		Object: diagnosis.ResourceRef{
			Kind:      ev.InvolvedObject.Kind,
			Namespace: ev.InvolvedObject.Namespace,
			Name:      ev.InvolvedObject.Name,
		},
	}
	if out.Count == 0 {
		out.Count = 1
	}
	// Events emitted through the events.k8s.io API only set eventTime, and
	// series events carry their last occurrence in the series.
	if out.FirstSeen.IsZero() {
		out.FirstSeen = ev.EventTime.Time
	}
	if ev.Series != nil {
		out.Count = ev.Series.Count
		out.LastSeen = ev.Series.LastObservedTime.Time
	}
	if out.LastSeen.IsZero() {
		out.LastSeen = out.FirstSeen
	}
	return out
}
