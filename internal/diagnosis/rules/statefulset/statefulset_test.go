package statefulset

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func evaluate(snap *snapshot.StatefulSet) []diagnosis.Diagnosis {
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

func TestStatefulSetRules(t *testing.T) {
	tests := []struct {
		name    string
		snap    func() *snapshot.StatefulSet
		want    []string
		notWant []string
	}{
		{
			name: "a healthy set produces no findings",
			snap: func() *snapshot.StatefulSet {
				return kubetest.StatefulSetSnap(
					kubetest.StatefulSet("db").Replicas(3).Status(3, 3, 3).Build(),
					true, true, true)
			},
			notWant: []string{IDOrderedRolloutBlocked, IDUnavailableReplicas, IDServiceNotFound},
		},
		{
			name: "one unready replica blocks the ones after it",
			snap: func() *snapshot.StatefulSet {
				// Three wanted, two exist, and the second is not ready.
				return kubetest.StatefulSetSnap(
					kubetest.StatefulSet("db").Replicas(3).Status(1, 2, 2).Build(),
					true, false)
			},
			want: []string{IDOrderedRolloutBlocked, IDUnavailableReplicas},
		},
		{
			name: "parallel management blocks nothing",
			snap: func() *snapshot.StatefulSet {
				return kubetest.StatefulSetSnap(
					kubetest.StatefulSet("db").Replicas(3).Parallel().Status(1, 2, 2).Build(),
					true, false)
			},
			want:    []string{IDUnavailableReplicas},
			notWant: []string{IDOrderedRolloutBlocked},
		},
		{
			name: "the governing Service is missing",
			snap: func() *snapshot.StatefulSet {
				snap := kubetest.StatefulSetSnap(
					kubetest.StatefulSet("db").Replicas(1).Status(1, 1, 1).Build(), true)
				snap.Service = snapshot.HeadlessService{Name: "db", Exists: snapshot.Missing}
				return snap
			},
			want: []string{IDServiceNotFound},
		},
		{
			name: "the governing Service is not headless",
			snap: func() *snapshot.StatefulSet {
				snap := kubetest.StatefulSetSnap(
					kubetest.StatefulSet("db").Replicas(1).Status(1, 1, 1).Build(), true)
				snap.Service = snapshot.HeadlessService{Name: "db", Exists: snapshot.Found, Headless: false}
				return snap
			},
			want: []string{IDServiceNotHeadless},
		},
		{
			name: "an OnDelete rollout is waiting on purpose",
			snap: func() *snapshot.StatefulSet {
				return kubetest.StatefulSetSnap(
					kubetest.StatefulSet("db").Replicas(2).OnDelete().PendingRollout().
						Status(2, 2, 0).Build(), true, true)
			},
			want:    []string{IDUpdateOnDelete},
			notWant: []string{IDUnavailableReplicas},
		},
		{
			name: "a partitioned rollout is waiting on purpose",
			snap: func() *snapshot.StatefulSet {
				return kubetest.StatefulSetSnap(
					kubetest.StatefulSet("db").Replicas(3).Partition(2).PendingRollout().
						Status(3, 3, 1).Build(), true, true, true)
			},
			want:    []string{IDUpdatePartitioned},
			notWant: []string{IDUnavailableReplicas},
		},
		{
			name: "scaled to zero is not broken",
			snap: func() *snapshot.StatefulSet {
				return kubetest.StatefulSetSnap(kubetest.StatefulSet("db").Replicas(0).Build())
			},
			want:    []string{IDScaledToZero},
			notWant: []string{IDUnavailableReplicas, IDOrderedRolloutBlocked},
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

// The replicas that were never created are the point of the finding: they are
// not failing, they do not exist.
func TestBlockedRolloutNamesTheReplicasThatDoNotExist(t *testing.T) {
	snap := kubetest.StatefulSetSnap(
		kubetest.StatefulSet("db").Namespace("prod").Replicas(5).Status(1, 2, 2).Build(),
		true, false)

	d, ok := find(evaluate(snap), IDOrderedRolloutBlocked)
	if !ok {
		t.Fatal("expected the ordering finding")
	}
	if d.Component != "db-1" {
		t.Errorf("component = %q, want the blocking Pod", d.Component)
	}
	var never string
	for _, e := range d.Evidence {
		if e.Field == "neverCreated" {
			never = e.Value
		}
	}
	for _, want := range []string{"db-2", "db-3", "db-4"} {
		if !strings.Contains(never, want) {
			t.Errorf("neverCreated = %q, want it to include %s", never, want)
		}
	}
	if strings.Contains(never, "db-1") {
		t.Errorf("neverCreated = %q, the blocking Pod does exist", never)
	}
	if len(d.Suggestions) == 0 || !strings.Contains(d.Suggestions[0].Commands[0], "kubectl why pod db-1") {
		t.Error("the finding should send the user to the blocking Pod")
	}
}

func TestTemplatedClaims(t *testing.T) {
	t.Run("an unbound claim blocks its own replica", func(t *testing.T) {
		snap := kubetest.StatefulSetSnap(
			kubetest.StatefulSet("db").Replicas(2).ClaimTemplate("data").Status(1, 2, 2).Build(),
			true, false)
		kubetest.TemplatedClaim(snap, "data", 0, corev1.ClaimBound)
		kubetest.TemplatedClaim(snap, "data", 1, corev1.ClaimPending)

		d, ok := find(evaluate(snap), IDClaimNotBound)
		if !ok {
			t.Fatalf("expected an unbound claim finding, got %v", ids(evaluate(snap)))
		}
		if !strings.Contains(d.Summary, "db-1") {
			t.Errorf("summary = %q, want it to name the replica the claim belongs to", d.Summary)
		}
		if d.Severity != diagnosis.SeverityCritical {
			t.Errorf("severity = %s, want critical", d.Severity)
		}
	})

	t.Run("a claim waiting for its consumer is expected", func(t *testing.T) {
		snap := kubetest.StatefulSetSnap(
			kubetest.StatefulSet("db").Replicas(1).ClaimTemplate("data").Status(0, 1, 1).Build(),
			false)
		claim := kubetest.TemplatedClaim(snap, "data", 0, corev1.ClaimPending)
		claim.StorageClass = "standard"
		claim.BindingMode = "WaitForFirstConsumer"

		d, ok := find(evaluate(snap), IDClaimNotBound)
		if !ok {
			t.Fatal("expected the claim finding")
		}
		if d.Severity != diagnosis.SeverityInfo {
			t.Errorf("severity = %s, want info: the claim is waiting by design", d.Severity)
		}
	})

	t.Run("a missing claim is reported separately", func(t *testing.T) {
		snap := kubetest.StatefulSetSnap(
			kubetest.StatefulSet("db").Replicas(1).ClaimTemplate("data").Status(0, 0, 0).Build())
		snap.Claims["data-db-0"] = &snapshot.PVCRef{Name: "data-db-0", Exists: snapshot.Missing}

		if got := ids(evaluate(snap)); !contains(got, IDClaimNotFound) {
			t.Errorf("expected %s, got %v", IDClaimNotFound, got)
		}
	})

	// A claim that could not be read is not a claim that is broken.
	t.Run("an unreadable claim produces nothing", func(t *testing.T) {
		snap := kubetest.StatefulSetSnap(
			kubetest.StatefulSet("db").Replicas(1).ClaimTemplate("data").Status(0, 0, 0).Build())
		snap.Claims["data-db-0"] = &snapshot.PVCRef{Name: "data-db-0", Exists: snapshot.Unknown}

		got := ids(evaluate(snap))
		if contains(got, IDClaimNotFound) || contains(got, IDClaimNotBound) {
			t.Errorf("a claim that could not be read must not be reported: %v", got)
		}
	})
}

// A StatefulSet with no rollout pending is not held back by its strategy.
func TestUpdateStrategyIsQuietWithNothingToRoll(t *testing.T) {
	for _, snap := range []*snapshot.StatefulSet{
		kubetest.StatefulSetSnap(kubetest.StatefulSet("db").Replicas(2).OnDelete().Status(2, 2, 2).Build(), true, true),
		kubetest.StatefulSetSnap(kubetest.StatefulSet("db").Replicas(2).Partition(1).Status(2, 2, 2).Build(), true, true),
	} {
		got := ids(evaluate(snap))
		if contains(got, IDUpdateOnDelete) || contains(got, IDUpdatePartitioned) {
			t.Errorf("nothing to roll out, so nothing to report: %v", got)
		}
	}
}

func TestOrdinalParsing(t *testing.T) {
	tests := map[string]int{"db-0": 0, "db-12": 12, "data-db-3": 3, "my-app-2-7": 7}
	for name, want := range tests {
		got, ok := snapshot.Ordinal(name)
		if !ok || got != want {
			t.Errorf("Ordinal(%q) = %d, %v; want %d", name, got, ok, want)
		}
	}
	if _, ok := snapshot.Ordinal("db"); ok {
		t.Error("a name with no ordinal must not parse as one")
	}
}

func TestCatalogIsComplete(t *testing.T) {
	for _, meta := range Catalog() {
		if meta.ID == "" || meta.Title == "" || meta.Description == "" {
			t.Errorf("rule %+v is missing metadata", meta)
		}
		for _, id := range meta.IDs() {
			if !strings.HasPrefix(id, "STATEFULSET_") {
				t.Errorf("identifier %q does not follow the STATEFULSET_ prefix", id)
			}
		}
	}
}

// One problem, one finding: a replica whose volume is not bound is explained
// by the claim rule, so the Pod fallback must not repeat it underneath.
func TestClaimFindingSuppressesThePodFallback(t *testing.T) {
	snap := kubetest.StatefulSetSnap(
		kubetest.StatefulSet("db").Replicas(2).ClaimTemplate("data").Status(1, 2, 2).Build(),
		true, false)
	kubetest.TemplatedClaim(snap, "data", 0, corev1.ClaimBound)
	kubetest.TemplatedClaim(snap, "data", 1, corev1.ClaimPending)

	if got := ids(PodFindings(context.Background(), snap)); contains(got, "POD_NOT_READY") {
		t.Errorf("the claim rule already explains that Pod: %v", got)
	}

	// A Pod that is unready for some other reason still gets the fallback.
	other := kubetest.StatefulSetSnap(
		kubetest.StatefulSet("db").Replicas(2).ClaimTemplate("data").Status(1, 2, 2).Build(),
		true, false)
	kubetest.TemplatedClaim(other, "data", 0, corev1.ClaimBound)
	kubetest.TemplatedClaim(other, "data", 1, corev1.ClaimBound)

	if got := ids(PodFindings(context.Background(), other)); !contains(got, "POD_NOT_READY") {
		t.Errorf("an unexplained Pod must still be reported: %v", got)
	}
}
