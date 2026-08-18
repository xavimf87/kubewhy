package output

import (
	"encoding/json"
	"io"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
)

// JSON writes the report as machine-readable output.
//
// The shape is treated as public API: fields are added, not renamed, and rule
// identifiers stay stable so automation can rely on them.
func JSON(w io.Writer, report *diagnosis.Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if report.Diagnoses == nil {
		// An empty array reads better than null for consumers.
		report.Diagnoses = []diagnosis.Diagnosis{}
	}
	return encoder.Encode(report)
}
