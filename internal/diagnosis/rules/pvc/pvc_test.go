package pvc

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func evaluate(snap *snapshot.PVC) []diagnosis.Diagnosis {
	return diagnosis.Evaluate(context.Background(), Rules(), snap)
}

func ids(ds []diagnosis.Diagnosis) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.ID)
	}
	return out
}

func find(ds []diagnosis.Diagnosis, id string) (diagnosis.Diagnosis, bool) {
	for _, d := range ds {
		if d.ID == id {
			return d, true
		}
	}
	return diagnosis.Diagnosis{}, false
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestClaimRules(t *testing.T) {
	tests := []struct {
		name    string
		snap    func() *snapshot.PVC
		want    []string
		notWant []string
	}{
		{
			name: "a bound claim is working",
			snap: func() *snapshot.PVC {
				return kubetest.PVCSnap(kubetest.Claim("data").StorageClass("fast").Bound("pv-1").Build())
			},
			notWant: []string{IDStorageClassNotFound, IDProvisioningFailed, IDWaitingForConsumer, IDNoConsumer},
		},
		{
			name: "the requested storage class does not exist",
			snap: func() *snapshot.PVC {
				snap := kubetest.PVCSnap(kubetest.Claim("data").StorageClass("premium-ssd").Build())
				snap.Class = kubetest.ClassInfo("premium-ssd", "", true, snapshot.Missing)
				return snap
			},
			want: []string{IDStorageClassNotFound},
		},
		{
			name: "no class named and the cluster has no default",
			snap: func() *snapshot.PVC {
				snap := kubetest.PVCSnap(kubetest.Claim("data").Build())
				snap.Class = snapshot.StorageClassInfo{Exists: snapshot.Missing, DefaultExists: snapshot.Missing}
				return snap
			},
			want:    []string{IDNoDefaultClass},
			notWant: []string{IDStorageClassNotFound},
		},
		{
			name: "provisioning failed",
			snap: func() *snapshot.PVC {
				snap := kubetest.PVCSnap(kubetest.Claim("data").StorageClass("fast").Build())
				snap.Class = kubetest.ClassInfo("fast", "Immediate", true, snapshot.Found)
				snap.Events = snapshot.Events{kubetest.Event("Warning", "ProvisioningFailed",
					"failed to provision volume with StorageClass \"fast\": rpc error: quota exceeded")}
				return snap
			},
			want: []string{IDProvisioningFailed},
		},
		{
			name: "no volume matches a class-less claim",
			snap: func() *snapshot.PVC {
				snap := kubetest.PVCSnap(kubetest.Claim("data").NoStorageClass().Build())
				snap.Class = snapshot.StorageClassInfo{ExplicitlyNone: true, BindingMode: "Immediate"}
				snap.Events = snapshot.Events{kubetest.Event("Normal", "FailedBinding",
					"no persistent volumes available for this claim and no storage class is set")}
				return snap
			},
			want:    []string{IDNoMatchingVolume},
			notWant: []string{IDNoDefaultClass, IDStorageClassNotFound},
		},
		{
			name: "waiting for a consumer that exists is expected",
			snap: func() *snapshot.PVC {
				snap := kubetest.PVCSnap(kubetest.Claim("data").StorageClass("standard").Build())
				snap.Class = kubetest.ClassInfo("standard", "WaitForFirstConsumer", true, snapshot.Found)
				snap.Consumers = []*snapshot.Pod{kubetest.Snap(kubetest.Pod("db-0").
					Phase(corev1.PodPending).Container(kubetest.Container("db").NoStatus()).Build())}
				return snap
			},
			want:    []string{IDWaitingForConsumer},
			notWant: []string{IDNoConsumer},
		},
		{
			name: "waiting for a consumer that will never come",
			snap: func() *snapshot.PVC {
				snap := kubetest.PVCSnap(kubetest.Claim("data").StorageClass("standard").Build())
				snap.Class = kubetest.ClassInfo("standard", "WaitForFirstConsumer", true, snapshot.Found)
				return snap
			},
			want:    []string{IDNoConsumer},
			notWant: []string{IDWaitingForConsumer},
		},
		{
			name: "a lost claim",
			snap: func() *snapshot.PVC {
				snap := kubetest.PVCSnap(kubetest.Claim("data").StorageClass("fast").
					Phase(corev1.ClaimLost).Build())
				snap.Claim.Spec.VolumeName = "pv-gone"
				snap.Class = kubetest.ClassInfo("fast", "Immediate", true, snapshot.Found)
				return snap
			},
			want: []string{IDLost},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ids(evaluate(tt.snap()))
			for _, want := range tt.want {
				if !contains(got, want) {
					t.Errorf("expected %s, got %v", want, got)
				}
			}
			for _, unwanted := range tt.notWant {
				if contains(got, unwanted) {
					t.Errorf("did not expect %s, got %v", unwanted, got)
				}
			}
		})
	}
}

