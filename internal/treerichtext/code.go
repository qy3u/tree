package richtext

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func (v *view) codeStart(p int) bool {
	return p == 0 || !v.para(p-1).style.CodeBlock || v.para(p-1).style.CodeLanguage != v.para(p).style.CodeLanguage
}
func (v *view) codeEnd(p int) bool {
	return p+1 >= len(v.paras) || !v.para(p+1).style.CodeBlock || v.para(p+1).style.CodeLanguage != v.para(p).style.CodeLanguage
}

// Highlighting is presentation only: it never inserts formatting into the
// document, changes selection, or adds undo/save steps.
func (v *view) highlightCode(p int, original []ui.Span) []ui.Span {
	lang := editor.LanguageNamed(v.para(p).style.CodeLanguage)
	if lang == nil || lang.Tokenizer == nil {
		return original
	}
	state := v.codeState(p)
	var source strings.Builder
	for _, span := range original {
		source.WriteString(span.Text)
	}
	return codeSpans(source.String(), lang.Tokenizer, state)
}

// Cache the incoming lexical state per paragraph for this document version.
// Legacy/imported blocks may contain many paragraphs; scanning from the block
// start separately for each line would make their first render quadratic.
func (v *view) codeState(p int) uint64 {
	if v.codeStateVersion != v.version || len(v.codeStates) == 0 {
		v.codeStateVersion = v.version
		v.codeStates = []uint64{0}
	}
	for len(v.codeStates) <= p {
		i := len(v.codeStates) - 1
		state := v.codeStates[i]
		if v.codeStart(i) {
			state = 0
		}
		st := v.para(i).style
		if lang := editor.LanguageNamed(st.CodeLanguage); st.CodeBlock && lang != nil && lang.Tokenizer != nil {
			for _, line := range strings.Split(v.lineOf(i), "\u2028") {
				_, state = lang.Tokenizer.Tokenize(line, state, nil)
			}
		} else {
			state = 0
		}
		v.codeStates = append(v.codeStates, state)
	}
	if v.codeStart(p) {
		return 0
	}
	return v.codeStates[p]
}

func codeSpans(source string, tokenizer editor.Tokenizer, state uint64) []ui.Span {
	var spans []ui.Span
	for i, line := range strings.Split(source, "\u2028") {
		if i > 0 {
			spans = append(spans, ui.Span{Text: "\u2028"})
		}
		tokens, next := tokenizer.Tokenize(line, state, nil)
		state = next
		start := 0
		for _, token := range tokens {
			end := min(token.End, len(line))
			if end > start {
				spans = append(spans, ui.Span{Text: line[start:end], Color: codeColor(token.Kind)})
				start = end
			}
		}
		if start < len(line) {
			spans = append(spans, ui.Span{Text: line[start:]})
		}
	}
	return spans
}
func codeColor(kind editor.TokenKind) ui.Color {
	switch kind {
	case editor.TokenKeyword, editor.TokenTag:
		return ui.Hex("#007da5")
	case editor.TokenFunction:
		return ui.Hex("#d44369")
	case editor.TokenString, editor.TokenRegexp, editor.TokenCode:
		return ui.Hex("#31835a")
	case editor.TokenComment:
		return ui.Hex("#748394")
	case editor.TokenType, editor.TokenAttribute:
		return ui.Hex("#986801")
	case editor.TokenNumber, editor.TokenConstant, editor.TokenEscape:
		return ui.Hex("#8b55ad")
	case editor.TokenMeta, editor.TokenProperty:
		return ui.Hex("#a15c20")
	case editor.TokenOperator, editor.TokenPunctuation:
		return ui.Hex("#91918e")
	default:
		return ui.Color{}
	}
}

// CodeBlockBox includes its header and padding, but excludes page leading space.
// Start and End are rune offsets enclosing the entire code group.
type CodeBlockBox struct {
	Start, End int
	Rect       ui.Rect
	Language   string
}

func (d *Document) VisibleCodeBlocks() []CodeBlockBox {
	d.VisibleParagraphs()
	d.lock()
	v := d.shownIn.Value()
	d.mu.Unlock()
	if v == nil {
		return nil
	}
	var out []CodeBlockBox
	for i := v.first; i <= v.last && i < len(v.paras); i++ {
		if !v.para(i).style.CodeBlock {
			continue
		}
		first, last := i, i
		for !v.codeStart(first) {
			first--
		}
		for !v.codeEnd(last) {
			last++
		}
		top := v.hs.top(first)
		if first == 0 {
			top += float64(v.opts.LeadingSpace)
		}
		bottom := v.hs.top(last) + float64(v.paraLayout(last).h)
		indent := float32(v.para(first).style.Indent) * v.look.indent
		out = append(out, CodeBlockBox{Start: v.shown.paraStart(first), End: v.shown.paraStart(last) + v.para(last).n, Language: v.para(first).style.CodeLanguage,
			Rect: ui.Rect{X: v.padX + indent, Y: v.padY + float32(top-v.scroll), W: v.width - indent, H: float32(bottom - top)}})
		i = last
	}
	return out
}
