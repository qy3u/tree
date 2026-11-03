package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
)

// A navigation destination, never a page ID or a row in the notes database.
const libraryRoute = "tree:library"

func (a *app) loadFavorites() {
	a.favorites = map[string]time.Time{}
	_ = json.Unmarshal([]byte(a.store.Setting("favorites")), &a.favorites)
	if a.favorites == nil {
		a.favorites = map[string]time.Time{}
	}
}

func (a *app) isFavorite(id string) bool {
	_, ok := a.favorites[id]
	return ok
}

func (a *app) favoriteLabel(id string) string {
	if a.isFavorite(id) {
		return "取消收藏"
	}
	return "添加到收藏"
}

func (a *app) toggleFavorite(id string) {
	if p := a.find(id); p == nil || p.Deleted != "" {
		return
	}
	// Persist before changing the UI; a failed write leaves the old state intact.
	next := make(map[string]time.Time, len(a.favorites)+1)
	for key, value := range a.favorites {
		next[key] = value
	}
	if a.isFavorite(id) {
		delete(next, id)
	} else {
		next[id] = time.Now()
	}
	data, err := json.Marshal(next)
	if err == nil {
		err = a.store.SetSetting("favorites", string(data))
	}
	if err != nil {
		a.report(err)
		return
	}
	a.favorites = next
}

func (a *app) favoriteButton(c *ui.Context, id string) ui.Element {
	icon := "star"
	if a.isFavorite(id) {
		icon = "star-filled"
	}
	button := iconButton(c, icon, a.favoriteLabel(id)).KeepFocus()
	if a.isFavorite(id) {
		button.TextColor(ui.Hex("#b48b43"))
	}
	if button.Clicked() {
		a.toggleFavorite(id)
	}
	return button
}

func (a *app) libraryPages() []*Page {
	var pages []*Page
	for _, p := range a.pages {
		if p.Deleted != "" {
			continue
		}
		if a.libraryFavorites {
			if !a.isFavorite(p.ID) {
				continue
			}
		} else if a.visited[p.ID].IsZero() {
			continue
		}
		pages = append(pages, p)
	}
	sort.SliceStable(pages, func(i, j int) bool {
		left, right := a.visited[pages[i].ID], a.visited[pages[j].ID]
		if a.libraryFavorites {
			left, right = a.favorites[pages[i].ID], a.favorites[pages[j].ID]
		}
		if !left.Equal(right) {
			return left.After(right)
		}
		return pages[i].ID < pages[j].ID
	})
	return pages
}

func lastEditedLabel(updated string, now time.Time) string {
	at, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return "—"
	}
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	}
	at = at.In(time.Local)
	if at.Year() == now.In(time.Local).Year() {
		return at.Format("1月2日")
	}
	return at.Format("2006年1月2日")
}

