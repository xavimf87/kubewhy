package diagnosis

import "testing"

func TestPrioritizePutsRootCausesFirst(t *testing.T) {
	in := []Diagnosis{
		{ID: "POD_UNSCHEDULABLE_VOLUME", Severity: SeverityCritical, Confidence: ConfidenceCertain, CausedBy: "POD_PVC_NOT_BOUND"},
		{ID: "POD_READINESS_PROBE_FAILED", Severity: SeverityWarning, Confidence: ConfidenceCertain},
		{ID: "POD_PVC_NOT_BOUND", Severity: SeverityCritical, Confidence: ConfidenceCertain},
		{ID: "POD_NOTE", Severity: SeverityInfo, Confidence: ConfidencePossible},
	}

	got := Prioritize(in)
	want := []string{"POD_PVC_NOT_BOUND", "POD_UNSCHEDULABLE_VOLUME", "POD_READINESS_PROBE_FAILED", "POD_NOTE"}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("position %d = %s, want %s (order: %v)", i, got[i].ID, id, idsOf(got))
		}
	}
}

func TestPrioritizePrefersHigherConfidence(t *testing.T) {
	in := []Diagnosis{
		{ID: "B", Severity: SeverityCritical, Confidence: ConfidencePossible},
		{ID: "A", Severity: SeverityCritical, Confidence: ConfidenceCertain},
	}
	if got := Prioritize(in); got[0].ID != "A" {
		t.Errorf("order = %v, want the certain finding first", idsOf(got))
	}
}

func TestDeriveStatus(t *testing.T) {
	tests := []struct {
		name   string
		report Report
		want   Status
	}{
		{name: "no findings", report: Report{}, want: StatusHealthy},
		{
			name:   "warning only",
			report: Report{Diagnoses: []Diagnosis{{Severity: SeverityWarning}}},
			want:   StatusDegraded,
		},
		{
			name:   "critical wins",
			report: Report{Diagnoses: []Diagnosis{{Severity: SeverityWarning}, {Severity: SeverityCritical}}},
			want:   StatusUnhealthy,
		},
		{
			name:   "info alone is still healthy",
			report: Report{Diagnoses: []Diagnosis{{Severity: SeverityInfo}}},
			want:   StatusHealthy,
		},
		{
			name:   "missing evidence is not health",
			report: Report{Degradations: []Degradation{{Resource: "events"}}},
			want:   StatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.report.DeriveStatus(); got != tt.want {
				t.Errorf("DeriveStatus() = %s, want %s", got, tt.want)
			}
		})
	}
}

func idsOf(ds []Diagnosis) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.ID)
	}
	return out
}
