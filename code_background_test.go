package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFirstCodeBlockBackgroundExcludesPageHeading(t *testing.T) {
	for _, children := range []int{0, 2} {
		t.Run(fmt.Sprintf("children_%d", children), func(t *testing.T) {
			a, tt := commandTestApp(t)
			a.current().page.Title = "页面标题"
			for range children {
				if _, err := a.store.Create(a.selected, "子页面", emptyBody()); err != nil {
					t.Fatal(err)
				}
			}
			if children > 0 {
				a.report(a.reload())
			}
			tt.SetScale(1)
			tt.Frame()
			tt.Click("笔记正文")
			tt.Type("/code")
			tt.Key(0, ui.KeyEnter)
			check := func() {
				t.Helper()
				tt.Move(10, 10)
				tt.Frame()
				editor, _ := tt.Find("笔记正文")
				boxes := a.current().doc.VisibleParagraphs()
				if len(boxes) != 1 || !a.current().doc.Text().ParagraphStyleAt(0).CodeBlock {
					t.Fatal("expected one code block")
				}
				img := tt.Image()
				x := int(editor.X + editor.W - 24)
				textTop := editor.Y + boxes[0].Top
				// The title, child-page rows and their spacing must all stay white.
				for y := int(editor.Y + 2); y < int(editor.Y+a.current().doc.VisibleCodeBlocks()[0].Rect.Y); y++ {
					r, g, b, _ := img.At(x, y).RGBA()
					if r != 65535 || g != 65535 || b != 65535 {
						t.Fatalf("code background leaked above body at y=%d (text top %.1f): %x/%x/%x", y, textTop, r, g, b)
					}
				}
				r, g, b, _ := img.At(x, int(textTop+5)).RGBA()
				if r == 65535 && g == 65535 && b == 65535 {
					t.Fatal("code block lost its background")
				}
			}
			check()
			editor, _ := tt.Find("笔记正文")
			first := a.current().doc.VisibleParagraphs()[0]
			tt.ClickAt(editor.X+30, editor.Y+first.FirstLineCenter)
			tt.Type("fn test() {")
			tt.Key(0, ui.KeyEnter)
			tt.Type("}")
			check()
		})
	}
}
