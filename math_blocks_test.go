package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caelyreth/untie"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

const indexFormula = `\text{Index Price}=\sum_{i=1}^{n}w_iP_i`

func TestMathRendererSupportedExpressions(t *testing.T) {
	renderer, err := formulaRenderer()
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{indexFormula, `\frac{a}{b}+\sqrt{x^2+y^2}`, `\int_0^1 x^2\,dx`, `\begin{pmatrix}a&b\\c&d\end{pmatrix}`} {
		r, err := renderer.Render(context.Background(), source, untie.Display, untie.Style{})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Diagnostics) != 0 || r.Box == nil || r.Box.Bounds().Width <= 0 {
			t.Fatalf("%s: %+v", source, r.Diagnostics)
		}
	}
}
func mathSources(text richtext.Text) []string {
	var result []string
	for _, r := range text.Runs(0, text.Len()) {
		if s, ok := mathSource(r.Attributes.Attachment); ok {
			result = append(result, s)
		}
	}
	return result
}
func TestMathMarkdownAndStorageRoundTrip(t *testing.T) {
	md := "# 标题\n\n\\[\n" + indexFormula + "\n\\]\n\n正文\n\n$$\\frac{1}{2}$$\n\n```tex\n\\[x\\]\n```"
	b := parseMarkdownBody(md)
	if sources := mathSources(b.Text); len(sources) != 2 || sources[0] != indexFormula {
		t.Fatal(sources)
	}
	for _, p := range b.Text.Paragraphs() {
		if p.Style.CodeBlock && !strings.Contains(richtext.TextOf(p).String(), `\[x\]`) {
			t.Fatal("code changed")
		}
	}
	again := parseMarkdownBody(b.Markdown())
	if strings.Join(mathSources(again.Text), "|") != strings.Join(mathSources(b.Text), "|") {
		t.Fatal("Markdown lost formula")
	}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var restored Body
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	restored.Text = hydrate(restored, t.TempDir())
	for _, at := range restored.attachments() {
		if at.Draw == nil {
			t.Fatal("formula drawing not restored")
		}
	}
	if got := mathSources(restored.Text); len(got) != 2 || got[0] != indexFormula {
		t.Fatal(got)
	}
}
func mathTestEditor(t *testing.T, leaf bool) (*app, *ui.Tester, *draft, *draft) {
	t.Helper()
	if leaf {
		a, tt, root, v := createTestLeaf(t)
		tt.Key(0, ui.KeyEnter)
		return a, tt, v.draft, root
	}
	a, tt := commandTestApp(t)
	return a, tt, a.current(), a.current()
}
func TestMathPasteEditUndoAndReload(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		name := "page"
		if leaf {
			name = "leaf"
		}
		t.Run(name, func(t *testing.T) {
			a, tt, d, root := mathTestEditor(t, leaf)
			tt.SetClipboard("\\[\n" + indexFormula + "\n\\]")
			tt.Key(ui.Cmd, ui.KeyV)
			tt.Frame()
			tt.Frame()
			if got := mathSources(d.doc.Text()); len(got) != 1 || got[0] != indexFormula {
				t.Fatalf("paste: %q / %v", d.doc.Text().String(), got)
			}
			if err := tt.Click("公式"); err != nil {
				t.Fatal(err)
			}
			if a.mathEdit == nil || !tt.Focused("公式 LaTeX") {
				t.Fatal("formula click did not open/focus editor")
			}
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type(`\frac{1}{2}`)
			if err := tt.Click("保存公式"); err != nil {
				t.Fatal(err)
			}
			if a.mathEdit != nil || mathSources(d.doc.Text())[0] != `\frac{1}{2}` {
				t.Fatal("save failed")
			}
			tt.Key(ui.Cmd, ui.KeyZ)
			if got := mathSources(d.doc.Text()); len(got) != 1 || got[0] != indexFormula {
				t.Fatal("edit undo failed", got)
			}
			tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
			if got := mathSources(d.doc.Text()); len(got) != 1 || got[0] != `\frac{1}{2}` {
				t.Fatal("edit redo failed", got)
			}
			tt.Advance(time.Second)
			if err := tt.Click("公式"); err != nil {
				t.Fatal(err)
			}
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type("cancel me")
			tt.Key(0, ui.KeyEscape)
			if a.mathEdit != nil || mathSources(d.doc.Text())[0] != `\frac{1}{2}` {
				t.Fatal("cancel changed formula")
			}
			if err := a.flush(); err != nil {
				t.Fatal(err)
			}
			next, err := newApp(a.store)
			if err != nil {
				t.Fatal(err)
			}
			body := next.find(root.page.ID).Body
			if leaf {
				body = body.Leaves[d.leafID].Content
			}
			if got := mathSources(hydrate(body, a.store.dir)); len(got) != 1 || got[0] != `\frac{1}{2}` {
				t.Fatal("reload lost source", got)
			}
			if !strings.Contains(body.Markdown(), "\\[\n\\frac{1}{2}\n\\]") {
				t.Fatal(body.Markdown())
			}
		})
	}
}
func TestMathSlashBothEditors(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		a, tt, d, _ := mathTestEditor(t, leaf)
		tt.Type("/math")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if a.mathEdit == nil || !tt.Focused("公式 LaTeX") {
			t.Fatalf("leaf=%t: slash did not start editing", leaf)
		}
		tt.Type(indexFormula)
		tt.Key(ui.Cmd, ui.KeyEnter)
		tt.Frame()
		if a.mathEdit != nil || len(mathSources(d.doc.Text())) != 1 || mathSources(d.doc.Text())[0] != indexFormula {
			t.Fatal("slash save failed")
		}
	}
}
func TestMathPasteContextAndRichCopy(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	tt.Type("前文替换后文")
	d.doc.SetSelection(2, 4)
	tt.Frame()
	tt.SetClipboard(`$$x_i^2$$`)
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	if d.doc.Text().String() != "前文\n"+string(richtext.ObjectReplacement)+"\n后文" {
		t.Fatal(d.doc.Text().String())
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if d.doc.Text().String() != "前文替换后文" {
		t.Fatal("paste not one undo")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	d.doc.SetSelection(3, 4)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyC)
	d.doc.SetSelection(d.doc.Text().Len(), d.doc.Text().Len())
	tt.Key(0, ui.KeyEnter)
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	if len(mathSources(d.doc.Text())) != 2 {
		t.Fatal("rich copy lost source")
	}
	for _, at := range (Body{Text: d.doc.Text()}).attachments() {
		if at.Draw == nil {
			t.Fatal("rich copy lost rendering")
		}
	}
	// HTML pasted from a browser preserves ordinary rich text surrounding the formula.
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte(`<p><b>保持粗体</b></p><p>\[x^2\]</p>`)))))
	tt.Key(0, ui.KeyEnter)
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	if len(mathSources(d.doc.Text())) != 3 {
		t.Fatal("HTML math missing")
	}
	found := false
	for _, r := range d.doc.Text().Runs(0, d.doc.Text().Len()) {
		if strings.TrimSpace(r.Text) == "保持粗体" && r.Attributes.Bold() {
			found = true
		}
	}
	if !found {
		for _, r := range d.doc.Text().Runs(0, d.doc.Text().Len()) {
			t.Logf("%q: %+v", r.Text, r.Attributes)
		}
		t.Fatal("HTML formatting lost")
	}
}
func TestMathLiteralAndTypedEnter(t *testing.T) {
	for _, literal := range []string{`\[unfinished`, `cost $20`, "```tex\n\\[x\\]\n```", "~~~\n$$x$$\n~~~", `\\[x\]`} {
		text, changed := convertMathText(richtext.NewText(literal))
		if changed || text.String() != literal {
			t.Fatal("changed literal", literal)
		}
	}
	a, tt := commandTestApp(t)
	d := a.current()
	tt.Type(`\[x_i^2\]`)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if got := mathSources(d.doc.Text()); len(got) != 1 || got[0] != `x_i^2` {
		t.Fatal("typed conversion", got)
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if d.doc.Text().String() != `\[x_i^2\]` {
		t.Fatal("typed undo")
	}
	d.doc.SetText(richtext.NewText("").FormatParagraphs(0, 0, func(s *richtext.ParagraphStyle) { s.CodeBlock = true }))
	d.doc.SetSelection(0, 0)
	tt.Frame()
	tt.SetClipboard(`\[x^2\]`)
	tt.Key(ui.Cmd, ui.KeyV)
	if len(mathSources(d.doc.Text())) != 0 || d.doc.Text().String() != `\[x^2\]` {
		t.Fatal("code paste changed")
	}
}
func TestMathGitExportImport(t *testing.T) {
	s := testStore(t)
	p := createPage(t, s, "", "公式")
	p.Body = bodyFromMarkdown("\\[\n" + indexFormula + "\n\\]")
	p.Body.Leaves = map[string]Leaf{"leaf-math": {Title: "公式知识", Content: bodyFromMarkdown(`$$\frac{1}{2}$$`)}}
	p.Body.Text = p.Body.Text.Replace(p.Body.Text.Len(), p.Body.Text.Len(), richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: "\n"}, {Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: leafAttachment("leaf-math")}}}}))
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	dir := exportFixture(t, s)
	before := gitFiles(t, dir)
	if err := s.ExportGit(dir); err != nil {
		t.Fatal(err)
	}
	if diff := changedGitFiles(before, gitFiles(t, dir)); len(diff) != 0 {
		t.Fatal("non deterministic export", diff)
	}
	destination := filepath.Join(t.TempDir(), "restored")
	if err := RestoreGit(dir, destination); err != nil {
		t.Fatal(err)
	}
	dest, err := openStore(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer dest.Close()
	pages, err := dest.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, page := range pages {
		if page.Title == "公式" {
			found = true
			if mathSources(page.Body.Text)[0] != indexFormula {
				t.Fatal("Git lost source")
			}
			if len(page.Body.Leaves) != 1 {
				t.Fatal("Git lost Leaf")
			}
		}
	}
	if !found {
		t.Fatal("missing imported page")
	}
}

func TestMathEditOutsideCancelStaleAndInvalidSource(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	tt.SetClipboard(`\[x^2\]`)
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	if err := tt.Click("公式"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("x^3")
	tt.Key(0, ui.KeyEnter)
	tt.Type("+y")
	if a.mathEdit == nil || a.mathEdit.source != "x^3\n+y" {
		t.Fatal("Enter should edit a multiline source")
	}
	tt.ClickAt(1090, 750)
	tt.Frame()
	if a.mathEdit != nil || mathSources(d.doc.Text())[0] != "x^3\n+y" {
		t.Fatal("outside save")
	}
	tt.Advance(time.Second)
	if err := tt.Click("公式"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type(`\frac{`)
	tt.Click("保存公式")
	if mathSources(d.doc.Text())[0] != `\frac{` {
		t.Fatal("invalid source lost")
	}
	tt.Advance(time.Second)
	tt.Click("公式")
	d.doc.InsertText("new content")
	tt.Frame()
	if a.mathEdit != nil {
		t.Fatal("stale editor should close")
	}
}
