package pairing

import (
	"io"

	"github.com/mdp/qrterminal/v3"
)

// RenderQR writes a terminal-friendly QR code for the payload to w.
// Uses half-block characters: about 2x more compact than ASCII while
// remaining scannable by every Android camera we have tested.
func RenderQR(w io.Writer, payload string) {
	cfg := qrterminal.Config{
		Level:          qrterminal.M,
		Writer:         w,
		HalfBlocks:     true,
		BlackChar:      qrterminal.BLACK_BLACK,
		WhiteBlackChar: qrterminal.WHITE_BLACK,
		BlackWhiteChar: qrterminal.BLACK_WHITE,
		WhiteChar:      qrterminal.WHITE_WHITE,
		QuietZone:      2,
	}
	qrterminal.GenerateWithConfig(payload, cfg)
}
