package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

var paper = ui.Hex("#ffffff")
var sidebarColor = ui.Hex("#f8f7f6")
var ink = ui.Hex("#2f2f2d")
var muted = ui.Hex("#999894")
var line = ui.Hex("#eeedeb")

const sidebarWidth float32 = 252
const gutterWidth float32 = 44

func quietButton(c *ui.Context, label string) ui.Element {
	return ui.Button(c, label).Background(ui.Transparent).Border(0, line).Radius(5).TextColor(ink).FontSize(13).FontWeight(400).Padding(6, 8).Justify(ui.Start)
}

func (a *app) view(c *ui.Context) {
	theme := ui.LightTheme()
	theme.Background, theme.Text, theme.Surface, theme.Border = paper, ink, sidebarColor, line
	theme.SurfaceHover = ui.Hex("#efeeec")
	c.SetTheme(theme)
	if c.Shortcut(ui.Cmd, ui.KeyT) {
		a.showTabPicker()
	}
	if c.Shortcut(ui.Cmd, ui.KeyW) {
		a.closeTab(a.activeTab)
	}
	if c.Shortcut(ui.Ctrl, ui.KeyTab) {
		a.switchTab((a.activeTab + 1) % len(a.tabs))
	}
	if c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyTab) {
		a.switchTab((a.activeTab + len(a.tabs) - 1) % len(a.tabs))
	}
	if c.Shortcut(ui.Cmd, ui.KeyN) {
		a.addPage("")
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) {
		a.addPage(a.selected)
	}
	if c.Shortcut(ui.Cmd, ui.KeyP) {
		a.searchOpen = true
	}
	if c.Shortcut(ui.Cmd, ui.KeyS) {
		a.report(a.flush())
	}
	ui.Row(c).Fill().Background(paper).AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c)
		ui.Box(c).Width(1).Background(line)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			a.tabBar(c)
			d := a.current()
			if d == nil {
				a.newTabView(c)
				return
			}
			a.header(c, d)
			a.documentView(c, d)
			a.changed(d)
		})
	})
	a.dialogs(c)
}

func (a *app) sidebar(c *ui.Context) {
	ui.Column(c).Width(sidebarWidth).Shrink(0).Background(sidebarColor).Children(func() {
		bar := c.TitleBar()
		ui.Row(c).Height(max(bar.Height, 36)).Padding(0, 12, 0, max(14, bar.Left+8)).DragWindow().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Tree").FontSize(14).Bold()
		})
		ui.Column(c).Padding(8, 8, 0, 8).Gap(2).Children(func() {
			if sidebarAction(c, "search", "搜索", "搜索页面", "⌘P").Clicked() {
				a.searchOpen = true
			}
			if sidebarAction(c, "plus", "新建页面", "新建页面", "").Clicked() {
				a.addPage("")
			}
		})
		rootTarget := ui.Row(c).Height(44).Padding(16, 20, 4, 20).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "页面").FontSize(12).TextColor(muted)
		})
		if _, over := ui.DragOver[pageDrag](rootTarget); over {
			rootTarget.Background(line)
		}
		if id, ok := ui.Drop[pageDrag](rootTarget); ok {
			a.mutate(func() error { return a.store.Move(string(id), "") })
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 8).Children(func() {
			treeTheme := *c.Theme()
			treeTheme.Background, treeTheme.Accent, treeTheme.AccentText = sidebarColor, ui.Hex("#efeeec"), ink
			treeTheme.SurfaceHover = ui.Hex("#f0efed")
			ui.Themed(c, &treeTheme).Children(func() { ui.Tree(c, func() { a.tree(c, "", 0) }) })
		})
		ui.Column(c).Padding(8).Children(func() {
			if sidebarAction(c, "trash", "回收站", "回收站", "").Clicked() {
				a.trashOpen = true
			}
			quietButton(c, "导入与备份").Menu(func(m *ui.Menu) {
				if m.Item("导入 Markdown…").Chosen() {
					a.importMarkdown()
				}
				if m.Item("备份全部笔记…").Chosen() {
					a.backupLibrary()
				}
				if m.Item("从备份导入…").Chosen() {
					a.restoreLibrary()
				}
			})
		})
	})
}

func sidebarAction(c *ui.Context, icon, text, label, shortcut string) ui.Element {
	return quietButton(c, "").Label(label).Height(32).Padding(0, 10).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icons[icon]).FontSize(18).TextColor(ui.Hex("#85847f"))
		ui.Text(c, text).FontSize(14).TextColor(ui.Hex("#62615d")).Grow(1)
		if shortcut != "" {
			ui.Text(c, shortcut).FontSize(11).TextColor(muted)
		}
	})
}

