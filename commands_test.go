package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func commandTestApp(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	a, err := newApp(testStore(t))
	if err != nil {
		t.Fatal(err)
	}
	a.addPage("")
	tt := ui.NewTester(a.view, 1120, 800)
	if err = tt.Click("笔记正文"); err != nil {
		t.Fatal(err)
	}
	return a, tt
}

func TestSlashTodoLiveFilterAndEnter(t *testing.T) {
	a, tt := commandTestApp(t)
	for _, char := range "/todo" {
		tt.Type(string(char))
	}
	if a.slash == nil || a.slash.Query != "todo" {
		t.Fatalf("query = %+v", a.slash)
	}
	if _, ok := tt.Find("命令：待办列表"); !ok {
		t.Fatal("todo result missing")
	}
	if _, ok := tt.Find("命令：一级标题"); ok {
		t.Fatal("unrelated result still shown")
	}
	before, _ := a.current().doc.Selection()
	tt.Key(0, ui.KeyEnter)
	doc := a.current().doc
	if doc.Formatting().Paragraph.List != richtext.Tasks || doc.Text().PlainText() != "" || a.slash != nil {
		t.Fatalf("command did not replace /todo: %q / %+v", doc.Text().PlainText(), doc.Formatting())
	}
	doc.Undo()
	tt.Frame()
	if doc.Text().PlainText() != "/todo" {
		t.Fatal("undo should restore the whole command")
	}
	_, caret := doc.Selection()
	if caret != before {
		t.Fatal("undo did not restore selection")
	}
	doc.Redo()
	tt.Frame()
	tt.Type("验证待办")
	if md := doc.Text().Markdown(); !strings.Contains(md, "- [ ] 验证待办") {
		t.Fatalf("todo markdown: %q", md)
	}
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	pages, _ := a.store.List()
	for _, p := range pages {
		if p.ID == a.selected && !strings.Contains(p.Body.Markdown(), "- [ ] 验证待办") {
			t.Fatal("todo was not saved")
		}
	}
}

func TestSlashAliasesAndMouseSelection(t *testing.T) {
	for _, query := range []string{"待办", "TODO", "task", "checkbox", "to-do"} {
		t.Run(query, func(t *testing.T) {
			a, tt := commandTestApp(t)
			tt.Type("/" + query)
			if err := tt.Click("命令：待办列表"); err != nil {
				t.Fatal(err)
			}
			if a.current().doc.Formatting().Paragraph.List != richtext.Tasks || a.current().doc.Text().PlainText() != "" {
				t.Fatal("alias did not insert a todo")
			}
		})
	}
}

func TestSlashKeyboardNavigationAndAnchor(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("上一段")
	tt.Key(0, ui.KeyEnter)
	tt.Type("/heading")
	_, before := a.current().doc.Selection()
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyDown)
	_, after := a.current().doc.Selection()
	if before != after || a.slash == nil || a.slash.Selected != 1 {
		t.Fatal("arrow keys moved the caret instead of the command selection")
	}
	menu, _ := tt.Find("命令：二级标题")
	caret, _ := tt.TextCaret()
	if menu.Y < caret.Y || menu.X > caret.X+32 {
		t.Fatalf("menu is not next to edited paragraph: menu=%v caret=%v", menu, caret)
	}
	tt.Key(0, ui.KeyTab)
	if a.current().doc.Formatting().Paragraph.Heading != 2 || a.current().doc.Text().PlainText() != "上一段\n" {
		t.Fatalf("Tab command failed: %q", a.current().doc.Text().PlainText())
	}
}

func TestSlashBackspaceAndNoMatches(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("/todozz")
	if !tt.HasText("没有匹配的内容块") {
		t.Fatal("empty result state missing")
	}
	tt.Key(0, ui.KeyEnter)
	if a.current().doc.Text().PlainText() != "/todozz" {
		t.Fatal("unmatched command consumed text")
	}
	tt.Key(0, ui.KeyBackspace)
	tt.Key(0, ui.KeyBackspace)
	if _, ok := tt.Find("命令：待办列表"); !ok {
		t.Fatal("backspace did not update matches")
	}
	for range 5 {
		tt.Key(0, ui.KeyBackspace)
	}
	if a.slash != nil {
		t.Fatal("deleting slash left menu open")
	}
	tt.Key(0, ui.KeyEnter)
	if a.current().doc.Text().PlainText() != "\n" {
		t.Fatal("closed menu stole Enter")
	}
}

