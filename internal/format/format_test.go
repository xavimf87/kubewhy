package format

import (
	"testing"
	"time"
)

func TestDuration(t *testing.T) {
	tests := map[time.Duration]string{
		0:                  "0s",
		45 * time.Second:   "45s",
		12 * time.Minute:   "12m",
		90 * time.Minute:   "1h30m",
		3 * time.Hour:      "3h",
		26 * time.Hour:     "1d2h",
		9 * 24 * time.Hour: "9d",
		-1 * time.Second:   "0s",
	}
	for in, want := range tests {
		if got := Duration(in); got != want {
			t.Errorf("Duration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestCount(t *testing.T) {
	if got := Count(1, "restart", "restarts"); got != "1 restart" {
		t.Errorf("Count(1) = %q", got)
	}
	if got := Count(3, "restart", "restarts"); got != "3 restarts" {
		t.Errorf("Count(3) = %q", got)
	}
}
