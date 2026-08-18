package ingress

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	podrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pod"
	"github.com/xavimf87/kubewhy/internal/kubetest"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

func evaluate(snap *snapshot.Ingress) []diagnosis.Diagnosis {
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

// working builds an Ingress whose whole chain resolves.
func working() *snapshot.Ingress {
	snap := kubetest.IngressSnap(kubetest.Ingress("api").
		Class("nginx").Address("lb.example.com").
		Rule("api.example.com", "/", "api", 80).Build())
	snap.Class = snapshot.IngressClass{Name: "nginx", Exists: snapshot.Found}
	kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(2, 0))
	return snap
}

func TestIngressRules(t *testing.T) {
	tests := []struct {
		name    string
		snap    func() *snapshot.Ingress
		want    []string
		notWant []string
	}{
		{
			name:    "a working ingress produces no findings",
			snap:    working,
			notWant: []string{IDServiceNotFound, IDServiceNoEndpoints, IDServicePortNotFound, IDNoAddress, IDNoClass},
		},
		{
			name: "the backend service does not exist",
			snap: func() *snapshot.Ingress {
				snap := working()
				kubetest.IngressBackend(snap, "api", snapshot.Missing, nil, snapshot.EndpointSet{})
				return snap
			},
			want:    []string{IDServiceNotFound},
			notWant: []string{IDServiceNoEndpoints},
		},
		{
			name: "the backend service does not expose the port",
			snap: func() *snapshot.Ingress {
				snap := kubetest.IngressSnap(kubetest.Ingress("api").
					Class("nginx").Address("lb.example.com").
					Rule("api.example.com", "/", "api", 8080).Build())
				snap.Class = snapshot.IngressClass{Name: "nginx", Exists: snapshot.Found}
				kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(2, 0))
				return snap
			},
			want:    []string{IDServicePortNotFound},
			notWant: []string{IDServiceNoEndpoints},
		},
		{
			name: "the backend service has no ready endpoints",
			snap: func() *snapshot.Ingress {
				snap := working()
				kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(0, 2))
				return snap
			},
			want: []string{IDServiceNoEndpoints},
		},
		{
			name: "the named ingress class does not exist",
			snap: func() *snapshot.Ingress {
				snap := kubetest.IngressSnap(kubetest.Ingress("api").
					Class("nginx").Rule("api.example.com", "/", "api", 80).Build())
				snap.Class = snapshot.IngressClass{Name: "nginx", Exists: snapshot.Missing}
				kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(2, 0))
				return snap
			},
			want: []string{IDClassNotFound},
			// The missing class already explains the missing address.
			notWant: []string{IDNoAddress},
		},
		{
			name: "no class named and no default in the cluster",
			snap: func() *snapshot.Ingress {
				snap := kubetest.IngressSnap(kubetest.Ingress("api").
					Rule("api.example.com", "/", "api", 80).Build())
				snap.Class = snapshot.IngressClass{DefaultExists: snapshot.Missing}
				kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(2, 0))
				return snap
			},
			want:    []string{IDNoClass},
			notWant: []string{IDClassNotFound, IDNoAddress},
		},
		{
			name: "a cluster default class is enough",
			snap: func() *snapshot.Ingress {
				snap := kubetest.IngressSnap(kubetest.Ingress("api").Address("lb.example.com").
					Rule("api.example.com", "/", "api", 80).Build())
				snap.Class = snapshot.IngressClass{Name: "nginx", Exists: snapshot.Found, DefaultExists: snapshot.Found}
				kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(1, 0))
				return snap
			},
			notWant: []string{IDNoClass, IDClassNotFound},
		},
		{
			name: "no address after the grace period",
			snap: func() *snapshot.Ingress {
				snap := kubetest.IngressSnap(kubetest.Ingress("api").Age(30*time.Minute).
					Class("nginx").Rule("api.example.com", "/", "api", 80).Build())
				snap.Class = snapshot.IngressClass{Name: "nginx", Exists: snapshot.Found}
				kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(1, 0))
				return snap
			},
			want: []string{IDNoAddress},
		},
		{
			name: "a freshly created ingress is given time",
			snap: func() *snapshot.Ingress {
				snap := kubetest.IngressSnap(kubetest.Ingress("api").Age(20*time.Second).
					Class("nginx").Rule("api.example.com", "/", "api", 80).Build())
				snap.Class = snapshot.IngressClass{Name: "nginx", Exists: snapshot.Found}
				kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(1, 0))
				return snap
			},
			notWant: []string{IDNoAddress},
		},
		{
			name: "an ingress with nothing to route",
			snap: func() *snapshot.Ingress {
				snap := kubetest.IngressSnap(kubetest.Ingress("api").Address("lb.example.com").Build())
				snap.Class = snapshot.IngressClass{Name: "nginx", Exists: snapshot.Found}
				return snap
			},
			want: []string{IDNoRules},
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

// A Service that could not be read is not a Service that does not exist.
func TestUnreadableServiceIsNotReportedAsMissing(t *testing.T) {
	snap := working()
	kubetest.IngressBackend(snap, "api", snapshot.Unknown, nil, snapshot.EndpointSet{})

	if got := ids(evaluate(snap)); contains(got, IDServiceNotFound) {
		t.Errorf("a Service that could not be read must not be reported as missing: %v", got)
	}
}

// Several paths pointing at one broken Service are one problem.
func TestOneFindingPerBrokenService(t *testing.T) {
	snap := kubetest.IngressSnap(kubetest.Ingress("api").
		Class("nginx").Address("lb.example.com").
		Rule("api.example.com", "/v1", "api", 80).
		Rule("api.example.com", "/v2", "api", 80).
		Rule("www.example.com", "/", "api", 80).Build())
	snap.Class = snapshot.IngressClass{Name: "nginx", Exists: snapshot.Found}
	kubetest.IngressBackend(snap, "api", snapshot.Missing, nil, snapshot.EndpointSet{})

	got := evaluate(snap)
	if len(got) != 1 {
		t.Fatalf("expected one finding for one broken Service, got %v", ids(got))
	}
	routes := 0
	for _, e := range got[0].Evidence {
		if e.Field == "route" {
			routes++
		}
	}
	if routes != 3 {
		t.Errorf("route evidence = %d, want every affected route listed", routes)
	}
}

func TestBackendPodFindingsExplainMissingEndpoints(t *testing.T) {
	snap := working()
	entry := kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(0, 2))
	crashing := func(name string) *snapshot.Pod {
		return kubetest.Snap(kubetest.Pod(name).
			Condition(corev1.PodReady, corev1.ConditionFalse, "ContainersNotReady", "").
			Container(kubetest.Container("api").
				Waiting("CrashLoopBackOff", "back-off").
				LastTerminated("Error", 1).Restarts(4)).Build())
	}
	entry.Backends = []*snapshot.Pod{crashing("api-a"), crashing("api-b")}

	got := PodFindings(context.Background(), snap)
	if len(got) != 1 || got[0].ID != podrules.IDCrashLoop {
		t.Fatalf("expected the Pod rules to explain the backends, got %v", ids(got))
	}
	if got[0].Aggregate.Count != 2 {
		t.Errorf("aggregate = %+v, want both Pods collapsed", got[0].Aggregate)
	}
}