func (a *app) tree(c *ui.Context, parent string, depth int) {
	for _, p := range sortedChildren(a.pages, parent) {
		var children func()
		if len(sortedChildren(a.pages, p.ID)) > 0 {
			children = func() { a.tree(c, p.ID, depth+1) }
		}
		label := pageTitle(p)
		row := ui.TreeItem(c.Key(p.ID), "", a.expandedState(p.ID), children).Label(label).Selected(p.ID == a.selected).Height(30).Gap(3).Drag(pageDrag(p.ID))
		// A compact 8 DIP step keeps nested pages close to their parent.
		row.PaddingStart(4 + float32(depth)*8)
		row.ContextMenu(func(m *ui.Menu) { a.pageActions(m, p.ID) })
		row.Children(func() {
			pageIcon(c, p, 18).Margin(0, 5, 0, 0)
			title := ui.Text(c, label).FontSize(14).FontWeight(400).TextColor(ui.Hex("#62615d")).SingleLine().Grow(1).MinWidth(0)
			if p.ID == a.selected {
				title.FontWeight(600).TextColor(ink)
			}
			if row.Hovered() || p.ID == a.selected {
				if iconButton(c.Key("add:"+p.ID), "plus", "新建子页面："+label).Size(24, 24).Clicked() {
					a.addPage(p.ID)
				}
			}
			// Native menus deliver Chosen on a later frame, after the pointer
			// has left this row. Keep the owner mounted so its callback runs.
			actions := iconButton(c.Key("actions:"+p.ID), "more", "页面操作："+label).Size(24, 24)
			actions.Menu(func(m *ui.Menu) { a.pageActions(m, p.ID) })
			if !row.Hovered() && p.ID != a.selected && !actions.Focused() {
				actions.Opacity(0)
			}
		})
		if row.Clicked() {
			if row.ClickModifiers()&ui.Cmd != 0 {
				a.openTab(p.ID)
			} else {
				a.selectPage(p.ID)
			}
		}
		if id, over := ui.DragOver[pageDrag](row); over && string(id) != p.ID {
			_, y, _ := row.PointerPosition()
			if y < 7 {
				row.Children(func() { ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(2).Background(ui.Hex("#2e87dc")) })
			} else if y > 23 {
				row.Children(func() { ui.Box(c).Absolute().Bottom(0).Left(0).Right(0).Height(2).Background(ui.Hex("#2e87dc")) })
			} else {
				row.Background(line)
			}
		}
		if id, ok := ui.Drop[pageDrag](row); ok && string(id) != p.ID {
			_, y, _ := row.PointerPosition()
			a.mutate(func() error {
				if y < 7 || y > 23 {
					return a.store.Place(string(id), p.ID, y > 23)
				}
				return a.store.Move(string(id), p.ID)
			})
		}
	}
}

func pageTitle(p *Page) string {
	if strings.TrimSpace(p.Title) == "" {
		return "无标题"
	}
	return p.Title
}

func (a *app) header(c *ui.Context, d *draft) {
	ui.Row(c).Height(44).Shrink(0).Padding(0, 18).Gap(4).AlignItems(ui.Center).Children(func() {
		var path []*Page
		for p := d.page; p != nil; p = a.find(p.Parent) {
			path = append(path, p)
		}
		for i := len(path) - 1; i >= 0; i-- {
			p := path[i]
			label := pageTitle(p)
			if runes := []rune(label); len(runes) > 10 {
				label = string(runes[:10]) + "…"
			}
			if i > 1 && i < len(path)-1 {
				continue
			}
			if quietButton(c.Key("crumb:"+p.ID), "").Label(label).Gap(5).AlignItems(ui.Center).Tooltip(pageTitle(p)).Children(func() {
				if coloredPageIcon(p.Body.Icon) != nil {
					pageIcon(c, p, 16)
				}
				ui.Text(c, label).FontSize(13)
			}).Clicked() {
				a.selectPage(p.ID)
			}
			if i > 0 {
				ui.Text(c, "/").TextColor(ui.Hex("#d3d2ce"))
			}
		}
		ui.Box(c).Grow(1)
		insert := iconButton(c, "plus", "插入内容").KeepFocus()
		if insert.Clicked() {
			a.insertOpen = !a.insertOpen
			a.insertQuery = ""
			a.dismissSlash()
		}
		a.insertMenu(c, d, insert)
		more := iconButton(c, "more", "页面菜单")
		if more.Clicked() {
			a.pageMenu = !a.pageMenu
		}
		ui.Popover(c, more, &a.pageMenu, func() {
			menu := func(label string, action func()) {
				if quietButton(c, label).Clicked() {
					a.pageMenu = false
					action()
				}
			}
			menu("＋ 子页面", func() { a.addPage(d.page.ID) })
			menu("在新标签页中打开", func() { a.openTab(d.page.ID) })
			menu("设置页面图标", func() { a.showPageIcons(d.page.ID) })
			menu("复制页面链接", func() {
				if err := a.flush(); err != nil {
					a.report(err)
					return
				}
				c.WriteClipboard(referenceURL(d.page.ID, ""))
			})
			menu("移动到…", func() { a.moveOpen = true })
			menu("在侧栏上移", func() { a.mutate(func() error { return a.store.Reorder(d.page.ID, -1) }) })
			menu("在侧栏下移", func() { a.mutate(func() error { return a.store.Reorder(d.page.ID, 1) }) })
			menu("导出 Markdown…", a.export)
			menu("删除页面", func() { a.deletePage(d.page.ID) })
			ui.Box(c).Height(1).Background(line).Margin(5, 0)
			ui.Text(c, a.status).FontSize(11).TextColor(muted).Padding(4, 8)
			ui.Textf(c, "%d 字符", len([]rune(d.doc.Text().PlainText()))).FontSize(11).TextColor(muted).Padding(4, 8)
		})
	})
}

