package main

import (
	"fmt"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

func TestPageCommandNavigationDeleteRestore(t *testing.T) {
	a, tt := commandTestApp(t)
	parent := a.selected
	tt.Type("/page")
	tt.Key(0, ui.KeyEnter)
	child := a.current()
	if child.page.Parent != parent {
		t.Fatal("page command did not create child")
	}
	a.selectPage(parent)
	tt.Frame()
	if strings.Contains(a.current().doc.Text().PlainText(), "/page") {
		t.Fatal("command remains in parent")
	}
	if err := tt.Click("打开子页面：无标题"); err != nil {
		t.Fatal(err)
	}
	a.deletePage(child.page.ID)
	if a.selected != parent || a.find(child.page.ID).Deleted == "" {
		t.Fatal("deletion did not return to parent")
	}
	a.mutate(func() error { return a.store.Restore(child.page.ID) })
	if a.find(child.page.ID).Parent != parent || a.find(child.page.ID).Deleted != "" {
		t.Fatal("restore lost hierarchy")
	}
}
func TestBlockOperationsAndUndo(t *testing.T) {
	for _, action := range []string{"delete", "duplicate", "before", "after"} {
		t.Run(action, func(t *testing.T) {
			doc := richtext.NewDocument(richtext.ParseMarkdown("# 标题\n\n- [x] 任务\n\n末段"))
			before := doc.Text().Markdown()
			editBlock(doc, 1, action)
			if doc.Text().Markdown() == before {
				t.Fatal("block operation did nothing")
			}
			doc.Undo()
			if doc.Text().Markdown() != before {
				t.Fatal("undo lost block structure")
			}
		})
	}
}
func TestEmptyBlockStylesSurviveRestart(t *testing.T) {
	for _, style := range []richtext.ParagraphStyle{{List: richtext.Tasks, Indent: 2}, {Heading: 2}, {Quote: true}, {CodeBlock: true}} {
		s := testStore(t)
		a, _ := newApp(s)
		a.addPage("")
		id := a.selected
		d := a.current()
		d.doc.Edit(func(e *richtext.Edit) { e.FormatParagraphs(0, 0, func(st *richtext.ParagraphStyle) { *st = style }) })
		a.changed(d)
		if err := a.flush(); err != nil {
			t.Fatal(err)
		}
		reopened, err := newApp(s)
		if err != nil {
			t.Fatal(err)
		}
		reopened.selectPage(id)
		got := reopened.current().doc.Text().ParagraphStyleAt(0)
		got.ID = ""
		if got != style {
			t.Fatalf("empty style lost: %+v want %+v", got, style)
		}
	}
}
func TestNativeParagraphKeys(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("第一行")
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("第二行")
	if a.current().doc.Text().ParagraphCount() != 1 {
		t.Fatal("shift-enter created a block")
	}
	tt.Key(0, ui.KeyTab)
	if a.current().doc.Formatting().Paragraph.Indent != 1 {
		t.Fatal("tab did not indent")
	}
	tt.Key(ui.Shift, ui.KeyTab)
	if a.current().doc.Formatting().Paragraph.Indent != 0 {
		t.Fatal("shift-tab did not outdent")
	}
}
func TestPageScrollMovesTitleWithBody(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	d.doc.InsertText(strings.Repeat("长文段落\n", 100))
	d.doc.SetSelection(0, 0)
	tt.Frame()
	before, _ := tt.Find("页面标题")
	editor, _ := tt.Find("笔记正文")
	tt.Scroll(editor.X+100, editor.Y+300, 0, 200)
	after, _ := tt.Find("页面标题")
	if after.Y >= before.Y-100 {
		t.Fatalf("title stayed fixed: %v -> %v", before.Y, after.Y)
	}
}
func TestInlineTableEditUndo(t *testing.T) {
	a, tt := commandTestApp(t)
	a.openTable()
	a.table.Cells[0][0] = "原值"
	a.saveTable()
	tt.Frame()
	d := a.current()
	pos := -1
	for _, b := range d.doc.VisibleAttachments() {
		if strings.HasPrefix(b.Attachment.URL, "table:") {
			pos = b.Start
		}
	}
	if pos < 0 {
		t.Fatal("table invisible")
	}
	table := Table{Cells: [][]string{{"新值", "标题"}, {"内容", ""}}}
	a.updateInlineTable(d, pos, table)
	if !strings.Contains((Body{Text: d.doc.Text(), Tables: d.page.Body.Tables}).Markdown(), "新值") {
		t.Fatal("inline change not applied")
	}
	d.doc.Undo()
	if !strings.Contains((Body{Text: d.doc.Text(), Tables: d.page.Body.Tables}).Markdown(), "原值") {
		t.Fatal("undo lost cells")
	}
}
func TestSearchOpensMatchingText(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("开头\n要找的内容\n结尾")
	id := a.selected
	a.addPage("")
	a.openSearchResult(id, "内容")
	start, end := a.current().doc.Selection()
	if a.current().doc.Text().Slice(start, end).PlainText() != "内容" {
		t.Fatal("search did not select match")
	}
}
func TestPagePlaceAndCycle(t *testing.T) {
	s := testStore(t)
	a, _ := s.Create("", "甲", emptyBody())
	b, _ := s.Create("", "乙", emptyBody())
	c, _ := s.Create(a.ID, "子", emptyBody())
	if err := s.Place(a.ID, c.ID, false); err == nil {
		t.Fatal("cycle accepted")
	}
	if err := s.Place(c.ID, b.ID, false); err != nil {
		t.Fatal(err)
	}
	pages, _ := s.List()
	roots := sortedChildren(pages, "")
	if len(roots) != 3 || roots[1].ID != c.ID {
		t.Fatal("relative placement failed")
	}
}

func TestSidebarCreateAndContextDelete(t *testing.T) {
	a, tt := commandTestApp(t)
	parent := a.selected
	if err := tt.Click("新建子页面：无标题"); err != nil {
		t.Fatal(err)
	}
	child := a.current().page
	if child.Parent != parent {
		t.Fatal("sidebar plus did not open child")
	}
	// The explicit menu button operates on this page without requiring navigation.
	if err := tt.Click("页面操作：无标题"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("删除页面"); err != nil {
		t.Fatal(err)
	}
	if a.find(child.ID).Deleted == "" {
		t.Fatal("sidebar delete did not trash child")
	}
}
func TestInlineTableNativeCellTyping(t *testing.T) {
	a, tt := commandTestApp(t)
	a.openTable()
	a.saveTable()
	tt.Frame()
	d := a.current()
	var start int
	for _, b := range d.doc.VisibleAttachments() {
		start = b.Start
		break
	}
	label := fmt.Sprintf("表格 %d 第 1 行第 1 列", start)
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	tt.Type("单元格内容")
	if !strings.Contains((Body{Text: d.doc.Text(), Tables: d.page.Body.Tables}).Markdown(), "单元格内容") {
		t.Fatal("native cell input not committed")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	tt.Frame()
	if strings.Contains((Body{Text: d.doc.Text(), Tables: d.page.Body.Tables}).Markdown(), "单元格内容") {
		t.Fatal("cell undo not integrated")
	}
}

func TestScrollingOverTitleSharesPageScroll(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	d.doc.InsertText(strings.Repeat("内容\n", 100))
	d.doc.SetSelection(0, 0)
	tt.Frame()
	title, _ := tt.Find("页面标题")
	tt.Scroll(title.X+30, title.Y+20, 0, 120)
	if d.doc.ScrollOffset() < 100 {
		t.Fatal("wheel over title did not scroll page")
	}
}
