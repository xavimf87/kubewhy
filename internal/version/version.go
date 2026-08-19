// Package version carries the build metadata shown by `kubectl why version`.
package version

import (
	"fmt"
	"regexp"
	"runtime"
	"runtime/debug"
)

// These values are injected at build time with -ldflags. They keep their
// defaults when the binary is built with plain `go build`.
var (
	// Version is the release tag, e.g. v0.1.0.
	Version = "dev"
	// Commit is the git revision the binary was built from.
	Commit = ""
	// Date is the build timestamp in RFC 3339.
	Date = ""
)

// Info describes the running binary.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"buildDate,omitempty"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// semverRe matches a bare semantic version, with no leading v.
var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// canonical gives a version the leading v that the git tag, the GitHub release
// and `go install ...@v1.2.3` all use. Whoever builds the binary decides what
// goes in the ldflag, and a packager passing a bare 1.2.3 should not end up
// reporting a version that matches nothing a user can type.
func canonical(version string) string {
	if semverRe.MatchString(version) {
		return "v" + version
	}
	return version
}

// Get returns the build metadata, falling back to what the Go toolchain
// recorded when ldflags were not used.
func Get() Info {
	info := Info{
		Version:   canonical(Version),
		Commit:    Commit,
		BuildDate: Date,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
	if build, ok := debug.ReadBuildInfo(); ok {
		applyBuildInfo(&info, build)
	}
	return info
}

// applyBuildInfo fills in what the Go toolchain recorded, for binaries built
// without ldflags.
//
// `go install github.com/xavimf87/kubewhy/cmd/kubectl-why@latest` is the
// install method the README leads with, and it applies no ldflags at all — so
// without this the binary reports "dev" and a bug report cannot say which
// version it came from. Go records the module version it resolved, which is
// exactly the answer.
func applyBuildInfo(info *Info, build *debug.BuildInfo) {
	if info.Version == "dev" && build.Main.Version != "" && build.Main.Version != "(devel)" {
		info.Version = canonical(build.Main.Version)
	}
	if info.Commit != "" {
		return
	}
	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.Commit = setting.Value
		case "vcs.time":
			if info.BuildDate == "" {
				info.BuildDate = setting.Value
			}
		}
	}
}

// String renders the metadata as the version command prints it.
func (i Info) String() string {
	out := "KubeWhy " + i.Version
	if i.Commit != "" {
		out += "\ncommit:   " + i.Commit
	}
	if i.BuildDate != "" {
		out += "\nbuilt:    " + i.BuildDate
	}
	out += "\ngo:       " + i.GoVersion
	out += "\nplatform: " + i.Platform
	return out
}
