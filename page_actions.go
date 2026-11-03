package main

import (
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func (a *app) pageActions(m *ui.Menu, id string) {
	if m.Item("在新标签页中打开").Chosen() {
		a.openTab(id)
	}
	if m.Item("新建子页面").Chosen() {
		a.addPage(id)
	}
	if m.Item("重命名").Chosen() {
		a.selectPage(id)
		a.titleHandle.Focus()
	}
	if m.Item("移动到…").Chosen() {
		a.selectPage(id)
		a.moveOpen = true
	}
	m.Separator()
	if m.Item("删除页面").Chosen() {
		a.deletePage(id)
	}
}
func (a *app) deletePage(id string) {
	p := a.find(id)
	if p == nil {
		return
	}
	parent := p.Parent
	affected := false
	for current := a.find(a.selected); current != nil; current = a.find(current.Parent) {
		if current.ID == id {
			affected = true
			break
		}
	}
	a.mutate(func() error { return a.store.Trash(id) })
	if p = a.find(parent); affected && p != nil && p.Deleted == "" {
		a.selectPage(parent)
	}
}
func (a *app) childPages(c *ui.Context, parent string) {
	for _, p := range sortedChildren(a.pages, parent) {
		row := quietButton(c.Key("child:"+p.ID), "").Label("打开子页面：" + pageTitle(p)).Height(32).Gap(8).Children(func() {
			pageIcon(c, p, 18)
			ui.Text(c, pageTitle(p)).FontSize(16)
		})
		a.childPageDrop(c, row, p.ID, 16)
		if row.Clicked() {
			a.selectPage(p.ID)
		}
	}
}

// Child-page entries live above the editor, so they need their own gutter
// controls aligned with the native paragraph handles below them.
func (a *app) childPageGutter(c *ui.Context, d *draft, top, pointerY float32, over bool) {
	for i, p := range sortedChildren(a.pages, d.page.ID) {
		y := top + float32(i)*32
		handle := ui.Box(c.Key("child-grip:"+p.ID)).KeepFocus().Absolute().Left(12).Top(y+3).Size(24, 26).Padding(4).AlignItems(ui.Center).Justify(ui.Center).Radius(4).Role(ui.RoleButton).Focusable().Label("移动子页面：" + pageTitle(p)).Drag(pageDrag(p.ID))
		handle.Children(func() { ui.Icon(c, icons["grip"]).FontSize(16).TextColor(muted) })
		if !handle.Dragging() && !handle.Hovered() && !handle.Focused() && !(a.childMenuOpen && a.childMenu == p.ID) && !(over && pointerY >= y && pointerY < y+32) {
			handle.Opacity(0)
		}
		if handle.Hovered() {
			handle.Background(line)
		}

		handle.ContextMenu(func(m *ui.Menu) { a.pageActions(m, p.ID) })
		if handle.Clicked() {
			same := a.childMenu == p.ID && a.childMenuOpen
			a.childMenu = p.ID
			a.childMenuOpen = !same
			a.blockMenu = false
		}
		if a.childMenu == p.ID {
			ui.Popover(c, handle, &a.childMenuOpen, func() {
				item := func(label string, run func()) {
					if quietButton(c, label).Clicked() {
						a.childMenuOpen = false
						run()
					}
				}
				item("复制页面链接", func() {
					if err := a.flush(); err != nil {
						a.report(err)
					} else {
						c.WriteClipboard(referenceURL(p.ID, ""))
					}
				})
				item("在新标签页中打开", func() { a.openTab(p.ID) })
				item("重命名", func() { a.selectPage(p.ID); a.focusTitlePage = p.ID })
				item("移动到…", func() { a.selectPage(p.ID); a.moveOpen = true })
				item("上移", func() { a.mutate(func() error { return a.store.Reorder(p.ID, -1) }) })
				item("下移", func() { a.mutate(func() error { return a.store.Reorder(p.ID, 1) }) })
				ui.Box(c).Height(1).Background(line).Margin(4, 0)
				item("删除页面", func() { a.deletePage(p.ID) })
			})
		}

		a.childPageDrop(c, handle, p.ID, 13)
	}
}

func (a *app) childPageDrop(c *ui.Context, target ui.Element, id string, middle float32) {
	if drag, over := ui.DragOver[pageDrag](target); over && string(drag) != id {
		_, y, _ := target.PointerPosition()
		target.Children(func() {
			rule := ui.Box(c).Absolute().Left(0).Right(0).Height(2).Background(ui.Hex("#2e87dc"))
			if y < middle {
				rule.Top(0)
			} else {
				rule.Bottom(0)
			}
		})
	}
	if drag, ok := ui.Drop[pageDrag](target); ok && string(drag) != id {
		_, y, _ := target.PointerPosition()
		a.mutate(func() error { return a.store.Place(string(drag), id, y >= middle) })
	}
}

func (a *app) slashPage(d *draft) {
	if start, end, ok := a.pendingCommandRange(d); ok {
		d.doc.Edit(func(e *richtext.Edit) { e.ReplaceString(start, end, "", richtext.Attributes{}); e.Select(start, start) })
		a.changed(d)
	}
	a.pendingSlash = nil
	a.addPage(d.page.ID)
}
