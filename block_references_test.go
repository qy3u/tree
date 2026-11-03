package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func TestBlockReferenceCopyPasteClickAndReopen(t *testing.T) {
	a, tt := commandTestApp(t)
	target := a.current()
	target.page.Title = "Go 学习"
	tt.Type("并发示例")
	if err := tt.Click("移动段落 1"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("复制块链接"); err != nil {
		t.Fatal(err)
	}
	link := tt.Clipboard()
	page, block, ok := parseReference(link)
	if !ok || page != target.page.ID || block != target.doc.Text().ParagraphStyleAt(0).ID {
		t.Fatalf("bad copied link: %q", link)
	}
	a.addPage("")
	tt.Frame()
	source := a.current()
	if err := tt.Click("笔记正文"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyV)
	if got := source.doc.Text().PlainText(); got != "并发示例" {
		t.Fatalf("paste: %q", got)
	}
	if source.doc.Text().AttributesAt(0).Link != link {
		t.Fatal("paste is not linked")
	}
	source.doc.Undo()
	tt.Frame()
	if source.doc.Text().Len() != 0 {
		t.Fatal("reference paste is not one undo step")
	}
	source.doc.Redo()
	tt.Frame()
	// Move target far down after copying: offsets must not serve as anchors.
	target.doc.Edit(func(e *richtext.Edit) {
		e.ReplaceString(0, 0, strings.Repeat("前面的内容\n", 60), richtext.Attributes{})
	})
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	a = reopened
	tt = ui.NewTester(a.view, 1120, 800)
	tt.Frame()
	source = a.current()
	ed, _ := tt.Find("笔记正文")
	para := source.doc.VisibleParagraphs()[0]
	tt.ClickAt(ed.X+20, ed.Y+para.FirstLineCenter)
	tt.Frame()
	if a.selected != page {
		t.Fatalf("click did not navigate: %s, error %s", a.selected, a.failure)
	}
	d := a.current()
	pos, found := blockPosition(d.doc.Text(), block)
	if !found {
		t.Fatal("target ID did not survive save")
	}
	anchor, caret := d.doc.Selection()
	if anchor != pos || caret != pos {
		t.Fatalf("did not locate target: %d %d want %d", anchor, caret, pos)
	}
	if d.doc.ScrollOffset() <= 0 || !tt.Focused("笔记正文") {
		t.Fatal("target not scrolled/focused")
	}
	visible := false
	for _, box := range d.doc.VisibleParagraphs() {
		if box.Start == pos {
			visible = true
		}
	}
	if !visible || a.blockJump.block != block {
		t.Fatal("target not visible/highlighted")
	}
	tt.Advance(2 * time.Second)
	if a.blockJump.block != "" {
		t.Fatal("highlight did not clear")
	}
	if source.doc.Text().AttributesAt(0).Link != link {
		t.Fatal("link lost on reopen")
	}
}

func TestBlockReferenceIdentityEditingMovingAndDeletion(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	tt.Type("第一块\n目标块\n最后块")
	link, err := a.copyBlockLink(d, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, id, _ := parseReference(link)
	// Reorder, duplicate, format and edit without changing the original ID.
	moveParagraph(d.doc, 1, 0)
	editBlock(d.doc, 0, "duplicate")
	if d.doc.Text().ParagraphStyleAt(0).ID != id {
		t.Fatal("move changed ID")
	}
	count := 0
	for _, p := range d.doc.Text().Paragraphs() {
		if p.Style.ID == id {
			count++
		}
	}
	if count != 1 {
		t.Fatal("duplicate reused reference ID")
	}
	d.doc.SetSelection(0, 0)
	a.runBlockCommand(d, blockCommands[1], false)
	d.doc.Edit(func(e *richtext.Edit) { e.ReplaceString(1, 1, "新", richtext.Attributes{}) })
	if !a.openReference(link) || a.errorOpen {
		t.Fatal(a.failure)
	}
	if caret, _ := d.doc.Selection(); caret != 0 {
		t.Fatal("reference followed wrong copy")
	}
	editBlock(d.doc, 0, "delete")
	a.errorOpen = false
	a.openReference(link)
	if !a.errorOpen || !strings.Contains(a.failure, "内容块已删除") {
		t.Fatal("missing block was not reported")
	}
	d.doc.Undo()
	a.errorOpen = false
	a.openReference(link)
	if a.errorOpen {
		t.Fatal("undo did not restore reference")
	}
}

func TestBlockReferenceOldNoteIDsAndMissingPage(t *testing.T) {
	a, _ := commandTestApp(t)
	p, err := a.store.Create("", "旧笔记", bodyFromMarkdown("没有 ID 的段落"))
	if err != nil {
		t.Fatal(err)
	}
	a.reload()
	a.selectPage(p.ID)
	d := a.current()
	link, err := a.copyBlockLink(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	b.openReference(link)
	if b.errorOpen {
		t.Fatal("copy did not persist IDs", b.failure)
	}
	a.addPage("")
	original := a.selected
	if err := a.store.Trash(p.ID); err != nil {
		t.Fatal(err)
	}
	a.reload()
	a.openReference(link)
	if !a.errorOpen || a.selected != original {
		t.Fatal("deleted page reference changed navigation")
	}
	for _, bad := range []string{"tree://page/", "tree://other/page#id", "tree://page/a/b#id"} {
		a.errorOpen = false
		if !a.openReference(bad) || !a.errorOpen {
			t.Fatalf("invalid URL not handled: %q", bad)
		}
	}
	if a.openReference("https://example.com") {
		t.Fatal("external link consumed")
	}
}

func TestBlockReferencePastePreservesStyleAndCodeLiteral(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	tt.Type("引用目标")
	link, err := a.copyBlockLink(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	a.addPage("")
	tt.Frame()
	tt.Click("笔记正文")
	tt.Type("/todo")
	tt.Key(0, ui.KeyEnter)
	tt.SetClipboard(link)
	tt.Key(ui.Cmd, ui.KeyV)
	d = a.current()
	if d.doc.Text().ParagraphStyleAt(0).List != richtext.Tasks || d.doc.Text().AttributesAt(0).Link != link {
		t.Fatal("paste lost task style/reference")
	}
	a.addPage("")
	tt.Frame()
	tt.Click("笔记正文")
	tt.Type("/code")
	tt.Key(0, ui.KeyEnter)
	tt.SetClipboard(link)
	tt.Key(ui.Cmd, ui.KeyV)
	d = a.current()
	if d.doc.Text().PlainText() != link || d.doc.Text().AttributesAt(0).Link != "" {
		t.Fatal("code paste should remain literal")
	}
	a.addPage("")
	tt.Frame()
	tt.Click("笔记正文")
	tt.SetClipboard(link)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyV)
	if a.current().doc.Text().PlainText() != link {
		t.Fatal("paste and match style should remain literal")
	}
}
