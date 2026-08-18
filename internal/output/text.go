package output

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
)

// TextOptions controls how much of the report the text renderer shows.
type TextOptions struct {
	Style Style
	// Verbose adds every piece of evidence and what KubeWhy inspected.
	Verbose bool
}

// Text renders a report the way a human reads it: verdict first, then the
// finding that explains it, then the facts behind it.
func Text(w io.Writer, report *diagnosis.Report, opts TextOptions) error {
	r := &textRenderer{w: w, style: opts.Style, verbose: opts.Verbose}
	r.render(report)
	return r.err
}

type textRenderer struct {
	w       io.Writer
	style   Style
	verbose bool
	err     error
}

func (r *textRenderer) printf(format string, args ...any) {
	if r.err != nil {
		return
	}
	_, r.err = fmt.Fprintf(r.w, format, args...)
}

func (r *textRenderer) line(text string) { r.printf("%s\n", text) }
func (r *textRenderer) blank()           { r.printf("\n") }

func (r *textRenderer) render(report *diagnosis.Report) {
	r.headline(report)

	shown := map[string]bool{}
	for i, d := range report.Diagnoses {
		r.blank()
		r.diagnosis(d, i == 0, shown)
		shown[d.ID] = true
	}

	for _, section := range report.Overview {
		r.blank()
		r.section(section)
	}

	if len(report.Degradations) > 0 {
		r.blank()
		r.degradations(report.Degradations)
	}

	// An observation is not an issue, so the reassurance still belongs here.
	// It is deliberately not "everything works": Kubernetes cannot prove that.
	if report.Status == diagnosis.StatusHealthy {
		r.blank()
		r.line("No obvious Kubernetes-level issue detected.")
	}

	if r.verbose && len(report.Inspected) > 0 {
		r.blank()
		r.line(r.style.bold("Inspected"))
		for _, item := range report.Inspected {
			r.line("  " + item)
		}
	}
}

func (r *textRenderer) headline(report *diagnosis.Report) {
	symbol := r.style.symbolOK()
	paint := r.style.green
	switch report.Status {
	case diagnosis.StatusUnhealthy:
		symbol, paint = r.style.symbolFail(), r.style.red
	case diagnosis.StatusDegraded, diagnosis.StatusUnknown:
		symbol, paint = r.style.symbolWarn(), r.style.yellow
	}
	r.line(paint(symbol) + " " + r.style.bold(report.Headline))
}

// diagnosis renders one finding. The first critical finding is presented as
// the root cause; later ones are additional or consequences of it.
func (r *textRenderer) diagnosis(d diagnosis.Diagnosis, first bool, shown map[string]bool) {
	r.line(r.style.heading(r.heading(d, first, shown), tone(d.Severity)))

	summary := d.Summary
	if d.Aggregate != nil && d.Aggregate.Count > 1 {
		summary = fmt.Sprintf("%s (%d of %d Pods)", summary, d.Aggregate.Count, d.Aggregate.Total)
	}
	r.paragraph(summary, "  ")

	if d.Explanation != "" {
		r.blank()
		r.paragraph(d.Explanation, "  ")
	}

	if len(d.Evidence) > 0 {
		r.blank()
		r.line("  Evidence")
		r.evidence(d.Evidence)
	}

	if len(d.PossibleCauses) > 0 {
		r.blank()
		r.line("  Possible causes")
		for _, cause := range d.PossibleCauses {
			r.bullet(cause, "    ")
		}
	}

	if len(d.Suggestions) > 0 {
		r.blank()
		if len(d.Suggestions) == 1 {
			r.line("  Suggested action")
		} else {
			r.line("  Suggested actions")
		}
		for i, suggestion := range d.Suggestions {
			if i > 0 {
				r.blank()
			}
			r.paragraph(suggestion.Description, "    ")
			for _, cmd := range suggestion.Commands {
				r.line("      " + r.style.command(cmd))
			}
		}
	}

	if !r.verbose {
		return
	}
	r.blank()
	r.line("  " + r.style.dim("rule ") + r.style.ruleID(d.ID) +
		r.style.dim(fmt.Sprintf(", confidence %s, severity %s", d.Confidence, d.Severity)))
}

