package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *app) showSearch() {
	a.tabPicker.Open = false
	a.pageMenu, a.insertOpen, a.blockMenu = false, false, false
	a.dismissSlash()
	a.query = ""
	a.searchSelection = 0
	a.searchPointerX, a.searchPointerY = -1, -1
	a.searchOpen = true
}

func (a *app) searchDialog(c *ui.Context) {
	ui.DialogBase(c.Key("page-search"), &a.searchOpen, func(backdrop, panel ui.Element) {
		w, h := c.Size()
		width, height := min(float32(720), w-64), min(float32(580), h-96)
		x, y, _ := panel.PointerPosition()
		pointerMoved := x != a.searchPointerX || y != a.searchPointerY
		a.searchPointerX, a.searchPointerY = x, y
		backdrop.Background(ui.Transparent)
		panel.Label("搜索窗口").Width(width).Height(height).Padding(0).Gap(0).
			Radius(18).Background(paper).Border(1, line).
			Shadow(0, 12, 36, 0, ui.RGBA(0, 0, 0, .18))
		var input ui.Element
		ui.Row(c).Height(64).Shrink(0).Padding(0, 22).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons["search"]).FontSize(21).TextColor(muted)
			input = ui.TextInputBase(c, &a.query).Label("搜索关键词").
				Placeholder("搜索页面标题与正文…").PlaceholderColor(ui.Hex("#aaa9a5")).
				FontSize(17).TextColor(ink).Grow(1).MinWidth(0).FocusRing(false).AutoFocus()
		})
		query := strings.TrimSpace(a.query)
		pages := a.matchingPages(query)
		reveal := input.Changed()
		if reveal {
			a.searchSelection = 0
		}
		a.searchSelection = max(0, min(a.searchSelection, len(pages)-1))
		if input.Shortcut(0, ui.KeyDown) {
			a.searchSelection = max(0, min(len(pages)-1, a.searchSelection+1))
			reveal = true
		}
		if input.Shortcut(0, ui.KeyUp) {
			a.searchSelection = max(0, a.searchSelection-1)
			reveal = true
		}
		if input.Submitted() && len(pages) > 0 {
			a.openSearchResult(pages[a.searchSelection].ID, query)
			return
		}
		ui.Scroll(c.Key("search-results")).Label("搜索结果列表").Grow(1).MinHeight(0).Padding(0, 14, 14, 14).Children(func() {
			lastGroup := ""
			now := time.Now()
			for i, p := range pages {
				group := visitGroup(a.visited[p.ID], now)
				if query != "" {
					group = "搜索结果"
				}
				if group != lastGroup {
					ui.Text(c, group).FontSize(12).FontWeight(500).TextColor(ui.Hex("#85847f")).
						Height(38).Padding(12, 10, 6, 10).Shrink(0)
					lastGroup = group
				}
				path, snippet := "", ""
				if parent := a.find(p.Parent); parent != nil {
					path = a.pagePath(parent)
				}
				// Only show a short matching excerpt when the title did not match.
				// Empty searches remain a compact list of recently visited pages.
				if query != "" {
					body := a.pageBody(p).PlainText()
					if at, _ := textMatch(pageTitle(p), query); at < 0 {
						if at, _ := textMatch(body, query); at >= 0 {
							snippet = matchSnippet(body, query)
						}
					}
				}
				rowHeight := float32(38)
				if snippet != "" {
					rowHeight = 58
				}
				row := quietButton(c.Key("result:"+p.ID), "").Label("搜索结果："+pageTitle(p)).
					Height(rowHeight).Shrink(0).Padding(0, 10).Radius(8).Gap(12).KeepFocus().AlignItems(ui.Center)
				if a.searchSelection == i {
					row.Background(line)
				}
				row.Children(func() {
					pageIcon(c, p, 20)
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Text(c, pageTitle(p)).FontSize(14).FontWeight(500).TextColor(ui.Hex("#5f5e5a")).SingleLine().MinWidth(0).MaxWidth(width * .55)
							if path != "" {
								ui.Text(c, "— "+path).FontSize(12).TextColor(muted).SingleLine().Grow(1).MinWidth(0)
							}
						})
						if snippet != "" {
							ui.Text(c, snippet).FontSize(12).TextColor(muted).SingleLine().MinWidth(0)
						}
					})
				})
				if row.Hovered() && pointerMoved && !reveal {
					a.searchSelection = i
				}
				if reveal && a.searchSelection == i {
					row.ScrollIntoView()
				}
				if row.Clicked() {
					a.openSearchResult(p.ID, query)
				}
			}
			if len(pages) == 0 {
				ui.Column(c).Padding(44, 20).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icons["search"]).FontSize(28).TextColor(muted)
					ui.Text(c, "没有找到匹配的页面").FontSize(14).TextColor(muted)
				})
			}
		})
	})
}

func (a *app) pagePath(p *Page) string {
	var parts []string
	for p != nil {
		parts = append([]string{pageTitle(p)}, parts...)
		p = a.find(p.Parent)
	}
	return strings.Join(parts, " / ")
}
func textMatch(text, query string) (int, int) {
	r := []rune(text)
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return -1, -1
	}
	for i := 0; i+len(q) <= len(r); i++ {
		if strings.ToLower(string(r[i:i+len(q)])) == string(q) {
			return i, i + len(q)
		}
	}
	return -1, -1
}
func matchSnippet(text, query string) string {
	r := []rune(strings.ReplaceAll(text, "\n", " "))
	start, end := textMatch(string(r), query)
	if start < 0 {
		start = 0
		end = 0
	}
	lo, hi := max(0, start-24), min(len(r), end+60)
	s := string(r[lo:hi])
	if lo > 0 {
		s = "…" + s
	}
	if hi < len(r) {
		s += "…"
	}
	return s
}
func (a *app) openSearchResult(id, query string) {
	a.selectPage(id)
	a.searchOpen = false
	d := a.current()
	if d == nil {
		return
	}
	start, end := textMatch(d.doc.Text().String(), query)
	if start < 0 {
		for pos, para := range d.doc.Text().Paragraphs() {
			for _, run := range para.Runs {
				at := run.Attributes.Attachment
				if at != nil && strings.HasPrefix(at.URL, "table:") {
					t := d.page.Body.Tables[strings.TrimPrefix(at.URL, "table:")]
					if x, _ := textMatch(t.markdown(), query); x >= 0 {
						start, end = pos, pos+1
					}
				}
			}
		}
	}
	if start >= 0 {
		d.doc.SetSelection(start, end)
		a.editorHandle.Focus()
	} else {
		a.titleHandle.Focus()
	}
}
