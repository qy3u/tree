package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCaretPaintExcludesLineLeading(t *testing.T) {
	for _, kind := range []string{"text", "h1", "h2", "code"} {
		t.Run(kind, func(t *testing.T) {
			_, tt := commandTestApp(t)
			tt.SetScale(2)
			if kind != "text" {
				tt.Type("/" + kind)
				tt.Key(0, ui.KeyEnter)
			}
			tt.Move(10, 10)
			tt.Frame()
			caret, ok := tt.TextCaret()
			if !ok {
				t.Fatal("missing native caret")
			}
			img := tt.Image()
			first, last := -1, -1
			for y := int(math.Floor(float64(caret.Y * 2))); y < int(math.Ceil(float64((caret.Y+caret.H)*2))); y++ {
				for x := int(caret.X * 2); x < int((caret.X+caret.W+1)*2); x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					if r < 0x8888 && g < 0x8888 && b < 0x8888 {
						if first < 0 {
							first = y
						}
						last = y
						break
					}
				}
			}
			drawn := float32(last-first+1) / 2
			if first < 0 || drawn > caret.H*.9 || drawn < caret.H*.5 {
				t.Fatalf("painted height %.1f, line height %.1f", drawn, caret.H)
			}
			center := float32(first+last+1) / 4
			if math.Abs(float64(center-caret.Y-caret.H/2)) > 1 {
				t.Fatalf("caret not centered: %.1f vs %.1f", center, caret.Y+caret.H/2)
			}
		})
	}
}
