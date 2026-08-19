package version

import "testing"

// The version a binary reports has to be the string a user can act on: the git
// tag, the GitHub release and `go install ...@v1.2.3` all carry the leading v.
// Whoever builds the binary chooses what goes into the ldflag, and GoReleaser's
// own .Version template strips it.
func TestCanonicalVersion(t *testing.T) {
	tests := map[string]string{
		"1.2.3":            "v1.2.3",
		"v1.2.3":           "v1.2.3",
		"0.1.0-rc.1":       "v0.1.0-rc.1",
		"v0.1.0-9-gabc123": "v0.1.0-9-gabc123",
		"dev":              "dev",
		"":                 "",
	}
	for in, want := range tests {
		if got := canonical(in); got != want {
			t.Errorf("canonical(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetReportsSomethingUsable(t *testing.T) {
	info := Get()
	if info.Version == "" {
		t.Error("a build with no ldflags must still report a version")
	}
	if info.GoVersion == "" || info.Platform == "" {
		t.Errorf("info = %+v, want the toolchain and platform filled in", info)
	}
	if got := info.String(); got == "" || got[:8] != "KubeWhy " {
		t.Errorf("String() = %q", got)
	}
}
