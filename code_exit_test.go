package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestClickBelowLastCodeCreatesPlainBlock(t *testing.T) {
	for _, source := range []string{"", "fn test() {\u2028    // 中文\u2028}"} {
		for _, distance := range []float32{2, 140} {
			t.Run(fmt.Sprintf("%d_runes_%g_below", len([]rune(source)), distance), func(t *testing.T) {
				a, tt := commandTestApp(t)
				tt.Type("/code")
				tt.Key(0, ui.KeyEnter)
				tt.Type(source)
				doc := a.current().doc
				box := doc.VisibleCodeBlocks()[0]
				setCodeLanguage(a.current(), box, "rust")
				tt.Frame()
				original := doc.Text().Markdown()
				editor, _ := tt.Find("笔记正文")
				box = doc.VisibleCodeBlocks()[0]
				x, y := editor.X+80, editor.Y+box.Rect.Y+box.Rect.H+distance
				tt.ClickAt(x, y)
				if doc.Text().ParagraphCount() != 2 || doc.Formatting().Paragraph.CodeBlock || doc.Formatting().Paragraph.CodeLanguage != "" {
					t.Fatal("click did not create a plain block")
				}
				if !tt.Focused("笔记正文") {
					t.Fatal("click did not focus editor")
				}
				if anchor, caret := doc.Selection(); anchor != doc.Text().Len() || caret != anchor {
					t.Fatal("caret not at new block")
				}
				// A second click in the same blank space must not keep appending blocks.
				tt.ClickAt(x, y)
				if doc.Text().ParagraphCount() != 2 {
					t.Fatal("repeated click created extra blocks")
				}
				doc.Undo()
				tt.Frame()
				if doc.Text().Markdown() != original {
					t.Fatal("undo did not restore original code")
				}
				doc.Redo()
				tt.Frame()
				tt.Type("代码下面的正文")
				if got := doc.Text().String(); got != source+"\n代码下面的正文" {
					t.Fatalf("typing went to wrong block: %q", got)
				}
				if err := a.flush(); err != nil {
					t.Fatal(err)
				}
				reopened, err := newApp(a.store)
				if err != nil {
					t.Fatal(err)
				}
				reopened.selectPage(a.selected)
				text := reopened.current().doc.Text()
				if text.ParagraphCount() != 2 || text.ParagraphStyleAt(text.Len()).CodeBlock || text.ParagraphStyleAt(0).CodeLanguage != "rust" {
					t.Fatal("saved block styles changed")
				}
			})
		}
	}
}

func TestClickInsideCodeAndSelectionDragDoNotAppend(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("/code")
	tt.Key(0, ui.KeyEnter)
	tt.Type("first")
	tt.Key(0, ui.KeyEnter)
	tt.Type("last")
	doc := a.current().doc
	editor, _ := tt.Find("笔记正文")
	box := doc.VisibleCodeBlocks()[0]
	first := doc.VisibleParagraphs()[0]
	tt.ClickAt(editor.X+50, editor.Y+first.FirstLineCenter)
	if doc.Text().ParagraphCount() != 1 {
		t.Fatal("click inside code appended a block")
	}
	tt.Press(editor.X+15, editor.Y+first.FirstLineCenter)
	tt.Move(editor.X+90, editor.Y+box.Rect.Y+box.Rect.H+80)
	tt.Release(editor.X+90, editor.Y+box.Rect.Y+box.Rect.H+80)
	if doc.Text().ParagraphCount() != 1 {
		t.Fatal("selection drag appended a block")
	}
}