func (r *textRenderer) heading(d diagnosis.Diagnosis, first bool, shown map[string]bool) string {
	if d.CausedBy != "" && shown[d.CausedBy] {
		return "CONSEQUENCE"
	}
	if !first {
		return "ALSO DETECTED"
	}
	switch d.Severity {
	case diagnosis.SeverityCritical:
		return "ROOT CAUSE"
	case diagnosis.SeverityWarning:
		return "WARNING"
	default:
		return "OBSERVATION"
	}
}

// evidence prints field/value pairs aligned in a column, with the verbatim
// Kubernetes message underneath when there is one.
func (r *textRenderer) evidence(items []diagnosis.Evidence) {
	shown := items
	if !r.verbose && len(items) > 8 {
		shown = items[:8]
	}

	width := 0
	for _, e := range shown {
		if n := utf8.RuneCountInString(evidenceLabel(e)); n > width {
			width = n
		}
	}
	for _, e := range shown {
		label := evidenceLabel(e)
		padding := strings.Repeat(" ", width-utf8.RuneCountInString(label))
		switch {
		case e.Value != "":
			r.line("    " + r.style.key(label) + padding + "  " + r.style.value(e.Value))
		default:
			r.line("    " + r.style.key(label))
		}
		if e.Message != "" {
			for _, l := range wrap(e.Message, r.style.Width-6) {
				r.line("      " + l)
			}
		}
	}
	if len(shown) < len(items) {
		r.line("    " + r.style.dim(fmt.Sprintf("... %d more (use --verbose)", len(items)-len(shown))))
	}
}

// pad returns n spaces, or none when n is negative.
func pad(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

func evidenceLabel(e diagnosis.Evidence) string {
	if e.Field != "" {
		return e.Field
	}
	return e.Source
}

func (r *textRenderer) section(s diagnosis.Section) {
	r.line(r.style.bold(s.Title))
	keyWidth, valueWidth := 0, 0
	for _, item := range s.Items {
		if n := utf8.RuneCountInString(item.Key); n > keyWidth {
			keyWidth = n
		}
		// Values are only padded when a note follows them, so a plain
		// key/value section keeps no trailing space.
		if n := utf8.RuneCountInString(item.Value); item.Note != "" && n > valueWidth {
			valueWidth = n
		}
	}
	for _, item := range s.Items {
		line := "  " + item.Key + pad(keyWidth-utf8.RuneCountInString(item.Key)) + "  " + r.style.value(item.Value)
		if item.Note != "" {
			line += pad(valueWidth-utf8.RuneCountInString(item.Value)) + "  " + r.style.note(item.Note)
		}
		r.line(line)
	}
	for i, node := range s.Tree {
		if i == 0 {
			r.line("  " + node)
			continue
		}
		r.line("  " + strings.Repeat("   ", i-1) + r.style.treeBranch() + " " + node)
	}
}

func (r *textRenderer) degradations(items []diagnosis.Degradation) {
	r.line(r.style.yellow(r.style.symbolWarn()) + " " + r.style.bold("Diagnosis is incomplete"))
	for _, d := range items {
		r.blank()
		r.paragraph(fmt.Sprintf("KubeWhy could not read %s in this cluster.", d.Resource), "  ")
		if d.RequiredFor != "" {
			r.blank()
			r.line("  Required for")
			r.paragraph(d.RequiredFor, "    ")
		}
		r.blank()
		r.line("  Kubernetes returned")
		r.paragraph(orText(d.Detail, d.Reason), "    ")
	}
}

// bullet writes a bulleted line, indenting continuation lines under the text
// rather than under the bullet.
func (r *textRenderer) bullet(text, indent string) {
	marker := r.style.bullet() + " "
	hanging := indent + strings.Repeat(" ", utf8.RuneCountInString(marker))
	lines := wrap(text, r.style.Width-len(hanging))
	for i, line := range lines {
		if i == 0 {
			r.line(indent + marker + line)
			continue
		}
		r.line(hanging + line)
	}
}

// paragraph writes wrapped text with a fixed indent.
// tone maps a severity onto how loudly the renderer should present it.
func tone(severity diagnosis.Severity) severityTone {
	switch severity {
	case diagnosis.SeverityCritical:
		return toneCritical
	case diagnosis.SeverityWarning:
		return toneWarning
	default:
		return toneInfo
	}
}

func (r *textRenderer) paragraph(text, indent string) {
	for _, l := range wrap(text, r.style.Width-len(indent)) {
		r.line(indent + l)
	}
}

func orText(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}
