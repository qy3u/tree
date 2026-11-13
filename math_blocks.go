package main

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"sync"

	"github.com/caelyreth/untie"
	untiemygo "github.com/caelyreth/untie/contrib/mygo"
	"github.com/caelyreth/untie/render"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

// New Computer Modern Math is bundled under the GUST Font License; no network or system font is needed.
//
//go:embed assets/math/*
var mathAssets embed.FS

var formulaRenderer = sync.OnceValues(func() (*untiemygo.Renderer, error) {
	fontBytes, err := mathAssets.ReadFile("assets/math/NewCMMath-Book.otf")
	if err != nil {
		return nil, err
	}
	fonts, err := untie.LoadMathFont(fontBytes)
	if err != nil {
		return nil, err
	}
	return untiemygo.New(fonts, untiemygo.Options{DisplaySize: 22, CacheEntries: 128,
		Draw: untiemygo.DrawOptions{Padding: 12, Align: render.AlignCenter}})
})

func mathSource(at *richtext.Attachment) (string, bool) {
	if isInlineMath(at) {
		return strings.TrimPrefix(at.URL, "math-inline:"), true
	}
	if at == nil || !strings.HasPrefix(at.URL, "math:") {
		return "", false
	}
	return strings.TrimPrefix(at.URL, "math:"), true
}

func mathAttachment(source string) *richtext.Attachment {
	// Store source directly in the attachment, so copying, undo and Git snapshots
	// keep it losslessly without a separate mutable record or generated image.
	at := &richtext.Attachment{URL: "math:" + source, Name: "\\[\n" + source + "\n\\]", Width: 720, Height: 60}
	renderer, err := formulaRenderer()
	var result *untie.RenderResult
	if err == nil && strings.TrimSpace(source) != "" {
		result, err = renderer.Render(context.Background(), source, untie.Display, untie.Style{})
		if result != nil && result.Box != nil {
			at.Height = max(at.Height, result.Box.Bounds().Height+24)
		}
	}
	at.Draw = func(p *ui.Painter, r ui.Rect) {
		p.Clip(r, 0, func() {
			if result != nil && result.Box != nil {
				_ = renderer.Draw(p, result.Box, r)
			} else {
				label := "点击编辑公式"
				if strings.TrimSpace(source) != "" {
					label = "公式无法渲染，点击编辑 LaTeX"
				}
				p.Text(r.X+12, r.Y+(r.H-18)/2, label, 14, muted)
			}
		})
	}
	return at
}

func hydrateMath(text richtext.Text) richtext.Text {
	var paragraphs []richtext.Paragraph
	for _, p := range text.Paragraphs() {
		for i, run := range p.Runs {
			if source, ok := mathSource(run.Attributes.Attachment); ok {
				p.Runs[i].Attributes.Attachment = formulaAttachment(source, isInlineMath(run.Attributes.Attachment))
			}
		}
		paragraphs = append(paragraphs, p)
	}
	return richtext.TextOf(paragraphs...)
}

// Display delimiters must occupy their own block. Code fences, escaped
// delimiters, incomplete formulas and ordinary currency remain literal text.
type mathSpan struct {
	start, end int
	source     string
	inline     bool
}

func displayMathSpans(raw string) []mathSpan {
	lines := strings.Split(raw, "\n")
	offsets := make([]int, len(lines))
	for i := 1; i < len(lines); i++ {
		offsets[i] = offsets[i-1] + len([]rune(lines[i-1])) + 1
	}
	var spans []mathSpan
	var fence byte
	fenceLen := 0
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			n := 0
			for n < len(line) && line[n] == line[0] {
				n++
			}
			if fence == 0 {
				fence = line[0]
				fenceLen = n
			} else if line[0] == fence && n >= fenceLen && strings.TrimSpace(line[n:]) == "" {
				fence = 0
			}
			continue
		}
		if fence != 0 {
			continue
		}
		open, close := "", ""
		if strings.HasPrefix(line, `\[`) {
			open, close = `\[`, `\]`
		} else if strings.HasPrefix(line, "$$") {
			open, close = "$$", "$$"
		}
		if open == "" {
			continue
		}
		body := strings.TrimPrefix(line, open)
		last := i
		for !strings.Contains(body, close) && last+1 < len(lines) {
			last++
			body += "\n" + lines[last]
		}
		at := strings.Index(body, close)
		if at < 0 || strings.TrimSpace(body[at+len(close):]) != "" {
			continue
		}
		spans = append(spans, mathSpan{start: offsets[i], end: offsets[last] + len([]rune(lines[last])), source: strings.TrimSpace(body[:at])})
		i = last
	}
	return spans
}

