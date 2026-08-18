package diagnosis

import "testing"

func TestAggregateByRule(t *testing.T) {
	pod := func(name string) ResourceRef { return ResourceRef{Kind: "Pod", Namespace: "prod", Name: name} }
	in := []Diagnosis{
		{ID: "POD_OOM_KILLED", Component: "api", Summary: "oom", Subject: pod("a")},
		{ID: "POD_CRASH_LOOP", Component: "worker", Summary: "crash", Subject: pod("a")},
		{ID: "POD_OOM_KILLED", Component: "api", Summary: "oom", Subject: pod("b")},
		{ID: "POD_OOM_KILLED", Component: "api", Summary: "oom", Subject: pod("c")},
	}

	got := AggregateByRule(in, 3)
	if len(got) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(got), got)
	}
	if got[0].ID != "POD_OOM_KILLED" {
		t.Errorf("first = %s, want the most widespread problem first", got[0].ID)
	}
	if got[0].Aggregate.Count != 3 || got[0].Aggregate.Total != 3 {
		t.Errorf("aggregate = %+v, want 3 of 3", got[0].Aggregate)
	}
	if len(got[0].Aggregate.Subjects) != 3 {
		t.Errorf("subjects = %+v, want every Pod preserved", got[0].Aggregate.Subjects)
	}
	if got[1].Aggregate.Count != 1 {
		t.Errorf("second aggregate = %+v", got[1].Aggregate)
	}
}

// Findings about different containers are different problems.
func TestAggregateKeepsComponentsApart(t *testing.T) {
	in := []Diagnosis{
		{ID: "POD_CRASH_LOOP", Component: "api", Summary: "a", Subject: ResourceRef{Name: "p1"}},
		{ID: "POD_CRASH_LOOP", Component: "worker", Summary: "b", Subject: ResourceRef{Name: "p1"}},
	}
	if got := AggregateByRule(in, 1); len(got) != 2 {
		t.Errorf("got %d findings, want both containers reported", len(got))
	}
}
