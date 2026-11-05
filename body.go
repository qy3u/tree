package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

type Table struct {
	Cells [][]string `json:"cells"`
}
type Body struct {
	Leaves    map[string]Leaf              `json:"leaves,omitempty"`
	Icon      string                       `json:"icon,omitempty"`
	Schema    int                          `json:"schema"`
	Text      richtext.Text                `json:"text"`
	Tables    map[string]Table             `json:"tables,omitempty"`
	TaskTimes map[string][]TaskTimeSegment `json:"taskTimes,omitempty"`
}

func emptyBody() Body { return Body{Schema: 1, Text: richtext.NewText(""), Tables: map[string]Table{}} }
func bodyFromMarkdown(md string) Body {
	return parseMarkdownBody(md)
}
func (t Table) markdown() string {
	if len(t.Cells) == 0 {
		return ""
	}
	cols := 0
	for _, r := range t.Cells {
		cols = max(cols, len(r))
	}
	if cols == 0 {
		return ""
	}
	var b strings.Builder
	row := func(r []string) {
		b.WriteString("|")
		for col := 0; col < cols; col++ {
			v := ""
			if col < len(r) {
				v = r[col]
			}
			v = strings.ReplaceAll(v, "\\", "\\\\")
			v = strings.ReplaceAll(v, "|", "\\|")
			v = strings.ReplaceAll(v, "\n", "<br>")
			b.WriteString(" " + v + " |")
		}
		b.WriteByte('\n')
	}
	row(t.Cells[0])
	b.WriteString("|" + strings.Repeat(" --- |", cols) + "\n")
	for _, r := range t.Cells[1:] {
		row(r)
	}
	return strings.TrimSpace(b.String())
}
func tableAttachment(id string, t Table) *richtext.Attachment {
	cols := 1
	for _, row := range t.Cells {
		cols = max(cols, len(row))
	}
	rows := max(1, len(t.Cells))
	name := "表格"
	if len(t.Cells) > 0 {
		name = "表格：" + strings.Join(t.Cells[0], " · ")
	}
	return &richtext.Attachment{URL: "table:" + id, Name: name, Width: 560, Height: float32(rows*38 + 28), Draw: func(p *ui.Painter, r ui.Rect) {
		theme := p.Theme()
		rh := (r.H - 28) / float32(rows)
		cw := r.W / float32(cols)
		p.Fill(r, theme.Background, 2)
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: rh}, theme.Surface, 2)
		for i := 0; i <= rows; i++ {
			p.Fill(ui.Rect{X: r.X, Y: r.Y + float32(i)*rh, W: r.W, H: 1}, theme.Border, 0)
		}
		for j := 0; j <= cols; j++ {
			p.Fill(ui.Rect{X: r.X + float32(j)*cw, Y: r.Y, W: 1, H: r.H}, theme.Border, 0)
		}
		for i, row := range t.Cells {
			for j, cell := range row {
				rect := ui.Rect{X: r.X + float32(j)*cw + 8, Y: r.Y + float32(i)*rh + 7, W: cw - 16, H: rh - 12}
				p.Clip(rect, 0, func() { p.Text(rect.X, rect.Y, cell, 13, theme.Text) })
			}
		}
	}}
}
func dividerAttachment() *richtext.Attachment {
	return &richtext.Attachment{Name: "分隔线", URL: "divider:", Width: 560, Height: 24, Draw: func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H/2, W: r.W, H: 1}, p.Theme().Border, 0)
	}}
}
func hydrate(b Body, dir string) richtext.Text {
	var paras []richtext.Paragraph
	for _, para := range b.Text.Paragraphs() {
		for i, run := range para.Runs {
			at := run.Attributes.Attachment
			if at == nil {
				continue
			}
			copy := *at
			switch {
			case strings.HasPrefix(at.URL, "leaf:"):
				copy = *leafAttachment(strings.TrimPrefix(at.URL, "leaf:"))
			case strings.HasPrefix(at.URL, "table:"):
				if table, ok := b.Tables[strings.TrimPrefix(at.URL, "table:")]; ok {
					copy = *tableAttachment(strings.TrimPrefix(at.URL, "table:"), table)
				}
			case at.URL == "divider:":
				copy = *dividerAttachment()
			case strings.HasPrefix(at.URL, "asset:"):
				name := strings.TrimPrefix(at.URL, "asset:")
				if filepath.Base(name) == name {
					if data, err := os.ReadFile(filepath.Join(dir, "assets", name)); err == nil {
						copy.Image, _ = ui.DecodeBitmap(data)
					}
				}
			}
			para.Runs[i].Attributes.Attachment = &copy
		}
		paras = append(paras, para)
	}
	return richtext.TextOf(paras...)
}
func (b Body) Markdown() string {
	replacements := map[string]string{}
	var paras []richtext.Paragraph
	for _, p := range b.Text.Paragraphs() {
		for i, r := range p.Runs {
			at := r.Attributes.Attachment
			if at == nil {
				continue
			}
			if strings.HasPrefix(at.URL, "leaf:") {
				token := "TREELEAF" + newID() + "END"
				leaf := b.Leaves[strings.TrimPrefix(at.URL, "leaf:")]
				p.Runs[i] = richtext.Run{Text: token}
				replacements[token] = "## " + leafDisplayTitle(leaf.Title) + "\n\n" + leaf.Content.Markdown()
			} else if strings.HasPrefix(at.URL, "table:") || at.URL == "divider:" {
				token := "TREEBLOCK" + newID() + "END"
				value := "---"
				if at.URL != "divider:" {
					value = b.Tables[strings.TrimPrefix(at.URL, "table:")].markdown()
				}
				p.Runs[i] = richtext.Run{Text: token}
				replacements[token] = value
			} else if strings.HasPrefix(at.URL, "asset:") {
				copy := *at
				copy.URL = "assets/" + strings.TrimPrefix(at.URL, "asset:")
				p.Runs[i].Attributes.Attachment = &copy
			}
		}
		paras = append(paras, p)
	}
	md := strings.ReplaceAll(richtext.TextOf(paras...).Markdown(), "\u2028", "  \n")
	for token, value := range replacements {
		md = strings.ReplaceAll(md, token, value)
	}
	return md
}
func (b Body) PlainText() string {
	var s strings.Builder
	s.WriteString(b.Text.PlainText())
	for _, run := range b.Text.Runs(0, b.Text.Len()) {
		at := run.Attributes.Attachment
		if at != nil && strings.HasPrefix(at.URL, "leaf:") {
			leaf := b.Leaves[strings.TrimPrefix(at.URL, "leaf:")]
			s.WriteString(" " + leaf.Title + " " + leaf.Content.PlainText())
		}
		if at == nil || !strings.HasPrefix(at.URL, "table:") {
			continue
		}
		t := b.Tables[strings.TrimPrefix(at.URL, "table:")]
		for _, r := range t.Cells {
			s.WriteString(" " + strings.Join(r, " "))
		}
	}
	return s.String()
}
func copyTable(t Table) Table {
	var out Table
	for _, row := range t.Cells {
		out.Cells = append(out.Cells, append([]string(nil), row...))
	}
	return out
}

// Move a whole paragraph in one undo step. Selection and IDs follow the text.
func moveParagraph(doc *richtext.Document, from, to int) bool {
	text := doc.Text()
	var paras []richtext.Paragraph
	for _, p := range text.Paragraphs() {
		paras = append(paras, p)
	}
	if from < 0 || to < 0 || from >= len(paras) || to >= len(paras) || from == to {
		return false
	}
	p := paras[from]
	paras = append(paras[:from], paras[from+1:]...)
	paras = append(paras, nilParagraph())
	copy(paras[to+1:], paras[to:])
	paras[to] = p
	newText := richtext.TextOf(paras...)
	start := 0
	for i := 0; i < to; i++ {
		for _, r := range paras[i].Runs {
			start += len([]rune(r.Text))
		}
		start++
	}
	doc.Edit(func(e *richtext.Edit) {
		e.Replace(0, e.Text().Len(), newText)
		for pos, para := range newText.Paragraphs() {
			style := para.Style
			e.FormatParagraphs(pos, pos, func(st *richtext.ParagraphStyle) { *st = style })
		}
		e.Select(start, start)
	})
	return true
}
func nilParagraph() richtext.Paragraph { return richtext.Paragraph{} }
