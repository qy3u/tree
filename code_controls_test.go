package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCodeLanguagePickerHighlightCopyUndoAndSave(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("/code")
	tt.Key(0, ui.KeyEnter)
	tt.Type("fn test() {")
	tt.Key(0, ui.KeyEnter)
	tt.Type("  // 中文注释")
	tt.Key(0, ui.KeyEnter)
	tt.Type("}")
	doc := a.current().doc
	original := doc.Text().String()
	anchor, caret := doc.Selection()
	if err := tt.Click("代码语言 0"); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused("搜索代码语言") {
		t.Fatal("language search not focused")
	}
	tt.Type("rs")
	if _, ok := tt.Find("代码语言：Rust"); !ok {
		t.Fatal("Rust alias missing")
	}
	if _, ok := tt.Find("代码语言：Python"); ok {
		t.Fatal("unrelated language not filtered")
	}
	tt.Key(0, ui.KeyEnter)
	if doc.Text().ParagraphStyleAt(0).CodeLanguage != "rust" || doc.Text().String() != original {
		t.Fatal("language choice changed code")
	}
	if x, y := doc.Selection(); x != anchor || y != caret {
		t.Fatal("language choice lost selection")
	}
	tt.Move(10, 10)
	tt.Frame()
	ed, _ := tt.Find("笔记正文")
	box := doc.VisibleCodeBlocks()[0]
	button, _ := tt.Find("代码语言 0")
	firstLine := ed.Y + doc.VisibleParagraphs()[0].Top
	if button.Y+button.H >= firstLine {
		t.Fatal("code controls overlap first line")
	}
	img := tt.Image()
	colored := 0
	for y := int(firstLine); y < int(ed.Y+box.Rect.Y+box.Rect.H); y++ {
		for x := int(ed.X); x < int(ed.X+ed.W); x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if b > r+6000 && g > r+6000 {
				colored++
			}
		}
	}
	if colored < 5 {
		t.Fatal("Rust keywords were not painted in color")
	}
	if err := tt.Click("复制代码 0"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != "fn test() {\n  // 中文注释\n}" {
		t.Fatalf("copy = %q", got)
	}
	if doc.Text().String() != original {
		t.Fatal("copy mutated document")
	}
	doc.Undo()
	tt.Frame()
	if doc.Text().ParagraphStyleAt(0).CodeLanguage != "" || doc.Text().String() != original {
		t.Fatal("language undo affected code")
	}
	doc.Redo()
	tt.Frame()
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	reopened.selectPage(a.selected)
	if st := reopened.current().doc.Text().ParagraphStyleAt(0); st.CodeLanguage != "rust" {
		t.Fatal("language lost on reopen")
	}
	if md := reopened.current().page.Body.Markdown(); !strings.HasPrefix(md, "```rust\n") {
		t.Fatalf("export lost language: %q", md)
	}
	if err := tt.Click("代码语言 0"); err != nil {
		t.Fatal(err)
	}
	tt.Type("plain")
	tt.Key(0, ui.KeyEnter)
	if doc.Text().ParagraphStyleAt(0).CodeLanguage != "" {
		t.Fatal("plain text did not clear language")
	}
}

func TestCommonCodeLanguagesAndMarkdownRoundTrip(t *testing.T) {
	for _, name := range []string{"Rust", "Go", "Python", "JavaScript", "TypeScript", "Java", "C", "C++", "C#", "Swift", "Kotlin", "Ruby", "PHP", "Shell", "SQL", "JSON", "HTML", "CSS", "YAML"} {
		found := false
		for _, option := range codeLanguages(name) {
			if option.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %s", name)
		}
	}
	md := "```rust\n/* 中文\n multi-line */\nfn test() {}\n```\n\n```python\nprint(\"你好\")\n```"
	body := bodyFromMarkdown(md)
	if got := body.Markdown(); got != md {
		t.Fatalf("round trip:\n%s", got)
	}
	a, tt := commandTestApp(t)
	doc := a.current().doc
	doc.Insert(body.Text)
	tt.Frame()
	boxes := doc.VisibleCodeBlocks()
	if len(boxes) != 2 || boxes[0].Language != "rust" || boxes[1].Language != "python" {
		t.Fatalf("code block groups: %+v", boxes)
	}
	if got := codeBlockText(a.current(), boxes[0]); got != "/* 中文\n multi-line */\nfn test() {}" {
		t.Fatalf("copied group: %q", got)
	}
	setCodeLanguage(a.current(), boxes[0], "go")
	tt.Frame()
	if doc.Text().ParagraphStyleAt(boxes[1].Start).CodeLanguage != "python" {
		t.Fatal("language changed adjacent block")
	}
}

func TestCodeFenceLanguageAndTypingDoesNotPersistHighlightColors(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("```python")
	tt.Key(0, ui.KeyEnter)
	tt.Type("print(\"你好\")")
	doc := a.current().doc
	if doc.Text().ParagraphStyleAt(0).CodeLanguage != "python" {
		t.Fatal("fence shortcut lost language")
	}
	for _, r := range doc.Text().Runs(0, doc.Text().Len()) {
		if r.Attributes.Color != (ui.Color{}) {
			t.Fatal("syntax colors leaked into saved formatting")
		}
	}
	version := doc.Version()
	tt.Frame()
	tt.Frame()
	if doc.Version() != version {
		t.Fatal("painting changed document version")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Type("# comment")
	if doc.Text().ParagraphCount() != 1 || !doc.Text().ParagraphStyleAt(0).CodeBlock {
		t.Fatal("highlighting broke block-local Enter")
	}
}
