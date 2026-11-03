package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

type blockJump struct {
	page, block string
	until       time.Time
}

func referenceURL(page, block string) string {
	return (&url.URL{Scheme: "tree", Host: "page", Path: "/" + page, Fragment: block}).String()
}

func parseReference(raw string) (page, block string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "tree" || u.Host != "page" || u.User != nil || u.RawQuery != "" {
		return "", "", false
	}
	page = strings.TrimPrefix(u.Path, "/")
	if page == "" || strings.Contains(page, "/") {
		return "", "", false
	}
	return page, u.Fragment, true
}

func blockPosition(text richtext.Text, id string) (int, bool) {
	for pos, p := range text.Paragraphs() {
		if p.Style.ID == id {
			return pos, true
		}
	}
	return 0, false
}

// Persist the target before exposing its ID to another page or the clipboard.
func (a *app) copyBlockLink(d *draft, index int) (string, error) {
	i := 0
	for _, p := range d.doc.Text().Paragraphs() {
		if i == index {
			if err := a.flush(); err != nil {
				return "", err
			}
			return referenceURL(d.page.ID, p.Style.ID), nil
		}
		i++
	}
	return "", fmt.Errorf("这个内容块已不存在")
}

func (a *app) referenceText(p *Page) richtext.Text {
	if d := a.drafts[p.ID]; d != nil {
		return d.doc.Text()
	}
	return p.Body.Text
}

func (a *app) pasteReference(raw string) (richtext.Text, bool) {
	page, block, ok := parseReference(raw)
	if !ok {
		return richtext.Text{}, false
	}
	label := "无法找到的页面"
	if block != "" {
		label = "无法找到的内容块"
	}
	if p := a.find(page); p != nil {
		label = pageTitle(p)
		text := a.referenceText(p)
		if pos, found := blockPosition(text, block); block != "" && found {
			_, end := text.ParagraphRange(pos)
			summary := strings.Join(strings.Fields(text.Slice(pos, end).PlainText()), " ")
			if summary == "" {
				summary = "内容块"
			}
			chars := []rune(summary)
			if len(chars) > 36 {
				summary = string(chars[:36]) + "…"
			}
			label = summary
		} else if block != "" {
			label = "已删除的内容块"
		}
	}
	return pasteReferenceStyles(richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: label, Attributes: richtext.Attributes{Link: referenceURL(page, block), Tag: referenceTag}}}})), true
}

// Consume all Tree URLs locally, including malformed ones; never send them
// to the OS. A missing target must not silently land on a different block.
func (a *app) openReference(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "tree" {
		return false
	}
	page, block, ok := parseReference(raw)
	if !ok {
		a.report(fmt.Errorf("这个 Tree 链接无效"))
		return true
	}
	p := a.find(page)
	if p == nil || p.Deleted != "" {
		a.report(fmt.Errorf("引用的页面已删除或不存在"))
		return true
	}
	if block == "" {
		a.selectPage(page)
		if a.selected == page && !a.errorOpen {
			a.blockJump = blockJump{}
		}
		return true
	}
	pos, found := blockPosition(a.referenceText(p), block)
	if !found {
		a.report(fmt.Errorf("引用的内容块已删除或不存在"))
		return true
	}
	a.selectPage(page)
	if a.selected != page || a.errorOpen {
		return true
	}
	d := a.current()
	// Loading an older page can restyle references before this block, changing
	// rune offsets. Resolve again in the loaded document, always by stable ID.
	pos, found = blockPosition(d.doc.Text(), block)
	if !found {
		a.report(fmt.Errorf("引用的内容块已删除或不存在"))
		return true
	}
	d.doc.SetSelection(pos, pos)
	a.blockJump = blockJump{page: page, block: block}
	return true
}

func (a *app) blockReferenceHighlight(c *ui.Context, d *draft, editor ui.Element) {
	jump := &a.blockJump
	id := ""
	if jump.page == d.page.ID {
		if jump.until.IsZero() {
			jump.until = c.Now().Add(1600 * time.Millisecond)
			editor.Focus()
		}
		if c.Now().Before(jump.until) {
			id = jump.block
			c.After(jump.until.Sub(c.Now()))
		} else {
			*jump = blockJump{}
		}
	}
	editor.DrawOver(func(p *ui.Painter, r ui.Rect) {
		paintTaskBoxes(p, r, d.doc)
		if id == "" {
			return
		}
		p.Clip(r, 0, func() {
			text := d.doc.Text()
			for _, box := range d.doc.VisibleParagraphs() {
				if text.ParagraphStyleAt(box.Start).ID != id {
					continue
				}
				p.Fill(ui.Rect{X: r.X, Y: r.Y + box.Top, W: r.W, H: box.Height}, ui.RGBA(51, 125, 168, .12), 4)
			}
		})
	})
}
