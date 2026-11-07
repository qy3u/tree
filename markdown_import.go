package main

import (
	"github.com/egoist/mygo/treerichtext"
	"strings"
)

func markdownCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	var cells []string
	var cell strings.Builder
	escaped := false
	for _, r := range line {
		if escaped {
			cell.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '|' {
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
		} else {
			cell.WriteRune(r)
		}
	}
	if escaped {
		cell.WriteByte('\\')
	}
	cells = append(cells, strings.TrimSpace(cell.String()))
	return cells
}
func tableSeparator(line string) bool {
	if !strings.Contains(line, "|") {
		return false
	}
	for _, cell := range markdownCells(line) {
		cell = strings.Trim(cell, ": ")
		if len(cell) < 3 || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}
func parseMarkdownBody(md string) Body {
	b := emptyBody()
	mathTokens := map[string]mathSpan{}
	spans := allMathSpans(md)
	runes := []rune(md)
	for i := len(spans) - 1; i >= 0; i-- {
		span := spans[i]
		token := "TREEMATH" + newID() + "END"
		mathTokens[token] = span
		replacement := token
		if !span.inline {
			replacement = "\n" + token + "\n"
		}
		runes = append(append(append([]rune{}, runes[:span.start]...), []rune(replacement)...), runes[span.end:]...)
	}
	md = string(runes)
	lines := strings.Split(md, "\n")
	var out []string
	tokens := map[string]string{}
	fence := ""
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			kind := trimmed[:3]
			if fence == "" {
				fence = kind
			} else if fence == kind {
				fence = ""
			}
			out = append(out, lines[i])
			continue
		}
		if fence == "" && i+1 < len(lines) && strings.Contains(lines[i], "|") && tableSeparator(lines[i+1]) {
			cells := [][]string{markdownCells(lines[i])}
			cols := len(cells[0])
			i++
			for i+1 < len(lines) && strings.Contains(lines[i+1], "|") && strings.TrimSpace(lines[i+1]) != "" {
				i++
				r := markdownCells(lines[i])
				for len(r) < cols {
					r = append(r, "")
				}
				cells = append(cells, r[:cols])
			}
			id := newID()
			token := "TREETABLE" + id
			tokens[token] = id
			b.Tables[id] = Table{Cells: cells}
			out = append(out, "", token, "")
			continue
		}
		out = append(out, lines[i])
	}
	text := richtext.ParseMarkdown(strings.Join(out, "\n"))
	var paras []richtext.Paragraph
	for _, para := range text.Paragraphs() {
		if len(para.Runs) == 1 {
			if id, ok := tokens[para.Runs[0].Text]; ok {
				para.Runs = []richtext.Run{{Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: tableAttachment(id, b.Tables[id])}}}
			}
		}
		paras = append(paras, para)
	}
	b.Text = richtext.TextOf(paras...)
	for token, span := range mathTokens {
		for {
			raw := b.Text.String()
			pos := strings.Index(raw, token)
			if pos < 0 {
				break
			}
			start := len([]rune(raw[:pos]))
			end := start + len(token)
			if mathRangeProtected(b.Text, start, end) {
				b.Text = b.Text.ReplaceString(start, end, span.markup(), b.Text.AttributesAt(start))
			} else {
				span.start, span.end = start, end
				b.Text = b.Text.Replace(start, end, mathReplacement(b.Text, span))
			}
		}
		// Table cells remain textual; never leave internal parser placeholders in them.
		for id, table := range b.Tables {
			for i, row := range table.Cells {
				for j, cell := range row {
					table.Cells[i][j] = strings.ReplaceAll(cell, token, span.markup())
				}
			}
			b.Tables[id] = table
		}
	}
	return b
}
