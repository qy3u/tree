package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func TestEditorExternalLinks(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		name := "page"
		if leaf {
			name = "leaf"
		}
		t.Run(name, func(t *testing.T) {
			var tt *ui.Tester
			var d *draft
			label := "笔记正文"
			if leaf {
				_, tester, _, v := createTestLeaf(t)
				tt, d, label = tester, v.draft, "Leaf 正文"
			} else {
				a, tester := commandTestApp(t)
				tt, d = tester, a.current()
			}
			for _, target := range []string{"https://example.com/procedures.pdf#page=4", "https://example.com/procedures.pdf#page=37", "javascript:alert(1)"} {
				d.doc.Edit(func(e *richtext.Edit) {
					e.ReplaceString(0, e.Text().Len(), "Exchange Procedures", richtext.Attributes{Link: target})
					e.Select(0, 0)
				})
				tt.Frame()
				tt.Frame()
				rect, ok := tt.Find(label)
				if !ok {
					t.Fatal("editor missing")
				}
				// Click inside the first linked word, using the published line geometry.
				p := d.doc.VisibleParagraphs()[0]
				x, y := rect.X+12, rect.Y+p.Top+p.Height/2
				tt.Move(x, y)
				if tt.Cursor() != ui.CursorPointer {
					t.Fatalf("link cursor absent at %v,%v", x, y)
				}
				before := len(tt.OpenedURLs())
				tt.Advance(time.Second) // Keep independent clicks from becoming a double-click selection.
				tt.ClickAt(x, y)
				got := tt.OpenedURLs()
				if ui.SafeToOpen(target) {
					if len(got) != before+1 || got[len(got)-1] != target {
						t.Fatalf("click did not open %q: %v", target, got)
					}
				} else if len(got) != before {
					t.Fatalf("unsafe URL opened: %v", got)
				}
			}
		})
	}
}
