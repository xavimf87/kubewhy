// Package format holds the small presentation helpers shared by rules and
// report builders, so both describe the same fact the same way.
package format

import (
	"fmt"
	"time"
)

// Duration renders a duration the way kubectl does: coarse, short and
// readable, e.g. "12m", "3h14m", "2d3h".
func Duration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		hours := int(d.Hours())
		minutes := int(d.Minutes()) - hours*60
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, minutes)
	default:
		days := int(d.Hours()) / 24
		hours := int(d.Hours()) % 24
		if hours == 0 || days > 7 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd%dh", days, hours)
	}
}

// Count renders a count with a singular or plural noun.
func Count(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}