func TestSlashEscapeAndNewCommand(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("/todo")
	tt.Key(0, ui.KeyEscape)
	tt.Type("x")
	if a.slash != nil || a.current().doc.Text().PlainText() != "/todox" {
		t.Fatal("Escape changed text or reopened menu")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Type("/todo")
	if a.slash == nil {
		t.Fatal("new command did not reopen")
	}
	tt.Key(0, ui.KeyEnter)
	if a.current().doc.Text().PlainText() != "/todox\n" || a.current().doc.Formatting().Paragraph.List != richtext.Tasks {
		t.Fatal("new command consumed unrelated text")
	}
}

func TestSlashDoesNotHijackURLsPathsOrCode(t *testing.T) {
	for _, text := range []string{"https://example.com/todo", "/usr/local", "word/todo"} {
		t.Run(text, func(t *testing.T) {
			a, tt := commandTestApp(t)
			tt.Type(text)
			if a.slash != nil {
				t.Fatalf("opened on %q", text)
			}
		})
	}
	a, tt := commandTestApp(t)
	a.current().doc.ToggleCodeBlock()
	tt.Frame()
	tt.Type("/todo")
	if a.slash != nil {
		t.Fatal("opened in code block")
	}
}

func TestSlashPreservesSurroundingUnicodeText(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("中文内容 /todo")
	tt.Key(0, ui.KeyEnter)
	if a.current().doc.Text().PlainText() != "中文内容 " {
		t.Fatal("command deleted surrounding text")
	}
}

func TestSlashTableCancelAndCommit(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("/table")
	tt.Key(0, ui.KeyEnter)
	if !a.tableOpen {
		t.Fatal("table dialog did not open")
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.current().doc.Text().PlainText() != "/table" || a.slash != nil || a.pendingSlash != nil {
		t.Fatal("cancel changed command or left stale session")
	}
	if err := tt.Click("笔记正文"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("/table")
	tt.Key(0, ui.KeyEnter)
	if err := tt.Click("保存表格"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(a.current().doc.Text().PlainText(), "/table") {
		t.Fatal("committed table left command text")
	}
	a.current().doc.Undo()
	tt.Frame()
	if a.current().doc.Text().PlainText() != "/table" {
		t.Fatal("table command was not one undo step")
	}
}

func TestInsertMenuSearch(t *testing.T) {
	a, tt := commandTestApp(t)
	if err := tt.Click("插入内容"); err != nil {
		t.Fatal(err)
	}
	tt.Type("todo")
	if err := tt.Click("命令：待办列表"); err != nil {
		t.Fatal(err)
	}
	if a.current().doc.Formatting().Paragraph.List != richtext.Tasks {
		t.Fatal("toolbar search failed")
	}
}

func TestSlashMovingCaretCancelsOldCommand(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("/todo\n/todo")
	if a.slash == nil || a.slash.Start != 6 {
		t.Fatal("second command did not open")
	}
	a.current().doc.SetSelection(5, 5)
	tt.Frame()
	if a.slash != nil {
		t.Fatal("moving caret retained stale command range")
	}
	tt.Key(0, ui.KeyEnter)
	if a.current().doc.Text().PlainText() != "/todo\n\n/todo" {
		t.Fatal("caret move executed stale command")
	}
}

func TestSlashCloseButtonPreservesInput(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("/todo")
	if err := tt.Click("关闭命令菜单"); err != nil {
		t.Fatal(err)
	}
	if a.slash != nil || a.current().doc.Text().PlainText() != "/todo" {
		t.Fatal("close menu should keep command text")
	}
	tt.Key(0, ui.KeyEnter)
	if a.current().doc.Formatting().Paragraph.List == richtext.Tasks {
		t.Fatal("closed menu still executed a command")
	}
}
