package output

import (
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// ColorMode is the user's choice about coloured output.
type ColorMode string

const (
	// ColorAuto colours the output when it is going to a terminal.
	ColorAuto ColorMode = "auto"
	// ColorAlways colours it even when piped, for `less -R` and for CI logs
	// that render ANSI.
	ColorAlways ColorMode = "always"
	// ColorNever disables colour entirely.
	ColorNever ColorMode = "never"
)

// ParseColorMode validates the --color flag.
func ParseColorMode(value string) (ColorMode, error) {
	switch ColorMode(value) {
	case ColorAuto, ColorAlways, ColorNever:
		return ColorMode(value), nil
	default:
		return "", errUnknownColorMode{value}
	}
}

type errUnknownColorMode struct{ value string }

func (e errUnknownColorMode) Error() string {
	return "unknown colour mode " + strconv.Quote(e.value) + ": use auto, always or never"
}

// useColor decides whether to emit ANSI sequences.
//
// The order follows what users already expect from other CLIs: an explicit
// choice wins, then NO_COLOR, then a forced override for pipes, then the
// terminal's own capabilities.
func useColor(w io.Writer, mode ColorMode) bool {
	switch mode {
	case ColorNever:
		return false
	case ColorAlways:
		enableVirtualTerminal(w)
		return true
	}

	// https://no-color.org: any value means the user opted out.
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	// The counterpart, honoured by ls, grep and most modern CLIs.
	if force := os.Getenv("CLICOLOR_FORCE"); force != "" && force != "0" {
		enableVirtualTerminal(w)
		return true
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	if !isTerminal(w) {
		return false
	}
	// On Windows, a console only understands ANSI once virtual terminal
	// processing is turned on, and older consoles never will.
	return enableVirtualTerminal(w)
}

// isTerminal reports whether the writer is a terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// ANSI escape sequences. Only the original sixteen colours are used: they are
// the ones every terminal, multiplexer and CI log renderer agrees on, and they
// respect the user's own theme instead of overriding it.
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
)

// Kubernetes vocabulary worth colouring. This is a lookup table of words the
// API itself defines, not an interpretation: deciding what a state means is a
// rule's job, and the renderer only makes the word easier to spot.
var (
	goodWords = words(
		"Running", "Ready", "True", "Bound", "Completed", "Succeeded", "Active",
		"Available", "Healthy", "exists", "Found", "healthy",
	)
	badWords = words(
		"CrashLoopBackOff", "OOMKilled", "Error", "Failed", "False", "NotFound",
		"Unhealthy", "Evicted", "ImagePullBackOff", "ErrImagePull", "InvalidImageName",
		"ErrImageNeverPull", "CreateContainerConfigError", "CreateContainerError",
		"Lost", "Forbidden", "Unschedulable", "ProgressDeadlineExceeded", "FailedMount",
		"FailedScheduling", "FailedBinding", "ProvisioningFailed", "FailedCreate",
		"NotReady", "unhealthy",
		// Phrases the report uses for an object that is absent or unusable.
		"does not exist", "not found", "could not be read", "not started",
		"none published",
	)
	warnWords = words(
		"Pending", "ContainerCreating", "PodInitializing", "Unknown", "Terminating",
		"Waiting", "Terminated", "BackOff", "ContainersNotReady", "Paused", "paused",
		"degraded", "unknown", "no status reported", "no reason reported",
	)
)

func words(list ...string) *regexp.Regexp {
	quoted := make([]string, 0, len(list))
	for _, word := range list {
		quoted = append(quoted, regexp.QuoteMeta(word))
	}
	// Word boundaries keep "NotReady" from matching inside "NotReadyAddresses".
	return regexp.MustCompile(`\b(` + strings.Join(quoted, "|") + `)\b`)
}

// ratioRe matches the "0 of 3" and "0/1" shapes used for readiness counts.
var ratioRe = regexp.MustCompile(`^(\d+)\s*(?:of|/)\s*(\d+)\b`)

// value colours the Kubernetes vocabulary inside a rendered value.
func (s Style) value(text string) string {
	if !s.Color || text == "" {
		return text
	}

	// A ratio says more than any single word in it: nothing ready is a
	// failure, some of them is a warning, all of them is fine.
	if m := ratioRe.FindStringSubmatch(text); m != nil {
		have, _ := strconv.Atoi(m[1])
		want, _ := strconv.Atoi(m[2])
		switch {
		case want > 0 && have == 0:
			return s.paint(ansiRed, text)
		case have < want:
			return s.paint(ansiYellow, text)
		default:
			return s.paint(ansiGreen, text)
		}
	}

	out := badWords.ReplaceAllString(text, ansiRed+"$1"+ansiReset)
	out = warnWords.ReplaceAllString(out, ansiYellow+"$1"+ansiReset)
	out = goodWords.ReplaceAllString(out, ansiGreen+"$1"+ansiReset)
	return out
}

// note colours a trailing qualifier. Known vocabulary keeps its meaning;
// anything else recedes, because a note is secondary by definition.
func (s Style) note(text string) string {
	if !s.Color || text == "" {
		return text
	}
	if painted := s.value(text); painted != text {
		return painted
	}
	return s.paint(ansiDim, text)
}

// command colours a suggested command, which the user is meant to copy.
func (s Style) command(text string) string { return s.paint(ansiCyan, text) }

// ruleID colours a diagnosis identifier.
func (s Style) ruleID(text string) string { return s.paint(ansiMagenta, text) }

// heading colours a finding's heading by how serious the finding is.
func (s Style) heading(text string, severity severityTone) string {
	switch severity {
	case toneCritical:
		return s.paint(ansiBold+ansiRed, text)
	case toneWarning:
		return s.paint(ansiBold+ansiYellow, text)
	default:
		return s.paint(ansiBold+ansiBlue, text)
	}
}

// severityTone is the renderer's view of a severity: how loud to be.
type severityTone int

const (
	toneInfo severityTone = iota
	toneWarning
	toneCritical
)
