package collect

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// legacyStorageClassAnnotation is how the class was requested before the
// spec field existed. Clusters upgraded over many releases still carry it.
const legacyStorageClassAnnotation = "volume.beta.kubernetes.io/storage-class"

// resolveStorageClass fills in the class a claim asks for and its binding
// mode. A claim that names no class uses the cluster's default, which is
// resolved by listing classes only when necessary.
func resolveStorageClass(ctx context.Context, c *kube.Client, coll *snapshot.Collection, ref *snapshot.PVCRef) {
	// An explicitly empty class means the claim wants no dynamic
	// provisioning and expects a pre-created volume; there is nothing to look
	// up, and reporting a missing class would be wrong.
	if ref.Claim != nil && ref.Claim.Spec.StorageClassName != nil && *ref.Claim.Spec.StorageClassName == "" {
		ref.BindingMode = string(storagev1.VolumeBindingImmediate)
		return
	}
	name := storageClassName(ref.Claim)
	if name == "" {
		resolveDefaultStorageClass(ctx, c, coll, ref)
		return
	}
	ref.StorageClass = name

	class, err := c.Clientset.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	ref.ClassExists = existence(coll, "storageclasses", "checking the requested storage class exists", err)
	if ref.ClassExists == snapshot.Found {
		ref.BindingMode = bindingMode(class)
		coll.Inspect("storageclass " + name)
	}
}

func resolveDefaultStorageClass(ctx context.Context, c *kube.Client, coll *snapshot.Collection, ref *snapshot.PVCRef) {
	list, err := c.Clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		degrade(coll, "storageclasses", "identifying the default storage class", err)
		return
	}
	coll.Inspect("storageclasses")
	for i := range list.Items {
		class := &list.Items[i]
		if class.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			ref.StorageClass = class.Name
			ref.ClassExists = snapshot.Found
			ref.BindingMode = bindingMode(class)
			return
		}
	}
	// No default class exists. That is a fact a claim diagnosis can use.
	ref.ClassExists = snapshot.Missing
}

func bindingMode(class *storagev1.StorageClass) string {
	if class.VolumeBindingMode == nil {
		return string(storagev1.VolumeBindingImmediate)
	}
	return string(*class.VolumeBindingMode)
}

// storageClassName returns the class a claim requests, honouring the legacy
// annotation. An empty string means the claim relies on the default class.
func storageClassName(claim *corev1.PersistentVolumeClaim) string {
	if claim == nil {
		return ""
	}
	if claim.Spec.StorageClassName != nil && *claim.Spec.StorageClassName != "" {
		return *claim.Spec.StorageClassName
	}
	return claim.Annotations[legacyStorageClassAnnotation]
}
