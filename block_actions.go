package main

import (
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func editBlock(doc *richtext.Document, index int, action string) {
	var paras []richtext.Paragraph
	for _, p := range doc.Text().Paragraphs() {
		paras = append(paras, p)
	}
	if index < 0 || index >= len(paras) {
		return
	}
	switch action {
	case "delete":
		paras = append(paras[:index], paras[index+1:]...)
	case "duplicate":
		copy := paras[index]
		copy.Style.ID = newID()
		copy.Runs = append([]richtext.Run(nil), copy.Runs...)
		paras = append(paras, richtext.Paragraph{})
		copyIndex := index + 1
		for i := len(paras) - 1; i > copyIndex; i-- {
			paras[i] = paras[i-1]
		}
		paras[copyIndex] = copy
		index = copyIndex
	case "before", "after":
		if action == "after" {
			index++
		}
		paras = append(paras, richtext.Paragraph{})
		for i := len(paras) - 1; i > index; i-- {
			paras[i] = paras[i-1]
		}
		paras[index] = richtext.Paragraph{Style: richtext.ParagraphStyle{ID: newID()}}
	default:
		return
	}
	if len(paras) == 0 {
		paras = []richtext.Paragraph{{Style: richtext.ParagraphStyle{ID: newID()}}}
	}
	index = min(index, len(paras)-1)
	text := richtext.TextOf(paras...)
	pos := 0
	for i := 0; i < index; i++ {
		pos += paragraphLength(paras[i]) + 1
	}
	doc.Edit(func(e *richtext.Edit) {
		e.Replace(0, e.Text().Len(), text)
		for start, p := range text.Paragraphs() {
			st := p.Style
			e.FormatParagraphs(start, start, func(s *richtext.ParagraphStyle) { *s = st })
		}
		e.Select(pos, pos)
	})
}
func (a *app) blockActions(c *ui.Context, d *draft, index int) {
	run := func(label, action string) {
		if quietButton(c, label).KeepFocus().Height(28).Shrink(0).Clicked() {
			editBlock(d.doc, index, action)
			a.changed(d)
			a.blockMenu = false
			a.editorHandle.Focus()
		}
	}
	run("在上方插入", "before")
	run("在下方插入", "after")
	run("复制块", "duplicate")
	if quietButton(c, "复制块链接").KeepFocus().Height(28).Shrink(0).Clicked() {
		if link, err := a.copyBlockLink(d, index); err != nil {
			a.report(err)
		} else {
			c.WriteClipboard(link)
		}
		a.blockMenu = false
		a.editorHandle.Focus()
	}
	ui.Box(c).Height(1).Background(line).Margin(4, 0)
	for _, cmd := range blockCommands {
		if cmd.ID != "text" && cmd.ID != "h1" && cmd.ID != "h2" && cmd.ID != "h3" && cmd.ID != "todo" && cmd.ID != "bullet" && cmd.ID != "number" && cmd.ID != "quote" && cmd.ID != "code" {
			continue
		}
		if quietButton(c, "转为"+cmd.Label).KeepFocus().Height(28).Shrink(0).Clicked() {
			// The menu belongs to this block, independently of editor focus
			// or a selection changed while the popup was open.
			convertBlock(d.doc, index, cmd.Style)
			a.changed(d)
			a.editorHandle.Focus()
			a.blockMenu = false
		}
	}
	ui.Box(c).Height(1).Background(line).Margin(4, 0)
	run("删除块", "delete")
}

func paragraphStart(text richtext.Text, index int) (int, bool) {
	i := 0
	for start := range text.Paragraphs() {
		if i == index {
			return start, true
		}
		i++
	}
	return 0, false
}

func (a *app) blockMenuPanel(c *ui.Context, handle ui.Element, d *draft, index int) {
	_, height := c.Size()
	ui.PopoverBase(c.Key("block-menu"), handle, &a.blockMenu, func(panel ui.Element) {
		panel.Label("段落操作菜单").Width(204).Padding(4).Radius(8).Background(paper).Border(1, line).
			Shadow(0, 5, 18, 0, ui.RGBA(0, 0, 0, .12)).
			AttachTo(handle, ui.AnchorTopEnd, ui.AnchorTopStart).Margin(0, 6)
		ui.Scroll(c).Label("段落操作列表").Height(min(float32(466), height-32)).Children(func() {
			a.blockActions(c, d, index)
			for _, item := range []struct {
				label string
				delta int
			}{{"段落上移", -1}, {"段落下移", 1}} {
				destination := index + item.delta
				if quietButton(c, item.label).KeepFocus().Height(28).Shrink(0).
					Disabled(destination < 0 || destination >= d.doc.Text().ParagraphCount()).Clicked() {
					if moveParagraph(d.doc, index, destination) {
						a.changed(d)
					}
					a.blockMenu = false
					a.editorHandle.Focus()
				}
			}
		})
	})
}

// A type change preserves identity, nesting and inline content. Choosing the
// existing type must not uncheck a task or reset a code block's language.
func convertBlock(doc *richtext.Document, index int, style richtext.ParagraphStyle) {
	start, ok := paragraphStart(doc.Text(), index)
	if !ok {
		return
	}
	doc.Edit(func(e *richtext.Edit) {
		e.FormatParagraphs(start, start, func(p *richtext.ParagraphStyle) {
			style.ID, style.Indent, style.Align = p.ID, p.Indent, p.Align
			if style.List == richtext.Tasks && p.List == richtext.Tasks {
				style.Checked = p.Checked
			}
			if style.CodeBlock && p.CodeBlock {
				style.CodeLanguage = p.CodeLanguage
			}
			*p = style
		})
		e.Select(start, start)
	})
}
