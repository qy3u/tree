package main

import (
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

type linkHover struct {
	hit      richtext.LinkHit
	version  uint64
	scroll   float32
	lastOver time.Time
	copied   bool
}

func (a *app) linkHoverView(c *ui.Context, d *draft, editor ui.Element) {
	if a.linkOpen && a.linkEdit.draft == d {
		a.linkEditView(c, d, editor)
		return
	}
	h := &d.linkHover
	anchor, caret := d.doc.Selection()
	if a.urlPaste != nil || a.linkOpen || a.searchOpen || a.blockMenu || a.insertOpen || a.pageMenu || a.errorOpen || a.tableOpen || a.moveOpen || a.trashOpen || anchor != caret {
		*h = linkHover{}
		return
	}
	now := c.Now()
	// Discard stale ranges after edits or scrolling instead of editing the wrong text.
	if h.hit.URL != "" && (h.version != d.doc.Version() || h.scroll != d.doc.ScrollOffset()) {
		*h = linkHover{}
	}
	x, y, over := editor.PointerPosition()
	if over && editor.Hovered() {
		if hit, ok := d.doc.LinkAt(x, y); ok {
			if h.hit.URL != hit.URL || h.hit.Start != hit.Start {
				h.copied = false
			}
			h.hit, h.version, h.scroll, h.lastOver = hit, d.doc.Version(), d.doc.ScrollOffset(), now
		}
	}
	if h.hit.URL == "" {
		return
	}
	hit := h.hit
	target := ui.Box(c.Key("link-anchor:"+d.page.ID)).AttachTo(editor, ui.AnchorTopStart, ui.AnchorTopStart).Left(hit.Rect.X).Top(hit.Rect.Y).Size(hit.Rect.W, hit.Rect.H).PassThrough()
	ui.Overlay(c, func() {
		width, _ := c.Size()
		panel := ui.Row(c.Key("link-hover:"+d.page.ID)).Label("链接浮层").
			AttachTo(target, ui.AnchorBottomStart, ui.AnchorTopStart).Margin(4, 0, 0, 0).
			Width(min(float32(360), width-32)).Padding(4, 6).Gap(5).AlignItems(ui.Center).
			Background(paper).Border(1, line).Radius(11).Shadow(0, 3, 12, 0, ui.RGBA(0, 0, 0, .08))
		_, _, inside := panel.PointerPosition()
		if inside {
			h.lastOver = now
		}
		// A short grace period bridges the gap between the text and its popover.
		if now.Sub(h.lastOver) > 250*time.Millisecond {
			*h = linkHover{}
			panel.Hidden(true)
			return
		}
		c.After(260 * time.Millisecond)
		panel.Children(func() {
			ui.Icon(c, icons["globe"]).FontSize(16).TextColor(muted).Shrink(0)
			ui.Text(c, hit.URL).Label("原始链接：" + hit.URL).FontSize(13).TextColor(ui.Hex("#858480")).Grow(1).MinWidth(0).MaxLines(1)
			icon := "copy"
			if h.copied {
				icon = "check"
			}
			if iconButton(c, icon, "复制链接地址").KeepFocus().Shrink(0).Clicked() {
				c.Services().WriteClipboard(hit.URL)
				h.copied = true
			}
			if quietButton(c, "Edit").Label("编辑链接地址").KeepFocus().Shrink(0).Padding(4, 6).Clicked() {
				a.editorDraft = d
				a.editorHandle = d.editor
				d.doc.SetSelection(hit.Start, hit.End)
				a.openLink()
				a.linkEdit.rect = hit.Rect
				*h = linkHover{}
			}
		})
	})
}
