package main

import (
	"strings"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

type linkEditState struct {
	draft           *draft
	start, end      int
	version         uint64
	original, title string
	rect            ui.Rect
}

func (a *app) beginLinkEdit(d *draft) {
	text := d.doc.Text()
	lo, hi := d.doc.Selection()
	lo, hi = min(lo, hi), max(lo, hi)
	if lo == hi {
		i := lo
		if (i == text.Len() || text.AttributesAt(i).Link == "") && i > 0 {
			i--
		}
		if i < text.Len() && text.AttributesAt(i).Link != "" {
			url := text.AttributesAt(i).Link
			lo, hi = i, i+1
			for lo > 0 && text.AttributesAt(lo-1).Link == url {
				lo--
			}
			for hi < text.Len() && text.AttributesAt(hi).Link == url {
				hi++
			}
		}
	}
	a.linkURL = d.doc.Formatting().Link
	if lo < hi {
		a.linkURL = text.AttributesAt(lo).Link
	}
	title := text.Slice(lo, hi).String()
	a.linkEdit = linkEditState{draft: d, start: lo, end: hi, version: d.doc.Version(), original: title, title: title, rect: d.doc.CaretRect()}
	d.doc.SetSelection(lo, hi)
	a.linkOpen = true
}

func (a *app) finishLinkEdit(remove, focus bool) {
	s := a.linkEdit
	a.linkOpen = false
	if s.draft == nil || s.draft.doc.Version() != s.version {
		return
	}
	url := strings.TrimSpace(a.linkURL)
	title := s.title
	if remove {
		url = ""
		title = s.original
	}
	if title == "" {
		title = s.original
	}
	if title == "" {
		title = url
	}
	d := s.draft
	d.doc.Edit(func(e *richtext.Edit) {
		end := s.end
		if title != s.original {
			attrs := e.Text().AttributesAt(s.start)
			attrs.Link = url
			e.ReplaceString(s.start, s.end, title, attrs)
			end = s.start + len([]rune(title))
		} else {
			e.Format(s.start, end, func(attrs *richtext.Attributes) { attrs.Link = url })
		}
		e.Select(end, end)
	})
	a.changed(d)
	if focus {
		d.editor.Focus()
	}
}

func (a *app) linkEditView(c *ui.Context, d *draft, editor ui.Element) {
	s := &a.linkEdit
	if d.doc.Version() != s.version {
		a.linkOpen = false
		return
	}
	r := s.rect
	target := ui.Box(c.Key("link-edit-anchor:"+d.page.ID)).AttachTo(editor, ui.AnchorTopStart, ui.AnchorTopStart).
		Left(r.X).Top(r.Y).Size(max(r.W, 1), max(r.H, 1)).PassThrough()
	width, _ := c.Size()
	panel := ui.PopoverBase(c, target, &a.linkOpen, func(panel ui.Element) {
		panel.Label("编辑链接浮层").Width(min(float32(354), width-32)).Padding(12).Gap(10).
			Background(paper).Border(1, line).Radius(12).Margin(4, 0, 0, 0).Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, .1))
		ui.Text(c, "Page or URL").FontSize(13).FontWeight(600).TextColor(ui.Hex("#858480"))
		url := ui.TextInput(c, &a.linkURL).Label("链接地址").Placeholder("https://…").Height(32).FontSize(15).
			Background(sidebarColor).Border(1, ui.Hex("#e5e4e2")).Radius(7)
		ui.Text(c, "Link title").FontSize(13).FontWeight(600).TextColor(ui.Hex("#858480")).Margin(8, 0, 0, 0)
		title := ui.TextInput(c, &s.title).Label("链接文字").Height(32).FontSize(15).
			Background(sidebarColor).Border(1, ui.Hex("#e5e4e2")).Radius(7).AutoFocus()
		ui.Box(c).Height(1).Background(line).Margin(2, 0, 0, 0)
		remove := quietButton(c, "").Label("移除链接").Height(28).Padding(0, 2).Gap(10).Children(func() {
			ui.Icon(c, icons["trash"]).FontSize(18)
			ui.Text(c, "Remove link").FontSize(15)
		})
		if remove.Clicked() {
			a.finishLinkEdit(true, true)
		} else if url.Submitted() || title.Submitted() {
			a.finishLinkEdit(false, true)
		}
		if panel.OverlayShortcut(0, ui.KeyEscape) {
			a.linkOpen = false
			d.editor.Focus()
		}
	})
	if panel.Dismissed() {
		a.finishLinkEdit(false, false)
	}
}