func convertMathText(text richtext.Text) (richtext.Text, bool) {
	spans := allMathSpans(text.String())
	changed := false
	for i := len(spans) - 1; i >= 0; i-- {
		span := spans[i]
		if mathRangeProtected(text, span.start, span.end) {
			continue
		}
		text = text.Replace(span.start, span.end, mathReplacement(text, span))
		changed = true
	}
	return text, changed
}
func mathText(source string) richtext.Text {
	return richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: mathAttachment(source)}}}})
}

func (a *app) pasteMath(d *draft, data transfer.Data) bool {
	// Native rich copies already have their formula source and formatting.
	if b, err := data.Read(richtext.Format); err == nil && len(b) > 0 {
		return false
	}
	var text richtext.Text
	if b, err := data.Read(transfer.HTML); err == nil && len(b) > 0 {
		text = richtext.ParseHTML(string(b))
	} else {
		raw, err := data.Read(transfer.Markdown)
		if err != nil || len(raw) == 0 {
			raw, _ = data.Read(transfer.Text)
		}
		text = richtext.NewText(string(raw))
	}
	converted, ok := convertMathText(text)
	if !ok {
		return false
	}
	a.editorDraft = d
	a.editorHandle = d.editor
	a.urlPaste = nil
	lo, hi := d.doc.Selection()
	lo, hi = min(lo, hi), max(lo, hi)
	// Keep the display block separate from the surrounding paragraph.
	if at := converted.AttributesAt(0).Attachment; at != nil && !isInlineMath(at) && lo > 0 && d.doc.Text().Slice(lo-1, lo).String() != "\n" {
		converted = richtext.NewText("\n").Replace(1, 1, converted)
	}
	if at := converted.AttributesAt(converted.Len() - 1).Attachment; at != nil && !isInlineMath(at) && (hi == d.doc.Text().Len() || d.doc.Text().Slice(hi, hi+1).String() != "\n") {
		converted = converted.ReplaceString(converted.Len(), converted.Len(), "\n", richtext.Attributes{})
	}
	d.doc.Insert(converted)
	a.dismissSlash()
	a.changed(d)
	return true
}

// Enter after a closed display expression converts it in one undoable edit.
func (a *app) mathEnter(d *draft, key ui.Key, mods ui.Modifiers) bool {
	if key != ui.KeyEnter || mods != 0 {
		return false
	}
	lo, hi := d.doc.Selection()
	if lo != hi {
		return false
	}
	text := d.doc.Text()
	start, _ := text.ParagraphRange(hi)
	tail := strings.TrimSpace(text.Slice(start, hi).String())
	if !strings.HasSuffix(tail, `\]`) && !strings.HasSuffix(tail, "$$") {
		return false
	}
	for _, s := range displayMathSpans(text.String()) {
		if s.end != hi {
			continue
		}
		if _, ok := convertMathText(text.Slice(s.start, s.end)); !ok {
			return false
		}
		d.doc.Edit(func(e *richtext.Edit) {
			replacement := mathText(s.source)
			if s.end == text.Len() {
				replacement = replacement.ReplaceString(1, 1, "\n", richtext.Attributes{})
			}
			e.Replace(s.start, s.end, replacement)
			e.Select(s.start+2, s.start+2)
		})
		a.changed(d)
		return true
	}
	return false
}

type mathEditState struct {
	draft   *draft
	start   int
	version uint64
	source  string
	open    bool
	inline  bool
}

func (a *app) clickMath(d *draft, start int, at *richtext.Attachment) bool {
	if _, ok := mathSource(at); !ok {
		return false
	}
	a.beginMathEdit(d, start)
	return true
}

