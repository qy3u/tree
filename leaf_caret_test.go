package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLeafBoundaryNeverPaintsInsertionCaret(t *testing.T) {
	for _, collapsed := range []bool{false, true} {
		name := "expanded"
		if collapsed {
			name = "collapsed"
		}
		t.Run(name, func(t *testing.T) {
			a, tt, d, _ := createTestLeaf(t)
			tt.Type("一段知识")
			tt.Key(0, ui.KeyEnter)
			tt.Type("Leaf 正文")
			if collapsed {
				if err := tt.Click("收起 Leaf"); err != nil {
					t.Fatal(err)
				}
			}
			a.focusBlockEditor(d)
			d.doc.SetSelection(0, 0)
			tt.Frame()
			tt.Move(10, 10)
			check := func() {
				t.Helper()
				ed, _ := tt.Find("笔记正文")
				for _, box := range d.doc.VisibleAttachments() {
					if leafID(box.Attachment) == "" {
						continue
					}
					img := tt.Image()
					for _, edge := range []float32{ed.X + box.Rect.X, ed.X + box.Rect.X + box.Rect.W} {
						for x := int(edge) - 1; x <= int(edge)+1; x++ {
							for y := int(ed.Y + box.Rect.Y + 10); y < int(ed.Y+box.Rect.Y+box.Rect.H-10); y++ {
								r, g, b, _ := img.At(x, y).RGBA()
								if r < 0x8888 && g < 0x8888 && b < 0x8888 {
									t.Fatalf("Leaf edge contains caret at %d,%d", x, y)
								}
							}
						}
					}
				}
			}
			check()
			tt.Key(0, ui.KeyRight)
			check()
			tt.Key(0, ui.KeyLeft)
			check()
			clickLeafGrip(t, tt, d)
			tt.Key(0, ui.KeyEscape)
			check()
		})
	}
}
