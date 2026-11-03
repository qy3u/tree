package main

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func TestCodeEnterKeepsOneBlockAndSurvivesSave(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("/code")
	tt.Key(0, ui.KeyEnter)
	doc := a.current().doc
	tt.Key(0, ui.KeyEnter) // An empty code block must stay code.
	tt.Type("fn test() {}")
	pos := doc.Text().Len() - 1
	doc.SetSelection(pos, pos)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnter)
	want := "\u2028fn test() {\u2028\u2028}"
	assertCode := func() {
		t.Helper()
		if doc.Text().String() != want || doc.Text().ParagraphCount() != 1 || !doc.Text().ParagraphStyleAt(0).CodeBlock {
			t.Fatalf("Enter split or exited code: %q (%d blocks)", doc.Text().String(), doc.Text().ParagraphCount())
		}
	}
	assertCode()
	tt.Key(ui.Cmd, ui.KeyZ)
	if got := doc.Text().String(); got != "\u2028fn test() {\u2028}" {
		t.Fatalf("undo did not remove just the last newline: %q", got)
	}
	doc.Redo()
	tt.Frame()
	assertCode()
	wantMD := "```\n\nfn test() {\n\n}\n```"
	if got := (Body{Text: doc.Text()}).Markdown(); got != wantMD {
		t.Fatalf("code export changed whitespace: %q", got)
	}
	if got := doc.Text().HTML(); !strings.Contains(got, "<pre><code>\nfn test() {\n\n}</code></pre>") {
		t.Fatalf("HTML lost code newlines: %q", got)
	}
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	reopened.selectPage(a.selected)
	doc = reopened.current().doc
	assertCode()
}

func TestCodeEnterAtBoundariesAndSelection(t *testing.T) {
	for _, where := range []string{"start", "middle", "end", "selection"} {
		t.Run(where, func(t *testing.T) {
			a, tt := commandTestApp(t)
			doc := a.current().doc
			doc.Insert(richtext.ParseMarkdown("before\n\n```\nabc\n```\n\nafter"))
			start := strings.Index(doc.Text().String(), "abc")
			lo, hi := start, start
			code := "\u2028abc"
			switch where {
			case "middle":
				lo, hi, code = start+1, start+1, "a\u2028bc"
			case "end":
				lo, hi, code = start+3, start+3, "abc\u2028"
			case "selection":
				lo, hi, code = start+1, start+2, "a\u2028c"
			}
			doc.SetSelection(lo, hi)
			tt.Frame()
			tt.Key(0, ui.KeyEnter)
			if got := doc.Text().String(); got != "before\n"+code+"\nafter" || doc.Text().ParagraphCount() != 3 {
				t.Fatalf("code Enter changed surrounding blocks: %q", got)
			}
			if !doc.Text().ParagraphStyleAt(start).CodeBlock {
				t.Fatal("code style lost")
			}
		})
	}
}

