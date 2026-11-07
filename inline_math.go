package main

import (
	"sort"
	"strings"
	"unicode"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func isInlineMath(at *richtext.Attachment) bool {
	return at != nil && strings.HasPrefix(at.URL, "math-inline:")
}

func inlineMathAttachment(source string) *richtext.Attachment {
	at := &richtext.Attachment{URL: "math-inline:" + source, Name: `\(` + source + `\)`, Width: 38, Height: 20, Descent: 4}
	renderer, err := formulaRenderer()
	if err == nil && strings.TrimSpace(source) != "" {
		if object, ok := renderer.InlineMath(source, notebookFont.Size); ok {
			at.Width = max(1, object.Width) + 4
			at.Height = max(1, object.Height+object.Depth)
			at.Descent = object.Depth
			at.Draw = func(p *ui.Painter, r ui.Rect) {
				scale := r.W / at.Width
				r.X += 2 * scale
				r.W -= 4 * scale
				object.Draw(p, r)
			}
			return at
		}
	}
	at.Draw = func(p *ui.Painter, r ui.Rect) { p.Clip(r, 0, func() { p.Text(r.X+2, r.Y, "公式", 14, muted) }) }
	return at
}

func formulaAttachment(source string, inline bool) *richtext.Attachment {
	if inline {
		return inlineMathAttachment(source)
	}
	return mathAttachment(source)
}
func (s mathSpan) markup() string {
	if s.inline {
		return `\(` + s.source + `\)`
	}
	return "\\[\n" + s.source + "\n\\]"
}

// Explicit \(...\) may contain multiline TeX source; dollar formulas stay on one line.
// dollars follow Markdown's whitespace/digit rules to leave currency alone.
// Offsets are runes, including the delimiters, like the editor's selections.
func inlineMathSpans(raw string, display []mathSpan) ([]mathSpan, bool) {
	r := []rune(raw)
	blocked := make([]bool, len(r))
	for _, s := range display {
		for i := s.start; i < s.end; i++ {
			blocked[i] = true
		}
	}
	escaped := func(i int) bool {
		n := 0
		for i--; i >= 0 && r[i] == '\\'; i-- {
			n++
		}
		return n%2 != 0
	}
	// Protect fenced/indented code and Markdown code spans before looking for math.
	offset := 0
	var fence rune
	fenceSize := 0
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		rs := []rune(trimmed)
		protected := fence != 0 || strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
		if len(rs) >= 3 && (rs[0] == '`' || rs[0] == '~') {
			n := 0
			for n < len(rs) && rs[n] == rs[0] {
				n++
			}
			if n >= 3 {
				if fence == 0 {
					fence = rs[0]
					fenceSize = n
					protected = true
				} else if fence == rs[0] && n >= fenceSize && strings.TrimSpace(string(rs[n:])) == "" {
					fence = 0
				}
			}
		}
		if protected {
			for i := offset; i < offset+len([]rune(line)); i++ {
				blocked[i] = true
			}
		}
		offset += len([]rune(line)) + 1
	}
	var spans []mathSpan
	pending := false
	for i := 0; i < len(r); {
		if blocked[i] {
			i++
			continue
		}
		if r[i] == 'h' && (strings.HasPrefix(string(r[i:]), "https://") || strings.HasPrefix(string(r[i:]), "http://")) {
			for i < len(r) && !unicode.IsSpace(r[i]) {
				i++
			}
			continue
		}
		if r[i] == '`' && !escaped(i) {
			n := 1
			for i+n < len(r) && r[i+n] == '`' {
				n++
			}
			end := -1
			for j := i + n; j < len(r) && r[j] != '\n'; j++ {
				if r[j] == '`' {
					count := 1
					for j+count < len(r) && r[j+count] == '`' {
						count++
					}
					if count == n {
						end = j + count
						break
					}
					j += count - 1
				}
			}
			if end >= 0 {
				i = end
				continue
			}
			for i < len(r) && r[i] != '\n' {
				i++
			}
			continue
		}
		paren := r[i] == '\\' && !escaped(i) && i+1 < len(r) && r[i+1] == '('
		dollar := r[i] == '$' && !escaped(i)
		if dollar && ((i > 0 && r[i-1] == '$') || (i+1 < len(r) && r[i+1] == '$')) {
			i++
			continue
		}
		if !paren && !dollar {
			i++
			continue
		}
		n := 1
		if paren {
			n = 2
		}
		if dollar && i+n < len(r) && unicode.IsSpace(r[i+n]) {
			i++
			continue
		}
		end := -1
		endDelimiter := 1
		if paren {
			endDelimiter = 2
		}
		for j := i + n; j < len(r) && (paren || (r[j] != '\n' && r[j] != '\u2028')); j++ {
			if blocked[j] {
				break
			}
			if escaped(j) {
				continue
			}
			if paren && r[j] == '\\' && j+1 < len(r) && r[j+1] == ')' {
				end = j
				break
			}
			if dollar && r[j] == '$' {
				if j == i+n || unicode.IsSpace(r[j-1]) || (j+1 < len(r) && (unicode.IsDigit(r[j+1]) || r[j+1] == '$')) {
					break
				}
				end = j
				break
			}
		}
		if end >= 0 {
			source := string(r[i+n : end])
			if strings.TrimSpace(source) != "" {
				spans = append(spans, mathSpan{start: i, end: end + endDelimiter, source: source, inline: true})
			}
			i = end + endDelimiter
			pending = false
		} else {
			// Only the current line can contain an unfinished formula.
			if !strings.ContainsAny(string(r[i:]), "\n\u2028") {
				pending = true
			}
			i += n
		}
	}
	return spans, pending
}
func allMathSpans(raw string) []mathSpan {
	display := displayMathSpans(raw)
	inline, _ := inlineMathSpans(raw, display)
	spans := append(display, inline...)
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	return spans
}
func mathRangeProtected(text richtext.Text, start, end int) bool {
	for pos := start; pos < end; pos++ {
		a := text.AttributesAt(pos)
		if text.ParagraphStyleAt(pos).CodeBlock || a.Code || a.Attachment != nil || a.Link != "" {
			return true
		}
	}
	return false
}
func mathReplacement(text richtext.Text, s mathSpan) richtext.Text {
	attrs := text.AttributesAt(s.start)
	attrs.Attachment = formulaAttachment(s.source, s.inline)
	attrs.Link = ""
	attrs.Tag = ""
	attrs.Code = false
	return richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement), Attributes: attrs}}})
}

