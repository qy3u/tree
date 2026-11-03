package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPageReferenceMenuPasteClickAndReopen(t *testing.T) {
	a, tt := commandTestApp(t)
	target := a.current()
	target.page.Title = "学习笔记"
	// An empty page is a valid target, independent of any block ID.
	tt.Frame()
	if err := tt.Click("页面菜单"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("复制页面链接"); err != nil {
		t.Fatal(err)
	}
	link := tt.Clipboard()
	page, block, ok := parseReference(link)
	if !ok || page != target.page.ID || block != "" {
		t.Fatalf("page link: %q", link)
	}
	if a.pageMenu || a.selected != page {
		t.Fatal("copy did not close menu or changed page")
	}
	a.addPage("")
	tt.Frame()
	source := a.current()
	if err := tt.Click("笔记正文"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyV)
	if source.doc.Text().PlainText() != "学习笔记" || source.doc.Text().AttributesAt(0).Link != link {
		t.Fatal("paste did not create page reference")
	}
	source.doc.Undo()
	tt.Frame()
	if source.doc.Text().Len() != 0 {
		t.Fatal("paste not undoable")
	}
	source.doc.Redo()
	tt.Frame()
	target.page.Title = "已重命名"
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
	ed, _ := tt.Find("笔记正文")
	box := a.current().doc.VisibleParagraphs()[0]
	tt.ClickAt(ed.X+12, ed.Y+box.FirstLineCenter)
	tt.Frame()
	if a.selected != page || a.errorOpen {
		t.Fatalf("page click failed: %s", a.failure)
	}
	if a.current().page.Title != "已重命名" || a.blockJump.block != "" {
		t.Fatal("wrong target or block highlight")
	}
	a.navigateTab(-1)
	tt.Frame()
	if a.selected != source.page.ID {
		t.Fatal("cannot return to source")
	}
	if a.current().doc.Text().AttributesAt(0).Link != link {
		t.Fatal("reference lost on reopen")
	}
}

func TestPageReferenceMissingAndRestoredTarget(t *testing.T) {
	a, _ := commandTestApp(t)
	target := a.selected
	link := referenceURL(target, "")
	a.addPage("")
	source := a.selected
	if err := a.store.Trash(target); err != nil {
		t.Fatal(err)
	}
	a.reload()
	if !a.openReference(link) || !a.errorOpen || a.selected != source {
		t.Fatal("deleted target not handled")
	}
	if err := a.store.Restore(target); err != nil {
		t.Fatal(err)
	}
	a.reload()
	a.errorOpen = false
	a.openReference(link)
	if a.errorOpen || a.selected != target {
		t.Fatal("restored link did not work")
	}
	a.selectPage(source)
	a.openReference(referenceURL("missing-page", ""))
	if !a.errorOpen || a.selected != source {
		t.Fatal("missing page navigated")
	}
}
