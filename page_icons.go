package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

//go:embed assets/page-icons/*.svg assets/page-icons/catalog.json assets/page-icons/LICENSE assets/page-icons/SOURCE.json
var pageIconFiles embed.FS

type pageIconAsset struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Label string  `json:"label"`
	Group string  `json:"group"`
	SVG   *ui.SVG `json:"-"`
}

var pageIconCatalog = sync.OnceValue(func() []pageIconAsset {
	data, err := pageIconFiles.ReadFile("assets/page-icons/catalog.json")
	if err != nil {
		panic(err)
	}
	var items []pageIconAsset
	if err = json.Unmarshal(data, &items); err != nil {
		panic(err)
	}
	for i := range items {
		data, err = pageIconFiles.ReadFile("assets/page-icons/" + items[i].ID + ".svg")
		if err != nil {
			panic(err)
		}
		items[i].SVG, err = ui.ParseSVG(data)
		if err != nil {
			panic(fmt.Errorf("page icon %s: %w", items[i].ID, err))
		}
	}
	return items
})

func coloredPageIcon(id string) *ui.SVG {
	if id == "" {
		return nil
	}
	for _, icon := range pageIconCatalog() {
		if icon.ID == id {
			return icon.SVG
		}
	}
	return nil
}

func pageIcon(c *ui.Context, p *Page, size float32) ui.Element {
	if p != nil {
		if svg := coloredPageIcon(p.Body.Icon); svg != nil {
			return ui.Image(c, svg).Label("页面图标："+p.Body.Icon).Size(size, size).Shrink(0)
		}
	}
	return ui.Icon(c, icons["page"]).FontSize(size).TextColor(muted).Shrink(0)
}

func filteredPageIcons(query string) []pageIconAsset {
	words := strings.Fields(strings.ToLower(query))
	var result []pageIconAsset
	for _, icon := range pageIconCatalog() {
		text := strings.ToLower(icon.ID + " " + icon.Name + " " + icon.Label + " " + icon.Group)
		match := true
		for _, word := range words {
			if !strings.Contains(text, word) {
				match = false
				break
			}
		}
		if match {
			result = append(result, icon)
		}
	}
	return result
}

type pageIconPicker struct {
	Open        bool
	Page, Query string
}

func (a *app) showPageIcons(id string) {
	a.iconPicker = pageIconPicker{Open: true, Page: id}
}

func (a *app) setPageIcon(id, icon string) {
	if a.applyPageIcon(id, icon) {
		a.iconPicker.Open = false
	}
}

// Applying a random icon leaves the picker and its search state available.
func (a *app) applyPageIcon(id, icon string) bool {
	if icon != "" && coloredPageIcon(icon) == nil {
		return false
	}
	d := a.current()
	if d == nil || d.page.ID != id {
		return false
	}
	d.page.Body.Icon = icon
	d.dirty = true
	if err := a.flush(); err != nil {
		a.report(err)
		return false
	}
	return true
}

func pageIconSpace(p *Page) float32 {
	if coloredPageIcon(p.Body.Icon) != nil {
		return 84
	}
	return 0
}

func (a *app) pageIconControl(c *ui.Context, d *draft) {
	hasIcon := coloredPageIcon(d.page.Body.Icon) != nil
	showAdd := false
	if !hasIcon {
		area := ui.Box(c.Key("page-icon-hover:" + d.page.ID)).Absolute().Top(-34).Left(0).Right(0).Height(34)
		_, _, showAdd = area.PointerPosition()
	}
	// Keep one stable owner across add/replace/remove so popover input survives.
	button := quietButton(c.Key("page-icon:"+d.page.ID), "").KeepFocus().Label("设置页面图标").AlignItems(ui.Center).Justify(ui.Center)
	if hasIcon {
		button.Size(64, 64).Padding(0).Children(func() { pageIcon(c, d.page, 64) })
	} else {
		button.Absolute().Top(-30).Left(0).Height(24).Padding(0, 4).Gap(5).Children(func() {
			ui.Icon(c, icons["page"]).FontSize(13).TextColor(muted)
			ui.Text(c, "添加图标").FontSize(12).TextColor(muted)
		})
		if !showAdd && !button.Hovered() && !button.Focused() && !(a.iconPicker.Open && a.iconPicker.Page == d.page.ID) {
			button.Opacity(0)
		}
	}
	if button.Clicked() {
		a.showPageIcons(d.page.ID)
	}
	if a.iconPicker.Page != d.page.ID {
		return
	}
	ui.Popover(c, button, &a.iconPicker.Open, func() {
		ui.Column(c).Width(352).Gap(10).Padding(8).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "图标").FontSize(14).FontWeight(600)
				ui.Box(c).Grow(1)
				if quietButton(c, "移除").Label("移除页面图标").Disabled(!hasIcon).FontSize(12).Clicked() {
					a.setPageIcon(d.page.ID, "")
				}
			})
			ui.Box(c).Height(1).Background(line)
			ui.Row(c).Gap(6).Children(func() {
				input := ui.TextInputBase(c.Key("icon-search"), &a.iconPicker.Query).Label("搜索页面图标").Placeholder("搜索图标…").Height(32).Padding(6, 8).FontSize(13).Background(sidebarColor).Border(1, line).Radius(5).Grow(1).MinWidth(0).AutoFocus()
				matches := filteredPageIcons(a.iconPicker.Query)
				if input.Submitted() && len(matches) > 0 {
					a.setPageIcon(d.page.ID, matches[0].ID)
				}
				if iconButton(c, "shuffle", "随机页面图标").Size(32, 32).Clicked() {
					items := pageIconCatalog()
					a.applyPageIcon(d.page.ID, items[rand.IntN(len(items))].ID)
				}
			})
			items := filteredPageIcons(a.iconPicker.Query)
			ui.Scroll(c).Height(300).Children(func() {
				if len(items) == 0 {
					ui.Text(c, "没有匹配的图标").FontSize(13).TextColor(muted).Padding(12)
				}
				for start := 0; start < len(items); {
					end := start + 1
					for end < len(items) && items[end].Group == items[start].Group {
						end++
					}
					ui.Text(c, items[start].Group).FontSize(11).TextColor(muted).Padding(8, 2)
					ui.Grid(c).Columns(8).Gap(4).Children(func() {
						for _, icon := range items[start:end] {
							cell := quietButton(c.Key(icon.ID), "").Label("选择图标："+icon.ID).Tooltip(icon.Label).Size(36, 36).Padding(6).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Image(c, icon.SVG).Size(24, 24) })
							if icon.ID == d.page.Body.Icon {
								cell.Background(line)
							}
							if cell.Clicked() {
								a.setPageIcon(d.page.ID, icon.ID)
							}
						}
					})
					start = end
				}
			})
		})
	})
}

// Page references follow the target's live icon without rewriting note content.
func (a *app) drawPageReferenceIcon(p *ui.Painter, r ui.Rect, attrs richtext.Attributes) bool {
	if attrs.Tag != referenceIconTag {
		return false
	}
	page, block, ok := parseReference(attrs.Link)
	if !ok || block != "" {
		return false
	}
	target := a.find(page)
	if target == nil {
		return false
	}
	svg := coloredPageIcon(target.Body.Icon)
	if svg == nil {
		return false
	}
	p.Image(svg, ui.Rect{X: r.X, Y: r.Y + 1, W: 16, H: 16}, ui.Contain)
	return true
}