// An ExternalName backend publishes no endpoints by design.
func TestExternalNameBackendIsNotReported(t *testing.T) {
	snap := working()
	entry := kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(0, 0))
	entry.Service.Spec.Type = corev1.ServiceTypeExternalName
	entry.Service.Spec.ExternalName = "api.example.com"

	if got := ids(evaluate(snap)); contains(got, IDServiceNoEndpoints) {
		t.Errorf("an ExternalName backend has no endpoints by design: %v", got)
	}
}

func TestNoAddressStaysAtPossibleConfidence(t *testing.T) {
	snap := kubetest.IngressSnap(kubetest.Ingress("api").Age(time.Hour).
		Class("nginx").Rule("api.example.com", "/", "api", 80).Build())
	snap.Class = snapshot.IngressClass{Name: "nginx", Exists: snapshot.Found}
	kubetest.IngressBackend(snap, "api", snapshot.Found, []int32{80}, kubetest.ReadyEndpoints(1, 0))

	d, ok := find(evaluate(snap), IDNoAddress)
	if !ok {
		t.Fatal("expected an address finding")
	}
	if d.Confidence != diagnosis.ConfidencePossible {
		t.Errorf("confidence = %s, want possible: the API does not say why", d.Confidence)
	}
}

func TestCatalogIsComplete(t *testing.T) {
	for _, meta := range Catalog() {
		if meta.ID == "" || meta.Title == "" || meta.Description == "" {
			t.Errorf("rule %+v is missing metadata", meta)
		}
		for _, id := range meta.IDs() {
			if !strings.HasPrefix(id, "INGRESS_") {
				t.Errorf("identifier %q does not follow the INGRESS_ prefix", id)
			}
		}
	}
}
