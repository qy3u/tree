package main

import (
	"testing"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func TestTitleEnterPrependsPlainBlock(t *testing.T) {
	for _, source := range []string{
		"第一行已有内容\n\n后面的内容",
		"## 已有标题\n\n正文",
		"- [x] 已完成任务",
		"```go\nfunc main() {\n}\n```",
		"> **引用**与[链接](https://example.com)",
	} {
		t.Run(source, func(t *testing.T) {
			a, tt := commandTestApp(t)
			d := a.current()
			d.page.Title = "测试标题"
			d.doc.SetText(bodyFromMarkdown(source).Text)
			d.doc.IdentifyParagraphs(newID)
			before := d.doc.Text()
			tt.Frame()
			if err := tt.Click("页面标题"); err != nil {
				t.Fatal(err)
			}
			tt.Key(ui.Cmd, ui.KeyRight)
			tt.Key(0, ui.KeyEnter)
			after := d.doc.Text()
			if after.ParagraphCount() != before.ParagraphCount()+1 || after.String() != "\n"+before.String() {
				t.Fatalf("Enter did not prepend a block: %q", after.String())
			}
			st := after.ParagraphStyleAt(0)
			if st.ID == "" || st.ID == before.ParagraphStyleAt(0).ID {
				t.Fatal("new block needs its own stable ID")
			}
			st.ID = ""
			if st != (richtext.ParagraphStyle{}) || !after.Slice(1, after.Len()).Equal(before) {
				t.Fatal("new block inherited formatting or existing blocks changed")
			}
			if anchor, caret := d.doc.Selection(); anchor != 0 || caret != 0 || !tt.Focused("笔记正文") {
				t.Fatal("caret did not move into the new first block")
			}
			tt.Key(ui.Cmd, ui.KeyZ)
			if !d.doc.Text().Equal(before) {
				t.Fatal("undo did not restore the original document")
			}
			tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
			if !d.doc.Text().Equal(after) {
				t.Fatal("redo did not restore the new block and its ID")
			}
			tt.Type("新的一行")
			if d.doc.Text().String() != "新的一行\n"+before.String() || d.page.Title != "测试标题" {
				t.Fatal("typing entered the title or existing first block")
			}
			if err := a.flush(); err != nil {
				t.Fatal(err)
			}
			reopened, err := newApp(a.store)
			if err != nil {
				t.Fatal(err)
			}
			if !reopened.current().doc.Text().Equal(d.doc.Text()) {
				t.Fatal("prepended block or original content changed on reopen")
			}
		})
	}
}

func TestTitleEnterReusesEmptyFirstBlock(t *testing.T) {
	for _, content := range []string{"", "\n后面的内容"} {
		a, tt := commandTestApp(t)
		d := a.current()
		d.doc.SetText(richtext.NewText(content))
		d.doc.IdentifyParagraphs(newID)
		before := d.doc.Text()
		tt.Frame()
		for range 2 {
			tt.Click("页面标题")
			tt.Key(0, ui.KeyEnter)
			if !d.doc.Text().Equal(before) || !tt.Focused("笔记正文") {
				t.Fatal("Enter should reuse an existing empty first block")
			}
		}
	}
}