// A claim waiting on purpose must not be reported as a failure, because that
// sends the user looking in the wrong place.
func TestWaitForFirstConsumerIsInformational(t *testing.T) {
	snap := kubetest.PVCSnap(kubetest.Claim("data").StorageClass("standard").Build())
	snap.Class = kubetest.ClassInfo("standard", "WaitForFirstConsumer", true, snapshot.Found)
	snap.Consumers = []*snapshot.Pod{kubetest.Snap(kubetest.Pod("db-0").Build())}

	d, ok := find(evaluate(snap), IDWaitingForConsumer)
	if !ok {
		t.Fatal("expected the waiting finding")
	}
	if d.Severity != diagnosis.SeverityInfo {
		t.Errorf("severity = %s, want info", d.Severity)
	}
	if len(d.Suggestions) == 0 || !strings.Contains(d.Suggestions[0].Commands[0], "kubectl why pod db-0") {
		t.Errorf("expected the finding to point at the Pod, got %+v", d.Suggestions)
	}
}

// Consumers that could not be listed must not become "no Pod uses it".
func TestUnknownConsumersProduceNoClaim(t *testing.T) {
	snap := kubetest.PVCSnap(kubetest.Claim("data").StorageClass("standard").Build())
	snap.Class = kubetest.ClassInfo("standard", "WaitForFirstConsumer", true, snapshot.Found)
	snap.ConsumersKnown = false

	if got := ids(evaluate(snap)); contains(got, IDNoConsumer) {
		t.Errorf("consumers that could not be listed must not be reported as absent: %v", got)
	}
}

func TestFallbackReportsAnUnexplainedPendingClaim(t *testing.T) {
	snap := kubetest.PVCSnap(kubetest.Claim("data").StorageClass("fast").Age(20 * time.Minute).Build())
	snap.Class = kubetest.ClassInfo("fast", "Immediate", true, snapshot.Found)

	if got := evaluate(snap); len(got) != 0 {
		t.Fatalf("no rule should fire, got %v", ids(got))
	}
	fallback := Fallback(snap)
	if len(fallback) != 1 || fallback[0].ID != IDPending {
		t.Fatalf("expected %s, got %v", IDPending, ids(fallback))
	}
	if !strings.Contains(fallback[0].Summary, "20m") {
		t.Errorf("summary should say how long it has waited: %q", fallback[0].Summary)
	}
}

func TestFallbackIsSilentForBoundClaims(t *testing.T) {
	snap := kubetest.PVCSnap(kubetest.Claim("data").Bound("pv-1").Build())
	if got := Fallback(snap); got != nil {
		t.Errorf("a bound claim produced %v", ids(got))
	}
}

func TestCatalogIsComplete(t *testing.T) {
	for _, meta := range Catalog() {
		if meta.ID == "" || meta.Title == "" || meta.Description == "" {
			t.Errorf("rule %+v is missing metadata", meta)
		}
		for _, id := range meta.IDs() {
			if !strings.HasPrefix(id, "PVC_") {
				t.Errorf("identifier %q does not follow the PVC_ prefix", id)
			}
		}
	}
}

// A provisioner cannot succeed against a class that does not exist, so the two
// findings are one story rather than two problems.
func TestProvisioningFailureIsLinkedToTheMissingClass(t *testing.T) {
	snap := kubetest.PVCSnap(kubetest.Claim("data").StorageClass("premium-ssd").Build())
	snap.Class = kubetest.ClassInfo("premium-ssd", "", true, snapshot.Missing)
	snap.Events = snapshot.Events{kubetest.Event("Warning", "ProvisioningFailed",
		`storageclass.storage.k8s.io "premium-ssd" not found`)}

	got := diagnosis.Prioritize(evaluate(snap))
	failure, ok := find(got, IDProvisioningFailed)
	if !ok {
		t.Fatal("expected a provisioning finding")
	}
	if failure.CausedBy != IDStorageClassNotFound {
		t.Errorf("causedBy = %q, want the missing class", failure.CausedBy)
	}
	if got[0].ID != IDStorageClassNotFound {
		t.Errorf("first finding = %s, want the root cause first", got[0].ID)
	}
}

// When a class exists, the provisioner's failure is its own problem.
func TestProvisioningFailureStandsAloneWithAValidClass(t *testing.T) {
	snap := kubetest.PVCSnap(kubetest.Claim("data").StorageClass("fast").Build())
	snap.Class = kubetest.ClassInfo("fast", "Immediate", true, snapshot.Found)
	snap.Events = snapshot.Events{kubetest.Event("Warning", "ProvisioningFailed", "quota exceeded")}

	d, _ := find(evaluate(snap), IDProvisioningFailed)
	if d.CausedBy != "" {
		t.Errorf("causedBy = %q, want no link when the class is fine", d.CausedBy)
	}
}
