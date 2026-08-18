package collect

import (
	"errors"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/xavimf87/kubewhy/internal/kubetest"
)

// appsReplicaSet builds a ReplicaSet owned by a Deployment.
type appsReplicaSet struct {
	name  string
	owner string
}

func (r appsReplicaSet) object() runtime.Object {
	controller := true
	return &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name:      r.name,
		Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{
			Kind: "Deployment", Name: r.owner, Controller: &controller,
		}},
	}}
}

func event(pod *corev1.Pod, eventType, reason, message string) runtime.Object {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: pod.Name + "." + reason, Namespace: pod.Namespace},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace, UID: pod.UID,
		},
		Type:           eventType,
		Reason:         reason,
		Message:        message,
		Count:          1,
		FirstTimestamp: metav1.NewTime(kubetest.Now.Add(-10 * time.Minute)),
		LastTimestamp:  metav1.NewTime(kubetest.Now.Add(-time.Minute)),
	}
}

func otherPod() *corev1.Pod { return kubetest.Pod("other").Build() }

func storageClass(name, bindingMode string) runtime.Object {
	mode := storagev1.VolumeBindingMode(bindingMode)
	return &storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: name},
		VolumeBindingMode: &mode,
	}
}

func ptr(s string) *string { return &s }

func asError(err error, target any) bool { return errors.As(err, target) }

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }
