package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPageIconPickerSearchChooseReplaceRemove(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	tt.Type("正文不会改变")
	before := d.doc.Text().String()
	titleBefore, _ := tt.Find("页面标题")
	if err := tt.Click("设置页面图标"); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused("搜索页面图标") {
		t.Fatal("search not focused")
	}
	tt.Type("书籍")
	if _, ok := tt.Find("选择图标：books"); !ok {
		t.Fatal("Chinese search missing")
	}
	if _, ok := tt.Find("选择图标：tractor"); ok {
		t.Fatal("search not filtered")
	}
	if err := tt.Click("选择图标：books"); err != nil {
		t.Fatal(err)
	}
	if a.iconPicker.Open || d.page.Body.Icon != "books" {
		t.Fatal("selection not applied")
	}
	titleAfter, _ := tt.Find("页面标题")
	if titleAfter.Y-titleBefore.Y != 84 {
		t.Fatalf("icon/title layout delta %v", titleAfter.Y-titleBefore.Y)
	}
	icon, _ := tt.Find("设置页面图标")
	if icon.Y+icon.H > titleAfter.Y {
		t.Fatal("icon overlaps title")
	}
	if d.doc.Text().String() != before {
		t.Fatal("icon changed content")
	}
	pages, err := a.store.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range pages {
		if p.ID == d.page.ID {
			found = p.Body.Icon == "books"
		}
	}
	if !found {
		t.Fatal("icon was not saved")
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.current().page.Body.Icon != "books" {
		t.Fatal("icon lost on restart")
	}
	if err := tt.Click("设置页面图标"); err != nil {
		t.Fatal(err)
	}
	tt.Type("tractor")
	tt.Key(0, ui.KeyEnter)
	if d.page.Body.Icon != "tractor" {
		t.Fatal("English/Enter selection failed")
	}
	tt.Click("设置页面图标")
	tt.Type("no-such-icon-xyz")
	if _, ok := tt.Find("没有匹配的图标"); !ok {
		t.Fatal("missing empty state")
	}
	tt.Key(0, ui.KeyEscape)
	if a.iconPicker.Open || d.page.Body.Icon != "tractor" {
		t.Fatal("cancel changed icon")
	}
	tt.Click("设置页面图标")
	tt.Click("随机页面图标")
	if coloredPageIcon(d.page.Body.Icon) == nil || !a.iconPicker.Open {
		t.Fatal("random icon failed")
	}
	tt.Click("移除页面图标")
	if d.page.Body.Icon != "" || a.iconPicker.Open {
		t.Fatal("remove failed")
	}
	titleRemoved, _ := tt.Find("页面标题")
	if titleRemoved.Y != titleBefore.Y {
		t.Fatal("icon removal left blank space")
	}
	if d.doc.Text().String() != before {
		t.Fatal("replacement/removal changed body")
	}
}

func TestPageIconMenuBackupAndRestore(t *testing.T) {
	a, tt := commandTestApp(t)
	target := a.selected
	tt.Click("页面菜单")
	tt.Click("设置页面图标")
	if !a.iconPicker.Open {
		t.Fatal("header menu did not open picker")
	}
	tt.Type("rocket")
	tt.Key(0, ui.KeyEnter)
	if a.current().page.Body.Icon != "rocket" {
		t.Fatal("menu icon selection failed")
	}
	path, err := a.store.Backup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dest := testStore(t)
	root, err := dest.RestoreBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := dest.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range sortedChildren(pages, root) {
		if p.Body.Icon == "rocket" {
			found = true
		}
	}
	if !found {
		t.Fatal("backup lost icon")
	}
	a.deletePage(target)
	if err := a.store.Restore(target); err != nil {
		t.Fatal(err)
	}
	a.reload()
	a.selectPage(target)
	if a.current().page.Body.Icon != "rocket" {
		t.Fatal("trash restore lost icon")
	}
	if coloredPageIcon("../../missing") != nil {
		t.Fatal("invalid ID did not fall back")
	}
}

func TestPageIconCatalogAndReferenceDrawing(t *testing.T) {
	items := pageIconCatalog()
	if len(items) != 140 {
		t.Fatalf("catalog count %d", len(items))
	}
	seen := map[string]bool{}
	for _, icon := range items {
		if icon.SVG == nil || seen[icon.ID] {
			t.Fatal("bad asset", icon.ID)
		}
		seen[icon.ID] = true
	}
	a, tt := commandTestApp(t)
	target := a.current()
	target.page.Title = "图标目标"
	a.setPageIcon(target.page.ID, "books")
	a.addPage("")
	tt.Frame()
	tt.Click("笔记正文")
	tt.SetClipboard(referenceURL(target.page.ID, ""))
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Frame()
	doc := a.current().doc
	ed, _ := tt.Find("笔记正文")
	boxes := doc.VisibleAttachments()
	if len(boxes) != 1 {
		t.Fatal("reference icon missing")
	}
	r := boxes[0].Rect
	img := tt.Image()
	colored := 0
	for y := int(ed.Y + r.Y); y < int(ed.Y+r.Y+r.H); y++ {
		for x := int(ed.X + r.X); x < int(ed.X+r.X+r.W); x++ {
			red, green, blue, _ := img.At(x, y).RGBA()
			if max(red, green, blue)-min(red, green, blue) > 6000 {
				colored++
			}
		}
	}
	if colored < 8 {
		t.Fatal("page reference did not use colored target icon")
	}
}

func TestPageIconRandomKeepsPickerOpen(t *testing.T) {
	for _, initial := range []string{"", "books"} {
		t.Run("initial="+initial, func(t *testing.T) {
			a, tt := commandTestApp(t)
			d := a.current()
			tt.Type("正文保持不变")
			before := d.doc.Text()
			a.setPageIcon(d.page.ID, initial)
			tt.Frame()
			if err := tt.Click("设置页面图标"); err != nil {
				t.Fatal(err)
			}
			tt.Type("book")
			for i := 0; i < 8; i++ {
				if err := tt.Click("随机页面图标"); err != nil {
					t.Fatalf("click %d: %v", i+1, err)
				}
				if !a.iconPicker.Open {
					t.Fatalf("picker closed after random click %d", i+1)
				}
				if a.iconPicker.Query != "book" {
					t.Fatal("random selection cleared the search")
				}
				if coloredPageIcon(d.page.Body.Icon) == nil {
					t.Fatal("invalid random icon")
				}
				if !d.doc.Text().Equal(before) {
					t.Fatal("random selection changed the document")
				}
			}
			lastIcon := d.page.Body.Icon
			tt.Key(0, ui.KeyEscape)
			if a.iconPicker.Open || d.page.Body.Icon != lastIcon {
				t.Fatal("Escape failed to close and keep the chosen icon")
			}
			reopened, err := newApp(a.store)
			if err != nil {
				t.Fatal(err)
			}
			reopened.selectPage(d.page.ID)
			if reopened.current().page.Body.Icon != lastIcon {
				t.Fatal("last random icon was not saved")
			}
		})
	}
}
