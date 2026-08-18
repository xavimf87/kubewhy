// Package output renders a diagnosis report for humans and for machines.
//
// Renderers never query Kubernetes and never interpret evidence: everything
// they show was decided by a rule.
package output

import (
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// Style holds the presentation choices for the text renderer.
type Style struct {
	Color   bool
	Unicode bool
	Width   int
}

// DetectStyle chooses colours, symbols and width for a writer.
//
// The choices follow what users already expect from other CLIs, so that
// KubeWhy behaves the same way in a terminal, in a pipe, in CI and on Windows
// without anyone having to configure it.
func DetectStyle(w io.Writer, mode ColorMode) Style {
	return Style{
		Color:   useColor(w, mode),
		Unicode: supportsUnicode(),
		Width:   terminalWidth(w),
	}
}

func supportsUnicode() bool {
	if os.Getenv("KUBEWHY_ASCII") != "" {
		return false
	}
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		v := strings.ToLower(os.Getenv(key))
		if v == "" {
			continue
		}
		return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
	}
	// Windows Terminal and modern macOS shells handle UTF-8 without the
	// locale being set, so an unset locale is not evidence against it.
	return os.Getenv("WT_SESSION") != "" || os.Getenv("TERM_PROGRAM") != ""
}

// terminalWidth returns the width to wrap at: what the terminal reports, what
// COLUMNS says, or a conservative default. It is capped because long lines are
// hard to read however wide the window is.
func terminalWidth(w io.Writer) int {
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 40 {
			return min(n, maxWidth)
		}
	}
	if f, ok := w.(*os.File); ok {
		if width, _, err := term.GetSize(int(f.Fd())); err == nil && width > 40 {
			return min(width, maxWidth)
		}
	}
	return 80
}

// maxWidth caps the wrapping width; prose stops being readable well before a
// modern terminal runs out of columns.
const maxWidth = 100

func (s Style) paint(code, text string) string {
	if !s.Color || text == "" {
		return text
	}
	return code + text + ansiReset
}

// key colours a field name, which should recede behind its value.
func (s Style) key(text string) string { return s.paint(ansiDim, text) }

func (s Style) bold(text string) string   { return s.paint(ansiBold, text) }
func (s Style) dim(text string) string    { return s.paint(ansiDim, text) }
func (s Style) red(text string) string    { return s.paint(ansiRed, text) }
func (s Style) green(text string) string  { return s.paint(ansiGreen, text) }
func (s Style) yellow(text string) string { return s.paint(ansiYellow, text) }

// Symbols. Colour is never the only carrier of meaning: the glyph itself
// distinguishes severities so piped and CI output stays readable.
func (s Style) symbolOK() string {
	if s.Unicode {
		return "✓"
	}
	return "[ok]"
}

func (s Style) symbolWarn() string {
	if s.Unicode {
		return "!"
	}
	return "[!]"
}

func (s Style) symbolFail() string {
	if s.Unicode {
		return "✗"
	}
	return "[x]"
}

func (s Style) bullet() string {
	if s.Unicode {
		return "•"
	}
	return "-"
}

func (s Style) treeBranch() string {
	if s.Unicode {
		return "└─"
	}
	return "\\_"
}

// wrap breaks text into lines that fit the given width, keeping words whole.
func wrap(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	line := words[0]
	for _, word := range words[1:] {
		if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) > width {
			lines = append(lines, line)
			line = word
			continue
		}
		line += " " + word
	}
	return append(lines, line)
}
