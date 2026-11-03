package main

import (
	"encoding/json"
	"fmt"

	"github.com/egoist/mygo/ui"
)

// Each tab owns its navigation history. Drafts remain shared by page ID so
// opening a page twice never creates competing copies of its contents.
type pageTab struct {
	ID       string   `json:"id"`
	History  []string `json:"history"`
	Position int      `json:"position"`
}

func (t pageTab) page() string { return t.History[t.Position] }

type tabSession struct {
	Tabs   []pageTab `json:"tabs"`
	Active int       `json:"active"`
}

func (a *app) restoreTabs() {
	var saved tabSession
	if json.Unmarshal([]byte(a.store.Setting("tabs")), &saved) == nil && len(saved.Tabs) > 0 {
		a.tabs, a.activeTab = saved.Tabs, saved.Active
	} else {
		a.tabs = []pageTab{{ID: newID(), History: []string{a.selected}}}
	}
	a.pruneTabs()
	a.loadVisits()
	a.selected = a.tabs[a.activeTab].page()
	a.revealTab = true
}
func (a *app) pruneTabs() {
	seen := map[string]bool{}
	for i := range a.tabs {
		t := &a.tabs[i]
		if t.ID == "" || seen[t.ID] {
			t.ID = newID()
		}
		seen[t.ID] = true
		position := -1
		var history []string
		for j, id := range t.History {
			if p := a.find(id); id != "" && (p == nil || p.Deleted != "") {
				continue
			}
			history = append(history, id)
			if j <= t.Position {
				position = len(history) - 1
			}
		}
		if len(history) == 0 {
			history = []string{""}
		}
		t.History, t.Position = history, max(0, position)
	}
	if len(a.tabs) == 0 {
		a.tabs = []pageTab{{ID: newID(), History: []string{""}}}
	}
	a.activeTab = max(0, min(a.activeTab, len(a.tabs)-1))
}
func (a *app) showTab() {
	a.showPage(a.tabs[a.activeTab].page())
	a.recordVisit(a.selected)
	a.revealTab = true
	data, err := json.Marshal(tabSession{a.tabs, a.activeTab})
	if err == nil {
		err = a.store.SetSetting("tabs", string(data))
	}
	a.report(err)
	a.report(a.store.SetSetting("selected", a.selected))
}
func (a *app) saveBeforeTabChange() bool {
	if err := a.flush(); err != nil {
		a.report(err)
		return false
	}
	return true
}
func (a *app) openTab(id string) {
	if id != "" {
		if p := a.find(id); p == nil || p.Deleted != "" {
			return
		}
	}
	if !a.saveBeforeTabChange() {
		return
	}
	a.tabs = append(a.tabs, pageTab{ID: newID(), History: []string{id}})
	a.activeTab = len(a.tabs) - 1
	a.showTab()
}
func (a *app) switchTab(index int) {
	if index < 0 || index >= len(a.tabs) || index == a.activeTab || !a.saveBeforeTabChange() {
		return
	}
	a.activeTab = index
	a.showTab()
}
func (a *app) closeTab(index int) {
	if index < 0 || index >= len(a.tabs) || !a.saveBeforeTabChange() {
		return
	}
	a.tabs = append(a.tabs[:index], a.tabs[index+1:]...)
	if index < a.activeTab {
		a.activeTab--
	}
	if len(a.tabs) == 0 {
		a.tabs = []pageTab{{ID: newID(), History: []string{""}}}
	}
	a.activeTab = min(a.activeTab, len(a.tabs)-1)
	a.showTab()
}
func (a *app) canNavigate(delta int) bool {
	t := a.tabs[a.activeTab]
	next := t.Position + delta
	return next >= 0 && next < len(t.History)
}
func (a *app) navigateTab(delta int) {
	if !a.canNavigate(delta) || !a.saveBeforeTabChange() {
		return
	}
	a.tabs[a.activeTab].Position += delta
	a.showTab()
}
func (a *app) tabBar(c *ui.Context) {
	reveal := a.revealTab
	a.revealTab = false
	height := max(c.TitleBar().Height, float32(36))
	width, _ := c.Size()
	tabsWidth := min(float32(len(a.tabs))*190, width-sidebarWidth-1-69-42)
	ui.Row(c).Height(height).Shrink(0).Background(sidebarColor).DragWindow().AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Padding(0, 6).AlignItems(ui.Center).Children(func() {
			back := iconButton(c, "back", "后退").Disabled(!a.canNavigate(-1))
			if !a.canNavigate(-1) {
				back.Opacity(.35)
			}
			if back.Clicked() {
				a.navigateTab(-1)
			}
			forward := iconButton(c, "forward", "前进").Disabled(!a.canNavigate(1))
			if !a.canNavigate(1) {
				forward.Opacity(.35)
			}
			if forward.Clicked() {
				a.navigateTab(1)
			}
		})
		ui.Box(c).Width(1).Background(line)
		ui.ScrollHorizontal(c).Width(tabsWidth).Shrink(0).Children(func() {
			ui.Row(c).Height(height).Children(func() {
				for i, t := range a.tabs {
					label := "新标签页"
					if p := a.find(t.page()); p != nil {
						label = pageTitle(p)
					}
					active := i == a.activeTab
					tab := ui.Row(c.Key(t.ID)).Width(190).Height(height).Shrink(0).AlignItems(ui.Center)
					if active {
						tab.Background(paper)
					}
					tab.Children(func() {
						color := muted
						if active {
							color = ink
						}
						if quietButton(c.Key("select"), "").Label(fmt.Sprintf("切换标签页 %d：%s", i+1, label)).Grow(1).MinWidth(0).Height(height).Radius(0).Padding(0, 14).Gap(6).AlignItems(ui.Center).Children(func() {
							if p := a.find(t.page()); p != nil && coloredPageIcon(p.Body.Icon) != nil {
								pageIcon(c, p, 15)
							}
							ui.Text(c, label).FontSize(13).TextColor(color).SingleLine().MinWidth(0)
						}).Clicked() {
							a.switchTab(i)
						}
						if len(a.tabs) > 1 {
							close := iconButton(c.Key("close"), "close", fmt.Sprintf("关闭标签页 %d", i+1)).Size(22, 22).Padding(4).Margin(0, 5, 0, 0)
							if !active && !tab.Hovered() {
								close.Opacity(0)
							}
							if close.Clicked() {
								a.closeTab(i)
							}
						}
						ui.Box(c).Width(1).Height(height).Background(line)
						if !active {
							ui.Box(c).Absolute().Bottom(0).Left(0).Right(0).Height(1).Background(line)
						}
					})
					if active && reveal {
						tab.ScrollIntoView()
					}
				}
			})
		})
		if iconButton(c, "plus", "新建标签页").Size(42, height).Shrink(0).Radius(0).Clicked() {
			a.showTabPicker()
		}
		ui.Box(c).Grow(1)
	})
}
func (a *app) newTabView(c *ui.Context) {
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(72, 48).Children(func() {
		ui.Column(c).MaxWidth(600).Gap(8).Children(func() {
			ui.Text(c, "新标签页").FontSize(28).Bold().Margin(0, 0, 20, 0)
			if sidebarAction(c, "search", "搜索页面", "标签页搜索", "⌘P").Clicked() {
				a.showSearch()
			}
			if sidebarAction(c, "plus", "新建页面", "标签页新建页面", "⌘N").Clicked() {
				a.addPage("")
			}
			ui.Text(c, "页面").FontSize(12).TextColor(muted).Margin(20, 10, 6, 10)
			for _, p := range a.pages {
				if p.Deleted == "" {
					if quietButton(c.Key(p.ID), "").Label("标签页打开：" + pageTitle(p)).Height(32).Gap(10).AlignItems(ui.Center).Children(func() { pageIcon(c, p, 18); ui.Text(c, pageTitle(p)).FontSize(14) }).Clicked() {
						a.selectPage(p.ID)
					}
				}
			}
		})
	})
}
