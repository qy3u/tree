package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func TestInlineMathBothEditorsTypingEditAndUndo(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		name := "page"
		if leaf {
			name = "leaf"
		}
		t.Run(name, func(t *testing.T) {
			a, tt, d, root := mathTestEditor(t, leaf)
			// Type character by character: Markdown must not eat LaTeX underscores.
			source := `\sum_{i=1}^{n} w_iP_i`
			for _, r := range "价格 \\(" + source + "\\)" {
				tt.Type(string(r))
			}
			tt.Type(" 元")
			if got := d.doc.Text().String(); got != "价格 "+string(richtext.ObjectReplacement)+" 元" {
				t.Fatal(got)
			}
			at := d.doc.Text().AttributesAt(3).Attachment
			if !isInlineMath(at) || at.Descent <= 0 || at.Width >= 720 || mathSources(d.doc.Text())[0] != source {
				t.Fatal("inline metrics/source", at)
			}
			tt.Frame()
			if err := tt.Click("公式"); err != nil {
				t.Fatal(err)
			}
			if !tt.Focused("公式 LaTeX") {
				t.Fatal("click did not edit")
			}
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type(`\frac{a}{b}`)
			tt.Key(ui.Cmd, ui.KeyEnter)
			tt.Frame()
			if !isInlineMath(d.doc.Text().AttributesAt(3).Attachment) || mathSources(d.doc.Text())[0] != `\frac{a}{b}` {
				t.Fatal("lost inline mode")
			}
			tt.Key(ui.Cmd, ui.KeyZ)
			if mathSources(d.doc.Text())[0] != source {
				t.Fatal("undo source")
			}
			tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
			tt.Advance(time.Second)
			tt.Click("公式")
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type("cancel")
			tt.Key(0, ui.KeyEscape)
			if mathSources(d.doc.Text())[0] != `\frac{a}{b}` {
				t.Fatal("cancel source")
			}
			if err := a.flush(); err != nil {
				t.Fatal(err)
			}
			reopened, err := newApp(a.store)
			if err != nil {
				t.Fatal(err)
			}
			body := reopened.find(root.page.ID).Body
			if leaf {
				body = body.Leaves[d.leafID].Content
			}
			hydrated := hydrate(body, a.store.dir)
			at = hydrated.AttributesAt(3).Attachment
			if !isInlineMath(at) || at.Draw == nil || at.Descent <= 0 {
				t.Fatal("reload lost inline formula")
			}
			if md := body.Markdown(); !strings.Contains(md, `\(\frac{a}{b}\)`) {
				t.Fatal(md)
			}
		})
	}
}
func TestInlineMathPasteReplaceCopyAndDelete(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		_, tt, d, _ := mathTestEditor(t, leaf)
		tt.Type("前🌲替换后")
		d.doc.SetSelection(2, 4)
		tt.Frame()
		tt.SetClipboard(`$x_i$ 和 \(y^2\)`)
		tt.Key(ui.Cmd, ui.KeyV)
		tt.Frame()
		want := "前🌲" + string(richtext.ObjectReplacement) + " 和 " + string(richtext.ObjectReplacement) + "后"
		if d.doc.Text().String() != want {
			t.Fatal(d.doc.Text().String())
		}
		tt.Key(ui.Cmd, ui.KeyZ)
		if d.doc.Text().String() != "前🌲替换后" {
			t.Fatal("paste undo")
		}
		tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
		d.doc.SetSelection(2, 3)
		tt.Frame()
		tt.Key(ui.Cmd, ui.KeyC)
		d.doc.SetSelection(d.doc.Text().Len(), d.doc.Text().Len())
		tt.Frame()
		tt.Key(ui.Cmd, ui.KeyV)
		tt.Frame()
		end := d.doc.Text().Len()
		at := d.doc.Text().AttributesAt(end - 1).Attachment
		if !isInlineMath(at) || at.Draw == nil || at.Descent <= 0 || len(mathSources(d.doc.Text())) != 3 {
			t.Fatal("rich copy lost drawing/metrics")
		}
		tt.Key(0, ui.KeyBackspace)
		if len(mathSources(d.doc.Text())) != 2 {
			t.Fatal("delete not atomic")
		}
		tt.Key(ui.Cmd, ui.KeyZ)
		if len(mathSources(d.doc.Text())) != 3 {
			t.Fatal("delete undo")
		}
	}
}
func TestInlineMathParsingAndMarkdownRoundTrip(t *testing.T) {
	raw := "**能量** $E=mc^2$，比率 \\(\\frac{a}{b}\\)。\n\n\\[\nx^2\n\\]\n\n`$literal$`\n\n```tex\n\\(literal\\)\n```"
	body := bodyFromMarkdown(raw)
	if got := mathSources(body.Text); len(got) != 3 || got[0] != "E=mc^2" || got[1] != `\frac{a}{b}` {
		t.Fatal(got)
	}
	if !body.Text.AttributesAt(0).Bold() {
		t.Fatal("surrounding formatting lost")
	}
	out := body.Markdown()
	again := bodyFromMarkdown(out)
	if strings.Join(mathSources(body.Text), "|") != strings.Join(mathSources(again.Text), "|") {
		t.Fatal("roundtrip", out)
	}
	for _, literal := range []string{`price $20 and $30`, `https://example.com/$x$/`, "`unclosed $x$", `\$x$`, `\\(x\)`, `$ x$`, `$x $`, `$x$2`, "$x\ny$", "`$x$`", "    $x$", "~~~\n$x$\n~~~", `\(unfinished`} {
		converted, changed := convertMathText(richtext.NewText(literal))
		if changed || converted.String() != literal {
			t.Fatalf("converted literal %q", literal)
		}
	}
	code := richtext.NewText(`$x_i$`).Format(0, 5, func(a *richtext.Attributes) { a.Code = true })
	if _, changed := convertMathText(code); changed {
		t.Fatal("converted inline code")
	}
	link := richtext.NewText(`https://example.com/$x$/`).Format(0, 24, func(a *richtext.Attributes) { a.Link = "https://example.com/$x$/" })
	if _, changed := convertMathText(link); changed {
		t.Fatal("converted link")
	}
}
func TestInlineMathDollarTypingAndPlainPaste(t *testing.T) {
	_, tt, d, _ := mathTestEditor(t, false)
	for _, r := range `质量 $m_1+m_2$` {
		tt.Type(string(r))
	}
	if len(mathSources(d.doc.Text())) != 1 {
		t.Fatal("dollar typing")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if !strings.Contains(d.doc.Text().String(), `$m_1+m_2$`) {
		t.Fatal("undo should restore complete TeX")
	}
	tt.Frame()
	if len(mathSources(d.doc.Text())) != 0 {
		t.Fatal("undo immediately reconverted")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	tt.Type(" 正文")
	if !strings.HasSuffix(d.doc.Text().String(), " 正文") {
		t.Fatal("typing after formula")
	}
	tt.SetClipboard(`\(plain\)`)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyV)
	if !strings.HasSuffix(d.doc.Text().String(), `\(plain\)`) {
		t.Fatal("plain paste converted")
	}
}
func TestInlineMathGitRoundTrip(t *testing.T) {
	store := testStore(t)
	p := createPage(t, store, "", "行内公式")
	p.Body = bodyFromMarkdown(`前 \(x_i\) 后 $y^2$`)
	p.Body.Leaves = map[string]Leaf{"math-leaf": {Title: "知识", Content: bodyFromMarkdown(`叶子 \(\frac{a}{b}\) 结束`)}}
	p.Body.Text = p.Body.Text.Replace(p.Body.Text.Len(), p.Body.Text.Len(), richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: "\n"}, {Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: leafAttachment("math-leaf")}}}}))
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	dir := exportFixture(t, store)
	destination := filepath.Join(t.TempDir(), "restore")
	if err := RestoreGit(dir, destination); err != nil {
		t.Fatal(err)
	}
	restored, err := openStore(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	pages, err := restored.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range pages {
		if page.Title != "行内公式" {
			continue
		}
		body := page.Body
		for _, part := range []Body{body, body.Leaves["math-leaf"].Content} {
			ats := (Body{Text: hydrate(part, restored.dir)}).attachments()
			found := false
			for _, at := range ats {
				if isInlineMath(at) {
					found = true
					if at.Draw == nil || at.Width >= 720 {
						t.Fatal("restore render")
					}
				}
			}
			if !found {
				t.Fatal("missing inline formula")
			}
		}
	}
	if diff := changedGitFiles(gitFiles(t, dir), gitFiles(t, exportFixture(t, restored))); len(diff) > 0 {
		t.Fatal("unstable roundtrip", diff)
	}
}

