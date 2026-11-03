package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTitleEnterFocusesBodyWithoutChangingContent(t *testing.T) {
	for _, body := range []string{"", "原有正文"} {
		t.Run(body, func(t *testing.T) {
			a, tt := commandTestApp(t)
			d := a.current()
			tt.Type(body)
			d.doc.SetSelection(0, d.doc.Text().Len())
			if err := tt.Click("页面标题"); err != nil {
				t.Fatal(err)
			}
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type("新标题")
			tt.Key(0, ui.KeyEnter)
			if !tt.Focused("笔记正文") {
				t.Fatal("Enter did not focus the body")
			}
			if d.doc.Text().PlainText() != body || d.page.Title != "新标题" {
				t.Fatal("Enter modified title or body content")
			}
			tt.Type("继续输入")
			if d.doc.Text().PlainText() != "继续输入"+body {
				t.Fatal("typing did not start at the body without replacing existing content")
			}
		})
	}
}

func TestNativeViewCreateEditSwitchAndSave(t *testing.T) {
	s := testStore(t)
	a, err := newApp(s)
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.view, 1120, 800)
	if err = tt.Click("新建页面"); err != nil {
		t.Fatal(err)
	}
	d := a.current()
	id := d.page.ID
	if err = tt.Click("页面标题"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("中文测试页")
	if err = tt.Click("笔记正文"); err != nil {
		t.Fatal(err)
	}
	tt.Type("测试文字")
	tt.Key(ui.Cmd, ui.KeyA)
	if err = tt.Click("加粗 · ⌘B"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !strings.Contains(d.doc.Text().Markdown(), "**测试文字**") {
		t.Fatalf("format: %s", d.doc.Text().Markdown())
	}
	if err = tt.Click("页面菜单"); err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("＋ 子页面"); err != nil {
		t.Fatal(err)
	}
	if a.current().page.Parent != id {
		t.Fatal("wrong parent")
	}
	a.selectPage(id)
	tt.Frame()
	if a.current().doc != d.doc {
		t.Fatal("page switch discarded undo state")
	}
	a.current().doc.Undo()
	tt.Frame()
	if strings.Contains(d.doc.Text().Markdown(), "**") {
		t.Fatal("undo failed")
	}
	if err = a.flush(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newApp(s)
	if err != nil {
		t.Fatal(err)
	}
	reopened.selectPage(id)
	if reopened.current().page.Title != "中文测试页" || reopened.current().doc.Text().PlainText() != "测试文字" {
		t.Fatal("saved content mismatch")
	}
}
func TestAttachmentInsertionUndoAndCrossPageTable(t *testing.T) {
	a, err := newApp(testStore(t))
	if err != nil {
		t.Fatal(err)
	}
	a.addPage("")
	source := a.current()
	a.table = Table{Cells: [][]string{{"表头"}, {"内容"}}}
	a.tablePage = source.page.ID
	a.tableStart = -1
	a.saveTable()
	before := source.doc.Text()
	source.doc.Undo()
	if source.doc.Text().PlainText() != "" {
		t.Fatal("attachment insertion is not one undo step")
	}
	source.doc.Redo()
	a.addPage("")
	dest := a.current()
	dest.doc.Insert(before)
	a.changed(dest)
	if len(dest.page.Body.Tables) != 1 {
		t.Fatalf("table missing after copy across pages: before=%s source=%+v dest=%s", before.Markdown(), source.page.Body.Tables, dest.doc.Text().Markdown())
	}
	if err = a.flush(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dest.page.Body.Markdown(), "表头") {
		t.Fatal("copied table export missing")
	}
}
func TestSlashMenu(t *testing.T) {
	a, err := newApp(testStore(t))
	if err != nil {
		t.Fatal(err)
	}
	a.addPage("")
	tt := ui.NewTester(a.view, 1120, 800)
	if err = tt.Click("笔记正文"); err != nil {
		t.Fatal(err)
	}
	tt.Type("/")
	tt.Frame()
	if a.slash == nil {
		t.Fatal("slash did not open menu")
	}
	if err = tt.Click("命令：二级标题"); err != nil {
		t.Fatal(err)
	}
	tt.Type("标题")
	if a.current().doc.Formatting().Paragraph.Heading != 2 || strings.Contains(a.current().doc.Text().PlainText(), "/") {
		t.Fatal("slash command failed")
	}
}
func TestSaveFailureRetainsDraft(t *testing.T) {
	a, err := newApp(testStore(t))
	if err != nil {
		t.Fatal(err)
	}
	d := a.current()
	copy := *d.page
	if err = a.store.Save(&copy); err != nil {
		t.Fatal(err)
	}
	d.doc.InsertText("尚未保存")
	a.changed(d)
	if err = a.flush(); err == nil {
		t.Fatal("conflicting save accepted")
	}
	if !d.dirty || !strings.Contains(d.doc.Text().PlainText(), "尚未保存") {
		t.Fatal("unsaved draft lost")
	}
}

func TestCompositionCommitsOnceAndUndo(t *testing.T) {
	a, err := newApp(testStore(t))
	if err != nil {
		t.Fatal(err)
	}
	a.addPage("")
	tt := ui.NewTester(a.view, 1120, 800)
	if err = tt.Click("笔记正文"); err != nil {
		t.Fatal(err)
	}
	tt.Compose("zhongwen", 8)
	if a.current().doc.Text().PlainText() != "" {
		t.Fatal("uncommitted composition leaked into document")
	}
	tt.Type("中文")
	tt.Compose("", 0)
	if got := a.current().doc.Text().PlainText(); got != "中文" {
		t.Fatalf("committed text = %q", got)
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if a.current().doc.Text().PlainText() != "" {
		t.Fatal("composition did not undo together")
	}
}

func TestTableEditUndoKeepsOriginalCells(t *testing.T) {
	a, err := newApp(testStore(t))
	if err != nil {
		t.Fatal(err)
	}
	a.addPage("")
	a.openTable()
	a.table.Cells[0][0] = "原表头"
	a.saveTable()
	d := a.current()
	d.doc.SetSelection(0, 1)
	a.openTable()
	if a.tableStart != 0 {
		t.Fatal("existing table not found")
	}
	a.table.Cells[0][0] = "新表头"
	a.saveTable()
	d.page.Body.Text = d.doc.Text()
	if !strings.Contains(d.page.Body.Markdown(), "新表头") {
		t.Fatal("edit not applied")
	}
	d.doc.Undo()
	d.page.Body.Text = d.doc.Text()
	if !strings.Contains(d.page.Body.Markdown(), "原表头") {
		t.Fatal("undo lost original table cells")
	}
}