func (a *app) libraryView(c *ui.Context) {
	w, _ := c.Size()
	available := w - sidebarWidth - 1
	padding := max(float32(24), min(float32(88), available*.07))
	contentWidth := available - padding*2
	sourceWidth := contentWidth * .27
	nameWidth := contentWidth - sourceWidth - 140
	ui.Scroll(c.Key("library")).Label("Library 页面").Grow(1).MinHeight(0).Padding(52, padding, 40, padding).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Margin(0, 0, 20, 0).Children(func() {
			ui.Text(c, "Library").FontSize(32).Bold().Grow(1)
			if ui.Button(c, "新建页面").Label("Library 新建页面").FontSize(14).FontWeight(500).Height(32).Padding(0, 14).Radius(6).Background(ui.Hex("#2e87dc")).TextColor(paper).Border(0, line).Clicked() {
				a.addPage("")
			}
		})
		ui.Row(c).Gap(6).Margin(0, 0, 12, 0).Children(func() {
			for i, item := range []struct{ icon, label string }{{"clock", "最近访问"}, {"star", "收藏"}} {
				active := a.libraryFavorites == (i == 1)
				button := quietButton(c.Key(item.icon), "").Label("Library "+item.label).
					Height(34).Padding(0, 13).Radius(17).Gap(8).AlignItems(ui.Center)
				color := ui.Hex("#85847f")
				if active {
					button.Background(ui.Hex("#f3f2f1"))
					color = ink
				}
				button.Children(func() {
					ui.Icon(c, icons[item.icon]).FontSize(18).TextColor(color)
					ui.Text(c, item.label).FontSize(14).FontWeight(500).TextColor(color)
				})
				if button.Clicked() {
					a.libraryFavorites = i == 1
				}
			}
		})
		heading := func(icon, title string, width float32) {
			col := ui.Row(c).Height(36).Padding(0, 8).Gap(7).AlignItems(ui.Center)
			if width > 0 {
				col.Width(width).Shrink(0)
			} else {
				col.Grow(1).MinWidth(0)
			}
			col.Children(func() {
				ui.Icon(c, icons[icon]).FontSize(16).TextColor(muted)
				ui.Text(c, title).FontSize(13).TextColor(ui.Hex("#85847f")).SingleLine().MinWidth(0)
			})
		}
		ui.Row(c).Children(func() {
			heading("page", "页面名称", nameWidth)
			heading("library", "来源", sourceWidth)
			heading("clock", "最后编辑时间", 0)
		})
		ui.Box(c).Height(1).Shrink(0).Background(line)
		pages := a.libraryPages()
		now := time.Now()
		for _, p := range pages {
			row := ui.Row(c.Key(p.ID)).Height(40).Shrink(0).AlignItems(ui.Center)
			if row.Hovered() {
				row.Background(ui.Hex("#f8f7f6"))
			}
			row.Children(func() {
				ui.Row(c).Width(nameWidth).Shrink(0).AlignItems(ui.Center).Children(func() {
					open := quietButton(c.Key("open"), "").Label("Library 打开："+pageTitle(p)).
						Height(40).Grow(1).MinWidth(0).Padding(0, 8).Gap(9).AlignItems(ui.Center)
					open.Children(func() {
						pageIcon(c, p, 20)
						ui.Text(c, pageTitle(p)).FontSize(14).FontWeight(500).SingleLine().MinWidth(0)
					})
					open.ContextMenu(func(m *ui.Menu) { a.pageActions(m, p.ID) })
					if open.Clicked() {
						a.selectPage(p.ID)
					}
					star := a.favoriteButton(c.Key("favorite"), p.ID).Label("Library "+a.favoriteLabel(p.ID)+"："+pageTitle(p)).Shrink(0).Margin(0, 6, 0, 0)
					if !a.isFavorite(p.ID) && !row.Hovered() && !star.Focused() {
						star.Opacity(0)
					}
				})
				ui.Row(c).Width(sourceWidth).Shrink(0).Padding(0, 8).AlignItems(ui.Center).Children(func() {
					if parent := a.find(p.Parent); parent != nil && parent.Deleted == "" {
						if quietButton(c, "").Label("Library 来源：" + pageTitle(p)).Height(32).Padding(0).Gap(7).MinWidth(0).Grow(1).AlignItems(ui.Center).Children(func() {
							pageIcon(c, parent, 18)
							ui.Text(c, pageTitle(parent)).FontSize(13).Underline().SingleLine().MinWidth(0)
						}).Clicked() {
							a.selectPage(parent.ID)
						}
					} else {
						ui.Text(c, "顶层页面").FontSize(13).TextColor(muted)
					}
				})
				ui.Text(c, lastEditedLabel(p.Updated, now)).FontSize(13).SingleLine().MinWidth(0).Grow(1).Padding(0, 8)
			})
			ui.Box(c).Height(1).Shrink(0).Background(line)
		}
		if len(pages) == 0 {
			message := "还没有最近访问的页面"
			if a.libraryFavorites {
				message = "还没有收藏的页面"
			}
			ui.Text(c, message).FontSize(14).TextColor(muted).Padding(36, 8)
		}
	})
}