func (a *app) toolbar(c *ui.Context, d *draft) {
	doc := d.doc
	f := doc.Formatting()
	style := fmt.Sprintf("H%d", f.Paragraph.Heading)
	if f.Paragraph.Heading == 0 {
		style = "正文"
	}
	if ui.Select(c, &style, []string{"正文", "H1", "H2", "H3", "H4", "H5", "H6"}).Label("段落样式").Width(76).Height(28).Border(0, line).Background(paper).Changed() {
		level := 0
		if style != "正文" {
			fmt.Sscanf(style, "H%d", &level)
		}
		doc.SetHeading(level)
	}
	ui.Box(c).Width(1).Height(18).Background(line).Margin(0, 4)
	btn := func(icon, tip string, on bool, run func()) {
		b := iconButton(c, icon, tip).KeepFocus()
		if on {
			b.Background(line)
		}
		if b.Clicked() {
			run()
		}
	}
	btn("bold", "加粗 · ⌘B", f.Bold == richtext.FormatOn, doc.ToggleBold)
	btn("italic", "斜体 · ⌘I", f.Italic == richtext.FormatOn, doc.ToggleItalic)
	btn("strike", "删除线", f.Strikethrough == richtext.FormatOn, doc.ToggleStrikethrough)
	btn("code", "行内代码", f.Code == richtext.FormatOn, doc.ToggleCode)
	btn("link", "链接 · ⌘K", f.Link != "", a.openLink)
}

func (a *app) insertMenu(c *ui.Context, d *draft, insert ui.Element) {
	ui.Popover(c, insert, &a.insertOpen, func() {
		search := ui.TextInput(c, &a.insertQuery).Label("搜索内容块").Placeholder("搜索，如 todo、待办、heading…").Width(280).AutoFocus()
		items := matchingCommands(a.insertQuery)
		if search.Submitted() && len(items) > 0 {
			a.runBlockCommand(d, items[0], false)
		}
		a.commandList(c, items, 0, search.Changed(), func(cmd blockCommand) { a.runBlockCommand(d, cmd, false) })
	})
}

