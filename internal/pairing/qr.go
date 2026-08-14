package pairing

import (
	"io"

	"github.com/mdp/qrterminal/v3"
)

type QROptions struct {
	// ASCII draws one full block per module instead of the compact half-block
	// layout, for terminals whose font renders half blocks poorly.
	ASCII bool
	// Invert lights up the dark modules instead of the light ones, for
	// terminals with a light background.
	Invert bool
	// Sixel draws the code as an image rather than out of characters. It takes
	// precedence over the other two, and only works on terminals that
	// implement the sixel protocol.
	Sixel bool
}

// RenderQR writes a terminal-friendly QR code for the payload to w.
// The default is half-block characters: about 2x more compact than full
// blocks while remaining scannable by every Android camera we have tested.
func RenderQR(w io.Writer, payload string, opts QROptions) {
	cfg := qrterminal.Config{
		Level:     qrterminal.M,
		Writer:    w,
		QuietZone: 2,
	}

	if opts.Sixel {
		// Sixel carries its own black-on-white palette, so neither the block
		// characters nor Invert apply to it.
		cfg.WithSixel = true
		qrterminal.GenerateWithConfig(payload, cfg)
		return
	}

	// Half-block cells are named <top>_<bottom>. The defaults light up the
	// light modules and leave the dark ones as background, which is what a
	// terminal with a dark background needs; inverting swaps the two.
	dark, light := qrterminal.BLACK_BLACK, qrterminal.WHITE_WHITE
	darkLight, lightDark := qrterminal.BLACK_WHITE, qrterminal.WHITE_BLACK
	if opts.Invert {
		dark, light = light, dark
		darkLight, lightDark = lightDark, darkLight
	}

	if opts.ASCII {
		// Full blocks spend one cell per module, and a terminal cell is about
		// half as wide as it is tall. Doubling every module keeps the code
		// square; a single character per module renders it twice as tall as
		// it is wide, which cameras struggle to read.
		cfg.BlackChar = dark + dark
		cfg.WhiteChar = light + light
		qrterminal.GenerateWithConfig(payload, cfg)
		return
	}

	cfg.HalfBlocks = true
	cfg.BlackChar = dark
	cfg.WhiteChar = light
	cfg.BlackWhiteChar = darkLight
	cfg.WhiteBlackChar = lightDark
	qrterminal.GenerateWithConfig(payload, cfg)
}
