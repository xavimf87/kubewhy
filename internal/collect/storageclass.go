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

// storageClassInfo resolves the class a claim relies on and how it binds.
//
// A claim that names no class uses the cluster's default, which costs a list
// request; that is only done when the answer can change a diagnosis, meaning
// when the claim has not bound.
func storageClassInfo(ctx context.Context, c *kube.Client, coll *snapshot.Collection, claim *corev1.PersistentVolumeClaim) snapshot.StorageClassInfo {
	// An explicitly empty class means the claim wants no dynamic
	// provisioning and expects a pre-created volume; there is nothing to look
	// up, and reporting a missing class would be wrong.
	if claim != nil && claim.Spec.StorageClassName != nil && *claim.Spec.StorageClassName == "" {
		return snapshot.StorageClassInfo{
			ExplicitlyNone: true,
			BindingMode:    string(storagev1.VolumeBindingImmediate),
		}
	}

	name := storageClassName(claim)
	if name == "" {
		return defaultStorageClassInfo(ctx, c, coll)
	}

	info := snapshot.StorageClassInfo{Name: name, Requested: true}
	class, err := c.Clientset.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	info.Exists = existence(coll, "storageclasses", "checking the requested storage class exists", err)
	if info.Exists == snapshot.Found {
		info.BindingMode = bindingMode(class)
		info.Provisioner = class.Provisioner
		coll.Inspect("storageclass " + name)
	}
	return info
}

func defaultStorageClassInfo(ctx context.Context, c *kube.Client, coll *snapshot.Collection) snapshot.StorageClassInfo {
	info := snapshot.StorageClassInfo{}
	list, err := c.Clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		degrade(coll, "storageclasses", "identifying the default storage class", err)
		return info
	}
	coll.Inspect("storageclasses")
	for i := range list.Items {
		class := &list.Items[i]
		if class.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			info.Name = class.Name
			info.Exists = snapshot.Found
			info.DefaultExists = snapshot.Found
			info.BindingMode = bindingMode(class)
			info.Provisioner = class.Provisioner
			return info
		}
	}
	// No default class exists. That is a fact a claim diagnosis can use.
	info.Exists = snapshot.Missing
	info.DefaultExists = snapshot.Missing
	return info
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
