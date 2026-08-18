//go:build !windows

package output

import "io"

// enableVirtualTerminal is a no-op everywhere except Windows, where consoles
// need ANSI processing turned on explicitly.
func enableVirtualTerminal(io.Writer) bool { return true }