func (a *app) beginMathEdit(d *draft, start int) {
	source, ok := mathSource(d.doc.Text().AttributesAt(start).Attachment)
	if !ok {
		return
	}
	a.mathEdit = &mathEditState{draft: d, start: start, version: d.doc.Version(), source: source, open: true, inline: isInlineMath(d.doc.Text().AttributesAt(start).Attachment)}
	a.editorDraft = d
	a.editorHandle = d.editor
	a.blockMenu = false
	a.linkOpen = false
	a.urlPaste = nil
}
func (a *app) finishMathEdit(save bool) {
	s := a.mathEdit
	a.mathEdit = nil
	if s == nil {
		return
	}
	if save && s.draft.doc.Version() == s.version {
		old, ok := mathSource(s.draft.doc.Text().AttributesAt(s.start).Attachment)
		if ok && old != s.source {
			at := formulaAttachment(s.source, s.inline)
			s.draft.doc.Edit(func(e *richtext.Edit) {
				anchor, caret := e.Selection()
				attrs := e.Text().AttributesAt(s.start)
				attrs.Attachment = at
				e.Replace(s.start, s.start+1, richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement), Attributes: attrs}}}))
				e.Select(anchor, caret)
			})
			a.changed(s.draft)
		}
	}
	a.focusBlockEditor(s.draft)
}
func (a *app) insertMath(d *draft) {
	lo, hi := d.doc.Selection()
	lo = min(lo, hi)
	if start, _, ok := a.pendingCommandRange(d); ok {
		lo = start
	}
	start := lo
	if lo > 0 && d.doc.Text().Slice(lo-1, lo).String() != "\n" {
		start++
	}
	a.insertAttachment(mathAttachment(""))
	a.beginMathEdit(d, start)
}
func (a *app) mathControls(c *ui.Context, d *draft, editor ui.Element) {
	s := a.mathEdit
	if s != nil && s.draft == d && d.doc.Version() != s.version {
		a.mathEdit = nil
		s = nil
	}
	visible := false
	for _, box := range d.doc.VisibleAttachments() {
		if _, ok := mathSource(box.Attachment); !ok {
			continue
		}
		r := box.Rect
		target := ui.Box(c.Key(fmt.Sprintf("formula:%s:%d", d.page.ID, box.Start))).Absolute().
			Left(gutterWidth+r.X).Top(r.Y).Size(r.W, r.H).Label("公式").PassThrough()
		if target.Hovered() {
			target.Background(ui.RGBA(0, 0, 0, .025)).Radius(4)
		}
		if s == nil || s.draft != d || s.start != box.Start {
			continue
		}
		visible = true
		width, _ := c.Size()
		panel := ui.PopoverBase(c, target, &s.open, func(panel ui.Element) {
			panel.Label("编辑公式").Width(min(float32(520), width-32)).Padding(16).Gap(12).Background(paper).Border(1, line).Radius(10).Margin(4, 0, 0, 0).Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, .1))
			ui.Text(c, "LaTeX").FontSize(13).TextColor(muted)
			input := ui.TextArea(c, &s.source).Label("公式 LaTeX").Height(100).FontSize(14).AutoFocus().Background(sidebarColor).Radius(6)
			preview := mathAttachment(s.source)
			ui.Box(c).Label("公式预览").Height(min(preview.Height, float32(180))).FillWidth().Draw(preview.Draw)
			renderer, err := formulaRenderer()
			if err == nil && strings.TrimSpace(s.source) != "" {
				result, e := renderer.Render(context.Background(), s.source, untie.Display, untie.Style{})
				if e != nil {
					err = e
				} else if len(result.Diagnostics) > 0 {
					err = fmt.Errorf("公式包含不支持或未完成的语法")
				}
			}
			if err != nil {
				ui.Text(c, "无法完整渲染，请检查 LaTeX。原文仍可保存。").FontSize(12).TextColor(muted)
			}
			ui.Row(c).Gap(8).Justify(ui.End).AlignItems(ui.Center).Children(func() {
				if quietButton(c, "取消").Label("取消公式编辑").Clicked() {
					a.finishMathEdit(false)
				}
				if quietButton(c, "完成").Label("保存公式").Clicked() {
					a.finishMathEdit(true)
				}
			})
			input.OnTextKey(func(k ui.TextKey) bool {
				if k.Key == ui.KeyEnter && k.Mods == ui.Cmd {
					a.finishMathEdit(true)
					return true
				}
				return false
			})
			if panel.OverlayShortcut(0, ui.KeyEscape) {
				a.finishMathEdit(false)
			}
		})
		if panel.Dismissed() && a.mathEdit == s {
			a.finishMathEdit(true)
		}
	}
	if s != nil && s.draft == d && !visible {
		a.finishMathEdit(true)
	}
}
