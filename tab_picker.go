package main

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

type tabPickerState struct {
	pointerX, pointerY float32
	Open               bool
	Query              string
	Selected           int
	previewID          string
	preview            *richtext.Document
}

func (a *app) loadVisits() {
	a.visited = map[string]time.Time{}
	_ = json.Unmarshal([]byte(a.store.Setting("pageVisits")), &a.visited)
	if a.visited == nil {
		a.visited = map[string]time.Time{}
	}
}
func (a *app) recordVisit(id string) {
	if id == "" {
		return
	}
	if a.visited == nil {
		a.visited = map[string]time.Time{}
	}
	a.visited[id] = time.Now()
	data, err := json.Marshal(a.visited)
	if err == nil {
		err = a.store.SetSetting("pageVisits", string(data))
	}
	a.report(err)
}
func (a *app) showTabPicker() {
	a.searchOpen = false
	a.pageMenu, a.insertOpen, a.blockMenu = false, false, false
	a.dismissSlash()
	a.tabPicker = tabPickerState{Open: true}
}
func (a *app) pageBody(p *Page) Body {
	body := p.Body
	if d := a.drafts[p.ID]; d != nil {
		body.Text = d.doc.Text()
	}
	return body
}
func (a *app) tabPickerPages() []*Page {
	return a.matchingPages(a.tabPicker.Query)
}