func (a *app) dialogs(c *ui.Context) {
	a.tabPickerDialog(c)
	ui.Modal(c, &a.errorOpen, func() {
		ui.Text(c, "操作未完成").Bold()
		ui.Text(c, a.failure).MaxWidth(540)
		if ui.Button(c, "知道了").Clicked() {
			a.errorOpen = false
		}
	})
	ui.DialogBase(c, &a.searchOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(600).Height(440).Padding(20).Gap(12).Radius(9).Background(paper)
		ui.TextInput(c, &a.query).Label("搜索关键词").Placeholder("搜索页面标题与正文…").AutoFocus()
		ui.Scroll(c).Grow(1).Children(func() {
			count := 0
			query := strings.ToLower(strings.TrimSpace(a.query))
			for _, p := range a.pages {
				if p.Deleted != "" {
					continue
				}
				body := p.Body
				if d := a.drafts[p.ID]; d != nil {
					body.Text = d.doc.Text()
				}
				if query != "" && !strings.Contains(strings.ToLower(p.Title+" "+body.PlainText()), query) {
					continue
				}
				count++
				if quietButton(c.Key("result:"+p.ID), "").Label("搜索结果：" + pageTitle(p)).Children(func() {
					ui.Column(c).Gap(4).Children(func() {
						ui.Text(c, pageTitle(p)).FontSize(14)
						ui.Text(c, a.pagePath(p)).FontSize(11).TextColor(muted).SingleLine()
						ui.Text(c, matchSnippet(body.PlainText(), query)).FontSize(12).TextColor(muted).MaxWidth(520).SingleLine()
					})
				}).Clicked() {
					a.openSearchResult(p.ID, query)
				}
			}
			if count == 0 {
				ui.Text(c, "没有找到匹配的页面").TextColor(muted)
			}
		})
	})
	ui.Modal(c, &a.linkOpen, func() {
		ui.Text(c, "编辑链接").Bold()
		ui.TextInput(c, &a.linkURL).Label("链接地址").Placeholder("https://…").Width(360).AutoFocus()
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "移除链接").Clicked() {
				if d := a.current(); d != nil {
					d.doc.SetLink("")
				}
				a.linkOpen = false
			}
			if ui.PrimaryButton(c, "应用链接").Clicked() {
				if d := a.current(); d != nil {
					d.doc.SetLink(a.linkURL)
				}
				a.linkOpen = false
			}
		})
	})
	ui.DialogBase(c, &a.trashOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(560).Height(420).Padding(24).Gap(12).Radius(9).Background(paper)
		ui.Text(c, "回收站").FontSize(22).Bold()
		ui.Text(c, "恢复页面时，会一起恢复这次删除的子页面。").FontSize(12).TextColor(muted)
		ui.Scroll(c).Grow(1).Children(func() {
			count := 0
			for _, p := range a.pages {
				if p.Deleted != p.ID {
					continue
				}
				count++
				ui.Row(c.Key("trash:"+p.ID)).Gap(8).Padding(6, 0).Children(func() {
					ui.Text(c, p.Title).Grow(1)
					if ui.Button(c, "恢复").Clicked() {
						a.mutate(func() error { return a.store.Restore(p.ID) })
					}
				})
			}
			if count == 0 {
				ui.Text(c, "回收站是空的").TextColor(muted)
			}
		})
	})
	ui.DialogBase(c, &a.moveOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(480).Height(420).Padding(24).Gap(10).Radius(9).Background(paper)
		ui.Text(c, "移动页面到").Bold()
		move := func(id string) { a.mutate(func() error { return a.store.Move(a.selected, id) }); a.moveOpen = false }
		if quietButton(c, "顶层页面").Clicked() {
			move("")
		}
		ui.Scroll(c).Grow(1).Children(func() {
			for _, p := range a.pages {
				if p.Deleted != "" || p.ID == a.selected {
					continue
				}
				descendant := false
				for parent := p.Parent; parent != ""; {
					if parent == a.selected {
						descendant = true
						break
					}
					pp := a.find(parent)
					if pp == nil {
						break
					}
					parent = pp.Parent
				}
				if descendant {
					continue
				}
				if quietButton(c.Key("move:"+p.ID), p.Title).Clicked() {
					move(p.ID)
				}
			}
		})
	})
	ui.DialogBase(c, &a.tableOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(700).Height(470).Padding(24).Gap(12).Radius(9).Background(paper)
		ui.Text(c, "编辑表格").FontSize(22).Bold()
		ui.Text(c, "第一行是表头。插入后可直接在正文中编辑单元格。").FontSize(12).TextColor(muted)
		ui.Scroll(c).Grow(1).Children(func() {
			for i := range a.table.Cells {
				ui.Row(c.Key(fmt.Sprintf("row:%d", i))).Gap(4).Padding(2, 0).Children(func() {
					for j := range a.table.Cells[i] {
						ui.TextInput(c.Key(fmt.Sprintf("cell:%d:%d", i, j)), &a.table.Cells[i][j]).Label(fmt.Sprintf("第 %d 行第 %d 列", i+1, j+1)).Placeholder("空白").Grow(1).MinWidth(0)
					}
				})
			}
		})
		ui.Row(c).Gap(6).Children(func() {
			if ui.Button(c, "＋ 行").Disabled(len(a.table.Cells) >= 30).Clicked() {
				a.table.Cells = append(a.table.Cells, make([]string, len(a.table.Cells[0])))
			}
			if ui.Button(c, "＋ 列").Disabled(len(a.table.Cells[0]) >= 6).Clicked() {
				for i := range a.table.Cells {
					a.table.Cells[i] = append(a.table.Cells[i], "")
				}
			}
			if ui.Button(c, "－ 末行").Disabled(len(a.table.Cells) <= 1).Clicked() {
				a.table.Cells = a.table.Cells[:len(a.table.Cells)-1]
			}
			if ui.Button(c, "－ 末列").Disabled(len(a.table.Cells[0]) <= 1).Clicked() {
				for i := range a.table.Cells {
					a.table.Cells[i] = a.table.Cells[i][:len(a.table.Cells[i])-1]
				}
			}
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.tableOpen = false
			}
			if ui.PrimaryButton(c, "保存表格").Clicked() {
				a.saveTable()
			}
		})
	})
	if !a.tableOpen {
		a.pendingSlash = nil
	}
}