// This runs only after committed typing, never on undo, cursor movement or
// paste-as-plain-text. It also shields unfinished TeX underscores from Markdown.
func (a *app) typedMath(d *draft, inserted string) bool {
	lo, hi := d.doc.Selection()
	if lo != hi {
		return false
	}
	text := d.doc.Text()
	start, _ := text.ParagraphRange(hi)
	if text.ParagraphStyleAt(start).CodeBlock || text.AttributesAt(max(start, hi-1)).Code {
		return false
	}
	prefix := text.Slice(start, hi).String()
	if strings.HasPrefix(strings.TrimSpace(prefix), `\[`) || strings.HasPrefix(strings.TrimSpace(prefix), "$$") {
		return true
	}
	spans, pending := inlineMathSpans(prefix, nil)
	if len(spans) == 0 {
		return pending
	}
	// Only consider formulas completed by this text insertion.
	insertedStart := hi - len([]rune(inserted))
	changed := false
	d.doc.Edit(func(e *richtext.Edit) {
		caret := hi
		for i := len(spans) - 1; i >= 0; i-- {
			s := spans[i]
			s.start += start
			s.end += start
			if s.end <= insertedStart || mathRangeProtected(text, s.start, s.end) {
				continue
			}
			e.Replace(s.start, s.end, mathReplacement(text, s))
			caret -= s.end - s.start - 1
			changed = true
		}
		if changed {
			e.Select(caret, caret)
		}
	})
	if changed {
		a.changed(d)
	}
	return changed || pending
}
