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
	}
	if len(paras) == 0 {
		paras = []richtext.Paragraph{{Style: richtext.ParagraphStyle{ID: newID()}}}
	}
	index = min(index, len(paras)-1)
	text := richtext.TextOf(paras...)
	pos, _ := text.ParagraphRange(index)
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
		if quietButton(c, label).Clicked() {
			editBlock(d.doc, index, action)
			a.changed(d)
			a.blockMenu = false
			a.editorHandle.Focus()
		}
	}
	run("在上方插入", "before")
	run("在下方插入", "after")
	run("复制块", "duplicate")
	if quietButton(c, "复制块链接").Clicked() {
		if link, err := a.copyBlockLink(d, index); err != nil {
			a.report(err)
		} else {
			c.WriteClipboard(link)
		}
		a.blockMenu = false
	}
	ui.Box(c).Height(1).Background(line).Margin(4, 0)
	for _, cmd := range blockCommands {
		if cmd.ID != "text" && cmd.ID != "h1" && cmd.ID != "h2" && cmd.ID != "h3" && cmd.ID != "todo" && cmd.ID != "bullet" && cmd.ID != "number" && cmd.ID != "quote" && cmd.ID != "code" {
			continue
		}
		if quietButton(c, "转为"+cmd.Label).Clicked() {
			a.runBlockCommand(d, cmd, false)
			a.blockMenu = false
		}
	}
	ui.Box(c).Height(1).Background(line).Margin(4, 0)
	run("删除块", "delete")
}
