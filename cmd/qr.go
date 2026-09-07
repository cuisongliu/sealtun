package cmd

import (
	"io"
	"os"

	"github.com/mdp/qrterminal/v3"
	"golang.org/x/term"
)

// printTerminalQR renders content as a compact half-block QR code so a phone
// on the same network can open a tunnel URL straight from the terminal.
func printTerminalQR(w io.Writer, content string) {
	qrterminal.GenerateWithConfig(content, qrterminal.Config{
		Level:      qrterminal.L,
		Writer:     w,
		HalfBlocks: true,
		QuietZone:  2,
	})
}

// qrNarrowTerminalWarning notes when the terminal is probably too narrow for
// the code to render unwrapped; a wrapped QR is unscannable and silently
// wastes the feature for the exact mobile-scan scenario it exists for.
func qrNarrowTerminalWarning(w io.Writer) string {
	file, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return ""
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil || width >= 60 {
		return ""
	}
	return "terminal is narrow; if the code below is unreadable, widen the terminal or open the URL manually"
}
