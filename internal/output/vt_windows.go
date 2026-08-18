//go:build windows

package output

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// enableVirtualTerminal turns on ANSI processing for a Windows console.
//
// Windows consoles print escape sequences literally until this is enabled, so
// without it coloured output is worse than no colour at all. Consoles too old
// to support it fail here, and colour is then switched off.
func enableVirtualTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	handle := windows.Handle(f.Fd())

	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		// Not a console at all, such as a pipe or a file. Sequences pass
		// through unchanged, which is what --color=always asks for.
		return true
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