func TestSelectionToolbarAppearsAndDismisses(t *testing.T) {
	_, tt := commandTestApp(t)
	tt.Type("选中文字")
	if _, ok := tt.Find("加粗 · ⌘B"); ok {
		t.Fatal("formatting bar is visible without a selection")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	if err := tt.Click("加粗 · ⌘B"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	if _, ok := tt.Find("加粗 · ⌘B"); ok {
		t.Fatal("formatting bar remains after collapsing the selection")
	}
}

func TestHoverHandleAndParagraphDrag(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("第一段\n第二段\n第三段")
	tt.Move(20, 20)
	if _, ok := tt.Find("移动段落 1"); ok {
		t.Fatal("paragraph handle remains visible away from content")
	}
	editor, _ := tt.Find("笔记正文")
	paras := a.current().doc.VisibleParagraphs()
	x, y := editor.X-20, editor.Y+paras[0].Top+14
	tt.Move(x, y)
	if _, ok := tt.Find("移动段落 1"); !ok {
		t.Fatal("hover did not reveal the handle")
	}
	tt.Press(x, y)
	tt.Move(x, y+6)
	tt.Move(x, editor.Y+paras[2].Top+14)
	tt.Release(x, editor.Y+paras[2].Top+14)
	if got := a.current().doc.Text().PlainText(); got != "第二段\n第三段\n第一段" {
		t.Fatalf("drag result = %q", got)
	}
	a.current().doc.Undo()
	if got := a.current().doc.Text().PlainText(); got != "第一段\n第二段\n第三段" {
		t.Fatalf("drag undo = %q", got)
	}
}

func TestSquareTaskMarkerKeepsNativeToggleAndUndo(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("/todo")
	tt.Key(0, ui.KeyEnter)
	tt.Type("待办")
	editor, _ := tt.Find("笔记正文")
	box := a.current().doc.VisibleParagraphs()[0]
	tt.ClickAt(editor.X+12, editor.Y+box.Top+14)
	if !a.current().doc.Text().ParagraphStyleAt(0).Checked {
		t.Fatal("task marker click did not check the task")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	style := a.current().doc.Text().ParagraphStyleAt(0)
	if style.Checked || style.List != richtext.Tasks {
		t.Fatal("task toggle undo changed the block type")
	}
}

func TestEmptyTodoDoesNotShowDocumentPlaceholder(t *testing.T) {
	a, tt := commandTestApp(t)
	doc := a.current().doc
	if notebookPlaceholder(doc.Text()) == "" {
		t.Fatal("empty plain page lost its hint")
	}
	tt.Type("/todo")
	tt.Key(0, ui.KeyEnter)
	if notebookPlaceholder(doc.Text()) != "" {
		t.Fatal("document hint overlaps the empty task marker")
	}
	editor, _ := tt.Find("笔记正文")
	caret, ok := tt.TextCaret()
	if !ok || caret.X < editor.X+notebookFont.Size*notebookTheme.Indent-1 {
		t.Fatalf("task caret is not after the checkbox: %+v", caret)
	}
	tt.Type("新的待办")
	if doc.Text().PlainText() != "新的待办" || doc.Formatting().Paragraph.List != richtext.Tasks {
		t.Fatal("task input changed")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	if notebookPlaceholder(doc.Text()) != "" {
		t.Fatal("deleting task text restored the overlapping hint")
	}
	tt.Key(0, ui.KeyBackspace)
	if notebookPlaceholder(doc.Text()) == "" {
		t.Fatal("returning to a plain paragraph did not restore the hint")
	}
}

func TestNotebookColumnAtSmallAndLargeWindowSizes(t *testing.T) {
	for _, size := range [][2]float32{{850, 580}, {1440, 1000}} {
		a, err := newApp(testStore(t))
		if err != nil {
			t.Fatal(err)
		}
		tt := ui.NewTester(a.view, int(size[0]), int(size[1]))
		editor, _ := tt.Find("笔记正文")
		title, _ := tt.Find("页面标题")
		left, right := editor.X-sidebarWidth-1, size[0]-editor.X-editor.W
		if math.Abs(float64(left-right)) > 1 || editor.W > 721 || editor.H < 180 {
			t.Fatalf("unbalanced or unusable content at %v: %+v", size, editor)
		}
		if math.Abs(float64(title.X-editor.X)) > 1 {
			t.Fatalf("title and body do not align: %+v / %+v", title, editor)
		}
	}
}

func TestGutterFollowsTypingWithoutPointerMovement(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("a\nb")
	editor, _ := tt.Find("笔记正文")
	boxes := a.current().doc.VisibleParagraphs()
	tt.Move(editor.X+60, editor.Y+boxes[1].Top+14)
	if _, ok := tt.Find("移动段落 2"); !ok {
		t.Fatal("hover did not show second handle")
	}
	tt.Key(0, ui.KeyEnter)
	if _, ok := tt.Find("移动段落 3"); !ok {
		t.Fatal("new line handle did not follow Enter immediately")
	}
	if _, ok := tt.Find("移动段落 2"); ok {
		t.Fatal("old hovered handle stayed after Enter")
	}
	tt.Type("c")
	tt.Key(0, ui.KeyEnter)
	if _, ok := tt.Find("移动段落 4"); !ok {
		t.Fatal("consecutive Enter did not update handle")
	}
	tt.Key(0, ui.KeyBackspace)
	if _, ok := tt.Find("移动段落 3"); !ok {
		t.Fatal("handle did not follow deleted newline")
	}
	tt.Move(editor.X+61, editor.Y+boxes[0].Top+14)
	if _, ok := tt.Find("移动段落 1"); !ok {
		t.Fatal("mouse movement did not take over from caret")
	}
	if _, ok := tt.Find("移动段落 3"); ok {
		t.Fatal("caret and hover handles both visible")
	}
}

func TestGutterCentersOnFirstTextLine(t *testing.T) {
	for _, prefix := range []string{"", "# ", "## ", "### ", "- ", "/code"} {
		t.Run(prefix, func(t *testing.T) {
			a, tt := commandTestApp(t)
			if prefix != "" {
				tt.Type(prefix)
			}
			if prefix == "/code" {
				tt.Key(0, ui.KeyEnter)
			}
			tt.Type("从这里开始")
			tt.Key(ui.Shift, ui.KeyEnter)
			tt.Type("第二行文字")
			a.current().doc.SetSelection(0, 0)
			tt.Frame()
			editor, _ := tt.Find("笔记正文")
			caret, ok := tt.TextCaret()
			if !ok {
				t.Fatal("missing caret")
			}
			tt.Move(editor.X+80, caret.Y+caret.H/2)
			tt.Frame()
			handle, ok := tt.Find("移动段落 1")
			if !ok {
				t.Fatal("missing handle")
			}
			if math.Abs(float64(handle.Y+handle.H/2-caret.Y-caret.H/2)) > 1 {
				t.Fatalf("handle not centered on first line: handle=%+v caret=%+v", handle, caret)
			}
		})
	}
}
