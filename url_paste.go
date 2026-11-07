package main

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

type urlPasteState struct {
	draft      *draft
	url        string
	start, end int
	version    uint64
	selected   int
	open       bool
}

func pastedWebURL(data transfer.Data) string {
	raw := ""
	if b, err := data.Read(transfer.Text); err == nil {
		raw = string(b)
	} else if b, err := data.Read(transfer.HTML); err == nil {
		raw = richtext.ParseHTML(string(b)).PlainText()
	}
	if strings.TrimSpace(raw) == "" {
		if urls, err := data.URLs(); err == nil && len(urls) == 1 {
			raw = urls[0]
		}
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || !ui.SafeToOpen(raw) {
		return ""
	}
	return raw
}

func (a *app) pasteWebURL(d *draft, data transfer.Data) bool {
	raw := pastedWebURL(data)
	if raw == "" {
		return false
	}
	a.editorDraft = d
	a.editorHandle = d.editor
	lo, hi := d.doc.Selection()
	lo, hi = min(lo, hi), max(lo, hi)
	attrs := d.doc.Text().AttributesAt(lo)
	attrs.Link, attrs.Tag, attrs.Attachment = raw, "", nil
	d.doc.Edit(func(e *richtext.Edit) {
		e.ReplaceString(lo, hi, raw, attrs)
		e.Select(lo+len([]rune(raw)), lo+len([]rune(raw)))
	})
	a.dismissSlash()
	a.urlPaste = &urlPasteState{draft: d, url: raw, start: lo, end: lo + len([]rune(raw)), version: d.doc.Version(), selected: 1, open: true}
	a.changed(d)
	return true
}

func (a *app) validURLPaste(d *draft) bool {
	s := a.urlPaste
	if s == nil || s.draft != d {
		return false
	}
	lo, hi := d.doc.Selection()
	if !s.open || d.doc.Version() != s.version || lo != s.end || hi != s.end {
		a.urlPaste = nil
		return false
	}
	return true
}

func (a *app) chooseURLPaste(link bool) {
	s := a.urlPaste
	if s == nil || !a.validURLPaste(s.draft) {
		return
	}
	d := s.draft
	if !link {
		d.doc.Edit(func(e *richtext.Edit) {
			e.Format(s.start, s.end, func(at *richtext.Attributes) { at.Link = ""; at.Tag = ""; at.Underline = false })
			e.Select(s.end, s.end)
		})
		a.changed(d)
	}
	a.urlPaste = nil
	a.focusBlockEditor(d)
}

func (a *app) urlPasteKey(d *draft, key ui.Key, mods ui.Modifiers) bool {
	if !a.validURLPaste(d) {
		return false
	}
	if mods != 0 {
		a.urlPaste = nil
		return false
	}
	switch key {
	case ui.KeyUp, ui.KeyDown:
		a.urlPaste.selected = 1 - a.urlPaste.selected
		return true
	case ui.KeyEnter, ui.KeyTab:
		a.chooseURLPaste(a.urlPaste.selected == 1)
		return true
	case ui.KeyEscape:
		a.urlPaste = nil
		return true
	}
	a.urlPaste = nil
	return false
}

func (a *app) urlPasteView(c *ui.Context, d *draft, editor ui.Element) {
	if !a.validURLPaste(d) {
		return
	}
	if a.blockMenu || a.linkOpen || a.searchOpen || a.insertOpen {
		a.urlPaste = nil
		return
	}
	s := a.urlPaste
	r := d.doc.CaretRect()
	anchor := ui.Box(c.Key("paste-url-anchor:"+d.page.ID)).AttachTo(editor, ui.AnchorTopStart, ui.AnchorTopStart).
		Left(r.X).Top(r.Y).Size(1, r.H).PassThrough()
	ui.PopoverBase(c, anchor, &s.open, func(panel ui.Element) {
		panel.Label("URL 粘贴方式").Width(210).Padding(5).Gap(2).Radius(12).Background(paper).Border(1, line).
			Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, .08)).Margin(4, 0, 0, 0)
		ui.Text(c, "Paste as").FontSize(12).FontWeight(600).TextColor(ui.Hex("#858480")).Padding(6, 8, 8, 8)
		for i, label := range []string{"Plain Text", "URL"} {
			row := quietButton(c, label).Label("粘贴为 "+label).KeepFocus().Height(30).Padding(4, 8).FontSize(14).Radius(6)
			if i == s.selected || row.Hovered() {
				row.Background(ui.Hex("#f3f2f1"))
			}
			if row.Clicked() {
				a.chooseURLPaste(i == 1)
			}
		}
	})
	if !s.open && a.urlPaste == s {
		a.urlPaste = nil
	}
}
