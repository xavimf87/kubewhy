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
)

// Style holds the presentation choices for the text renderer.
type Style struct {
	Color   bool
	Unicode bool
	Width   int
}

// DetectStyle chooses colours, symbols and width from the environment, the
// way a well-behaved CLI is expected to: no colour when the output is piped,
// no colour when NO_COLOR is set, ASCII when the locale is not UTF-8.
func DetectStyle(w io.Writer, forceNoColor bool) Style {
	tty := isTerminal(w)
	style := Style{
		Color:   tty && !forceNoColor && os.Getenv("NO_COLOR") == "",
		Unicode: supportsUnicode(),
		Width:   terminalWidth(),
	}
	return style
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
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

func terminalWidth() int {
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 40 {
			return min(n, 100)
		}
	}
	return 80
}

// ANSI escape sequences, applied only when colour is enabled.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
)

func (s Style) paint(code, text string) string {
	if !s.Color || text == "" {
		return text
	}
	return code + text + ansiReset
}

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
