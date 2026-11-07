package main

import (
	"testing"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func TestLinkEditTitleSaveCancelAndUndo(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	original := richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{
		{Text: "前文 "}, {Text: "Exchange Procedures", Attributes: richtext.Attributes{Link: "https://example.com/old", Weight: 700}}, {Text: " 后文"},
	}})
	d.doc.SetText(original)
	d.doc.SetSelection(6, 6)
	tt.Frame()
	original = d.doc.Text()
	// Cmd-K at a caret expands to the complete link, just like Edit on hover.
	tt.Key(ui.Cmd, ui.KeyK)
	if !a.linkOpen || a.linkEdit.original != "Exchange Procedures" {
		t.Fatal("caret link not expanded")
	}
	if !tt.Focused("链接文字") {
		t.Fatal("title not focused")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("交易规则 🌲")
	a.linkURL = "https://example.com/new#page=4"
	tt.Key(0, ui.KeyEnter)
	if a.linkOpen {
		t.Fatal("Enter did not close")
	}
	if got := d.doc.Text().String(); got != "前文 交易规则 🌲 后文" {
		t.Fatal(got)
	}
	if attrs := d.doc.Text().AttributesAt(3); attrs.Link != a.linkURL || !attrs.Bold() {
		t.Fatalf("lost link/format: %+v", attrs)
	}
	d.doc.Undo()
	if !d.doc.Text().Equal(original) {
		t.Fatal("URL and title not undone together")
	}
	d.doc.SetSelection(6, 6)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("应该取消")
	tt.Key(0, ui.KeyEscape)
	if a.linkOpen || !d.doc.Text().Equal(original) {
		t.Fatal("Escape changed document")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("点击外部保存")
	tt.ClickAt(1080, 700)
	tt.Frame()
	if a.linkOpen || d.doc.Text().String() != "前文 点击外部保存 后文" {
		t.Fatalf("outside click: %t %q", a.linkOpen, d.doc.Text().String())
	}
}

func TestLinkEditNewLinkAndStaleDocument(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	d.doc.SetText(richtext.NewText("普通文字"))
	d.doc.SetSelection(0, 4)
	tt.Frame()
	a.openLink()
	tt.Frame()
	a.linkURL = "https://example.com"
	tt.Key(0, ui.KeyEnter)
	if d.doc.Text().AttributesAt(0).Link != a.linkURL {
		t.Fatal("new link not applied")
	}
	d.doc.SetSelection(1, 1)
	tt.Frame()
	a.openLink()
	tt.Frame()
	a.linkEdit.title = "过期的文字"
	d.doc.SetText(richtext.NewText("新内容"))
	tt.Frame()
	if a.linkOpen || d.doc.Text().String() != "新内容" {
		t.Fatal("stale edit applied")
	}
}