// Both page choosers use live drafts and the same recent-visit order.
func (a *app) matchingPages(query string) []*Page {
	query = strings.ToLower(strings.TrimSpace(query))
	var pages []*Page
	for _, p := range a.pages {
		if p.Deleted != "" {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(a.pagePath(p)+" "+a.pageBody(p).PlainText()), query) {
			continue
		}
		pages = append(pages, p)
	}
	sort.SliceStable(pages, func(i, j int) bool {
		vi, vj := a.visited[pages[i].ID], a.visited[pages[j].ID]
		if !vi.Equal(vj) {
			return vi.After(vj)
		}
		if pages[i].Updated != pages[j].Updated {
			return pages[i].Updated > pages[j].Updated
		}
		return pages[i].ID < pages[j].ID
	})
	return pages
}
func visitGroup(visited, now time.Time) string {
	if visited.IsZero() {
		return "其他页面"
	}
	now = now.In(time.Local)
	visited = visited.In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if !visited.Before(today) {
		return "今天"
	}
	if !visited.Before(today.AddDate(0, 0, -1)) {
		return "昨天"
	}
	if !visited.Before(today.AddDate(0, 0, -30)) {
		return "过去 30 天"
	}
	return "更早"
}
func (a *app) chooseTabPage(p *Page) {
	if !a.saveBeforeTabChange() {
		a.tabPicker.Open = false
		return
	}
	if p == nil {
		created, err := a.store.Create("", "", emptyBody())
		if err != nil {
			a.tabPicker.Open = false
			a.report(err)
			return
		}
		if err = a.reload(); err != nil {
			a.tabPicker.Open = false
			a.report(err)
			return
		}
		p = created
		a.openTab(p.ID)
		a.focusTitlePage = p.ID
	} else {
		a.openTab(p.ID)
	}
	a.tabPicker.Open = false
}
func (a *app) tabPickerDialog(c *ui.Context) {
	ui.DialogBase(c.Key("tab-picker"), &a.tabPicker.Open, func(backdrop, panel ui.Element) {
		w, h := c.Size()
		x, y, _ := panel.PointerPosition()
		pointerMoved := x != a.tabPicker.pointerX || y != a.tabPicker.pointerY
		a.tabPicker.pointerX, a.tabPicker.pointerY = x, y
		width, height := min(float32(1000), w-64), min(float32(650), h-100)
		backdrop.Background(ui.Transparent)
		panel.Label("在新标签页中打开").Width(width).Height(height).Padding(0).Gap(0).Radius(18).Background(paper).Border(1, line).Shadow(0, 12, 36, 0, ui.RGBA(0, 0, 0, .18))
		var search ui.Element
		ui.Row(c).Height(58).Shrink(0).Padding(0, 18).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons["search"]).FontSize(21).TextColor(muted)
			search = ui.TextInputBase(c, &a.tabPicker.Query).Label("新标签页搜索").Placeholder("在新标签页中打开…").FontSize(17).Grow(1).MinWidth(0).FocusRing(false).AutoFocus()
		})
		pages := a.tabPickerPages()
		reveal := false
		if search.Changed() {
			a.tabPicker.Selected = 0
			if strings.TrimSpace(a.tabPicker.Query) != "" && len(pages) > 0 {
				a.tabPicker.Selected = 1
			}
			reveal = true
		}
		a.tabPicker.Selected = min(a.tabPicker.Selected, len(pages))
		if search.Shortcut(0, ui.KeyDown) {
			a.tabPicker.Selected = min(len(pages), a.tabPicker.Selected+1)
			reveal = true
		}
		if search.Shortcut(0, ui.KeyUp) {
			a.tabPicker.Selected = max(0, a.tabPicker.Selected-1)
			reveal = true
		}
		var selected *Page
		if a.tabPicker.Selected > 0 {
			selected = pages[a.tabPicker.Selected-1]
		}
		if search.Submitted() || search.Shortcut(ui.Cmd, ui.KeyEnter) {
			a.chooseTabPage(selected)
			return
		}
		ui.Row(c).Grow(1).MinHeight(0).Padding(0, 14, 12, 14).Gap(16).AlignItems(ui.Stretch).Children(func() {
			ui.Scroll(c).Grow(1).MinWidth(0).Children(func() {
				drawRow := func(index int, p *Page) {
					title, icon, label := "创建新页面", "compose", "标签页创建新页面"
					path := ""
					if p != nil {
						title, icon, label = pageTitle(p), "page", "标签页打开："+pageTitle(p)
						if parent := a.find(p.Parent); parent != nil {
							path = a.pagePath(parent)
						}
					}
					key := "create"
					if p != nil {
						key = p.ID
					}
					row := quietButton(c.Key(key), "").Label(label).Height(38).Shrink(0).Padding(0, 10).Radius(8).Gap(10).KeepFocus().AlignItems(ui.Center)
					if a.tabPicker.Selected == index {
						row.Background(line)
					}
					row.Children(func() {
						if p != nil {
							pageIcon(c, p, 19)
						} else {
							ui.Icon(c, icons[icon]).FontSize(19).TextColor(muted)
						}
						ui.Text(c, title).FontSize(14).FontWeight(500).SingleLine().MinWidth(0)
						if path != "" {
							ui.Text(c, "— "+path).FontSize(12).TextColor(muted).SingleLine().MinWidth(0).Grow(1)
						} else {
							ui.Box(c).Grow(1)
						}
						if a.tabPicker.Selected == index {
							ui.Text(c, "↵").FontSize(14).TextColor(muted)
						}
					})
					if row.Hovered() && pointerMoved && !reveal {
						a.tabPicker.Selected = index
					}
					if reveal && a.tabPicker.Selected == index {
						row.ScrollIntoView()
					}
					if row.Clicked() {
						a.chooseTabPage(p)
					}
				}
				drawRow(0, nil)
				lastGroup := ""
				now := time.Now()
				for i, p := range pages {
					group := visitGroup(a.visited[p.ID], now)
					if strings.TrimSpace(a.tabPicker.Query) != "" {
						group = "搜索结果"
					}
					if group != lastGroup {
						ui.Text(c, group).FontSize(12).TextColor(ui.Hex("#85847f")).Height(38).Padding(12, 10, 6, 10).Shrink(0)
						lastGroup = group
					}
					drawRow(i+1, p)
				}
				if len(pages) == 0 {
					ui.Text(c, "没有找到匹配的页面").FontSize(13).TextColor(muted).Padding(20, 10)
				}
			})
			// Preview uses a separate read-only document; it cannot change the page's
			// caret, scroll position, undo history, or contents.
			previewWidth := width * .37
			ui.Column(c).Width(previewWidth).Shrink(0).Margin(22, 0, 0, 0).Radius(12).Border(1, line).Background(paper).Clip().Children(func() {
				ui.Box(c).Height(64).Shrink(0).Background(sidebarColor)
				title, path, icon := "创建新页面", "Tree", "compose"
				var p *Page
				if a.tabPicker.Selected > 0 && a.tabPicker.Selected <= len(pages) {
					p = pages[a.tabPicker.Selected-1]
					title, path, icon = pageTitle(p), a.pagePath(p), "page"
				}
				ui.Column(c).Padding(18, 22).Gap(12).Children(func() {
					if p != nil {
						pageIcon(c, p, 30)
					} else {
						ui.Icon(c, icons[icon]).FontSize(30).TextColor(ink)
					}
					ui.Text(c, path).FontSize(11).TextColor(muted).SingleLine().MinWidth(0)
					ui.Text(c, title).Label("标签页预览标题").FontSize(22).Bold()
				})
				if p == nil {
					ui.Text(c, "创建一篇新的本地笔记").FontSize(14).TextColor(muted).Padding(0, 22)
				} else {
					if a.tabPicker.preview == nil || a.tabPicker.previewID != p.ID {
						a.tabPicker.previewID = p.ID
						a.tabPicker.preview = richtext.NewDocument(pasteReferenceStyles(hydrate(a.pageBody(p), a.store.dir)))
					}
					richtext.EditorBase(c.Key("preview:"+p.ID), a.tabPicker.preview, richtext.EditorOptions{Label: "页面内容预览", TagStyle: referenceSpanStyle, DrawAttachment: a.drawPageReferenceIcon, ReadOnly: true, Font: ui.Font{Size: 13, LineHeight: 1.6}, Theme: &notebookTheme, OnLink: func(string) {}}).Grow(1).MinHeight(0).Margin(0, 22, 20, 22).FocusRing(false)
				}
			})
		})
	})
}
