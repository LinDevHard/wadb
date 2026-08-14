package pairing

import (
	"strings"
	"testing"
)

const testPayload = "WIFI:T:ADB;S:studio-abcdefghij;P:0123456789ab;;"

func renderQR(t *testing.T, opts QROptions) string {
	t.Helper()
	var buf strings.Builder
	RenderQR(&buf, testPayload, opts)
	return buf.String()
}

func TestRenderQRASCIIFallbackDiffersFromHalfBlocks(t *testing.T) {
	halfBlocks := renderQR(t, QROptions{})
	ascii := renderQR(t, QROptions{ASCII: true})

	if halfBlocks == "" || ascii == "" {
		t.Fatal("a render produced no output")
	}
	if halfBlocks == ascii {
		t.Fatal("ASCII render matched the half-block one")
	}
	if strings.ContainsAny(ascii, "▀▄") {
		t.Fatal("ASCII render contains half-block characters")
	}
}

func TestRenderQRInvertSwapsDarkAndLight(t *testing.T) {
	normal := renderQR(t, QROptions{})
	inverted := renderQR(t, QROptions{Invert: true})

	if normal == inverted {
		t.Fatal("inverted render is identical to the normal one")
	}

	swap := strings.NewReplacer(" ", "█", "█", " ", "▀", "▄", "▄", "▀")
	if got := swap.Replace(normal); got != inverted {
		t.Fatal("inverted render is not the exact dark/light swap of the normal one")
	}
}

func TestRenderQRKeepsModulesSquare(t *testing.T) {
	tests := []struct {
		name string
		opts QROptions
	}{
		{name: "half blocks", opts: QROptions{}},
		{name: "half blocks inverted", opts: QROptions{Invert: true}},
		{name: "full blocks", opts: QROptions{ASCII: true}},
		{name: "full blocks inverted", opts: QROptions{ASCII: true, Invert: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := qrLines(renderQR(t, tt.opts))
			if len(lines) == 0 {
				t.Fatal("render produced no QR lines")
			}

			// Measured in screen units, where a terminal cell is one wide and
			// two tall. Both layouts should come out roughly square: anything
			// near 0.5 or 2.0 is a code stretched along one axis, which
			// cameras fail to read.
			width := len([]rune(lines[0]))
			height := 2 * len(lines)
			ratio := float64(width) / float64(height)
			if ratio < 0.8 || ratio > 1.25 {
				t.Fatalf("width %d over height %d gives ratio %.2f, want roughly square", width, height, ratio)
			}
		})
	}
}

func TestRenderQRSixelWritesAnImage(t *testing.T) {
	out := renderQR(t, QROptions{Sixel: true})

	if !strings.HasPrefix(out, "\x1bPq") {
		t.Fatalf("sixel render does not start with the sixel introducer: %.8q", out)
	}
	if strings.ContainsAny(out, "█▀▄") {
		t.Fatal("sixel render contains block characters")
	}
}

func TestRenderQRSixelIgnoresBlockOptions(t *testing.T) {
	plain := renderQR(t, QROptions{Sixel: true})
	withOthers := renderQR(t, QROptions{Sixel: true, ASCII: true, Invert: true})

	if plain != withOthers {
		t.Fatal("ASCII or Invert changed the sixel render")
	}
}

// qrLines returns the rendered lines that carry the code itself, dropping the
// blank quiet-zone rows above and below it.
func qrLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if strings.ContainsAny(line, "█▀▄") {
			lines = append(lines, line)
		}
	}
	return lines
}
