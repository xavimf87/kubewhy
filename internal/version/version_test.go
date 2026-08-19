package version

import (
	"runtime/debug"
	"testing"
)

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

// `go install <module>@latest` applies no ldflags, so the version has to come
// from what the toolchain recorded about the module it resolved. Without this
// the install method the README leads with reports "dev", and a bug report
// cannot say which version it came from.
func TestVersionFallsBackToTheModuleVersion(t *testing.T) {
	tests := []struct {
		name   string
		ldflag string
		module string
		want   string
	}{
		{name: "go install resolves a released version", ldflag: "dev", module: "v0.2.1", want: "v0.2.1"},
		{name: "a bare module version gets its v", ldflag: "dev", module: "0.2.1", want: "v0.2.1"},
		{name: "a local build stays dev", ldflag: "dev", module: "(devel)", want: "dev"},
		{name: "no module version at all", ldflag: "dev", module: "", want: "dev"},
		{name: "ldflags win over build info", ldflag: "v9.9.9", module: "v0.2.1", want: "v9.9.9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := Info{Version: tt.ldflag}
			applyBuildInfo(&info, &debug.BuildInfo{
				Main: debug.Module{Version: tt.module},
			})
			if info.Version != tt.want {
				t.Errorf("version = %q, want %q", info.Version, tt.want)
			}
		})
	}
}

func TestBuildInfoFillsInTheCommit(t *testing.T) {
	info := Info{Version: "dev"}
	applyBuildInfo(&info, &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123"},
			{Key: "vcs.time", Value: "2026-01-01T00:00:00Z"},
		},
	})
	if info.Commit != "abc123" || info.BuildDate != "2026-01-01T00:00:00Z" {
		t.Errorf("info = %+v, want the recorded revision and time", info)
	}

	// An explicit commit from ldflags is not overwritten.
	fromFlags := Info{Version: "v1.0.0", Commit: "deadbeef"}
	applyBuildInfo(&fromFlags, &debug.BuildInfo{
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}},
	})
	if fromFlags.Commit != "deadbeef" {
		t.Errorf("commit = %q, want the ldflag to win", fromFlags.Commit)
	}
}
