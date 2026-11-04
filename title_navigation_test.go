package main

import (
	"testing"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func TestTitleEnterBackspaceRoundTrip(t *testing.T) {
	for _, source := range []string{"", "已有正文", "## 已有标题", "- [x] 已完成", "```go\nfunc main() {}\n```", "> 引用"} {
		t.Run(source, func(t *testing.T) {
			a, tt := commandTestApp(t)
			d := a.current()
			d.doc.SetText(bodyFromMarkdown(source).Text)
			d.doc.IdentifyParagraphs(newID)
			original := d.doc.Text()
			tt.Frame()
			tt.Click("页面标题")
			tt.Type("中文🌲标题")
			for i := range 3 {
				tt.Key(0, ui.KeyEnter)
				if !tt.Focused("笔记正文") {
					t.Fatal("Enter did not focus body")
				}
				if i == 1 {
					tt.Type("新行")
					tt.Key(0, ui.KeyBackspace)
					tt.Key(0, ui.KeyBackspace)
				}
				tt.Key(0, ui.KeyBackspace)
				if !tt.Focused("页面标题") || !d.doc.Text().Equal(original) || d.page.Title != "中文🌲标题" {
					t.Fatal("Enter/Backspace did not restore the title and original blocks")
				}
			}
			tt.Type("末尾")
			if d.page.Title != "中文🌲标题末尾" {
				t.Fatalf("caret not at Unicode title end: %q", d.page.Title)
			}
			if err := a.flush(); err != nil {
				t.Fatal(err)
			}
			reopened, err := newApp(a.store)
			if err != nil {
				t.Fatal(err)
			}
			if !reopened.current().doc.Text().Equal(original) || reopened.current().page.Title != d.page.Title {
				t.Fatal("round trip changed saved blocks or title")
			}
		})
	}
}

func TestTitleBoundaryUndoAndNormalTitleEditing(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	tt.Type("已有正文")
	before := d.doc.Text()
	tt.Click("页面标题")
	tt.Type("原始标题")
	tt.Key(0, ui.KeyEnter)
	inserted := d.doc.Text()
	tt.Key(0, ui.KeyBackspace)
	tt.Key(ui.Cmd, ui.KeyZ)
	if !tt.Focused("笔记正文") || !d.doc.Text().Equal(inserted) || d.page.Title != "原始标题" {
		t.Fatal("immediate undo should restore the deleted block, not undo the title")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	if !d.doc.Text().Equal(before) {
		t.Fatal("redo did not remove the blank block")
	}
	tt.Click("页面标题")
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyBackspace)
	tt.Type("追加")
	tt.Key(ui.Cmd, ui.KeyZ)
	if !tt.Focused("页面标题") || d.page.Title != "原始标题" || !d.doc.Text().Equal(before) {
		t.Fatal("editing the title should restore its own undo handling")
	}
	// The only block must remain available when the page is completely empty.
	d.doc.SetText(richtext.NewText(""))
	tt.Click("页面标题")
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyBackspace)
	if !tt.Focused("页面标题") || d.doc.Text().ParagraphCount() != 1 {
		t.Fatal("empty page lost its editable block")
	}
}

func TestTitleBackspaceLeavesOtherEditingToBody(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
		anchor, caret    int
		key              ui.Key
		mods             ui.Modifiers
	}{
		{"nonempty start", "正文", "正文", 0, 0, ui.KeyBackspace, 0},
		{"delete character", "正文", "正", 2, 2, ui.KeyBackspace, 0},
		{"selection", "正文", "", 0, 2, ui.KeyBackspace, 0},
		{"later empty block", "正文\n", "正文", 3, 3, ui.KeyBackspace, 0},
		{"forward delete", "\n正文", "正文", 0, 0, ui.KeyDelete, 0},
		{"modified backspace", "\n正文", "\n正文", 0, 0, ui.KeyBackspace, ui.Alt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, tt := commandTestApp(t)
			d := a.current()
			d.doc.SetText(richtext.NewText(tc.text))
			d.doc.SetSelection(tc.anchor, tc.caret)
			tt.Frame()
			tt.Key(tc.mods, tc.key)
			if d.doc.Text().String() != tc.want || !tt.Focused("笔记正文") {
				t.Fatalf("normal body editing was intercepted: %q", d.doc.Text().String())
			}
		})
	}
	// An empty styled block first loses its format, then crosses to the title.
	a, tt := commandTestApp(t)
	d := a.current()
	d.doc.SetHeading(2)
	d.doc.SetSelection(0, 0)
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	if !tt.Focused("笔记正文") || d.doc.Formatting().Paragraph.Heading != 0 {
		t.Fatal("styled block should unstyle before crossing to title")
	}
	tt.Key(0, ui.KeyBackspace)
	if !tt.Focused("页面标题") {
		t.Fatal("plain empty block should return to title")
	}
}