func TestInlineMathWrappedClickAndMultilineEdit(t *testing.T) {
	a, tt, d, _ := mathTestEditor(t, false)
	prefix := strings.Repeat("这里是用于验证自然换行的文字。", 4)
	tt.SetClipboard(prefix + `\(x_i\)` + "后文")
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	at := len([]rune(prefix))
	box, _ := tt.Find("公式")
	editor, _ := tt.Find("笔记正文")
	if box.Y < editor.Y+100 {
		t.Fatal("formula did not wrap")
	}
	if err := tt.Click("公式"); err != nil {
		t.Fatal(err)
	}
	if a.mathEdit == nil || a.mathEdit.start != at {
		t.Fatal("wrapped hit selected wrong formula")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("x_i\n+y_i")
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	md := (Body{Text: d.doc.Text()}).Markdown()
	restored := bodyFromMarkdown(md)
	if got := mathSources(restored.Text); len(got) != 1 || got[0] != "x_i\n+y_i" {
		t.Fatal("multiline edited source lost", md, got)
	}
}

func TestInlineMathInsideTypedCodeRemainsLiteral(t *testing.T) {
	_, tt, d, _ := mathTestEditor(t, false)
	for _, r := range "`$x^2$`" {
		tt.Type(string(r))
	}
	if len(mathSources(d.doc.Text())) != 0 || !d.doc.Text().AttributesAt(0).Code {
		t.Fatal("inline code converted")
	}
}
