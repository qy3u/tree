package main

import (
	"testing"
	"time"
)

func TestLibraryNavigationRecentsAndSession(t *testing.T) {
	a, tt := commandTestApp(t)
	ids := tabTestPages(t, a)
	a.selectPage(ids[0])
	a.current().doc.InsertText("离开前尚未保存的内容")
	a.selectPage(ids[1])
	tt.Frame()
	count := len(a.pages)
	if err := tt.Click("打开 Library"); err != nil {
		t.Fatal(err)
	}
	if a.selected != libraryRoute || a.current() != nil || len(a.pages) != count {
		t.Fatal("Library should be a destination, not a stored note")
	}
	pages := a.libraryPages()
	if len(pages) < 2 || pages[0].ID != ids[1] || pages[1].ID != ids[0] {
		t.Fatal("recent pages not sorted by visit")
	}
	for _, p := range pages {
		if p.ID == ids[2] {
			t.Fatal("unvisited page appeared in recents")
		}
	}
	for _, label := range []string{"页面名称", "来源", "最后编辑时间", "Library 最近访问", "Library 收藏"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("missing Library control %s", label)
		}
	}
	for _, label := range []string{"Created by", "创建者", "Shared", "Private", "AI Meeting Notes", "Skills"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("unexpected Library column or tab %s", label)
		}
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.selected != libraryRoute || reopened.tabs[reopened.activeTab].page() != libraryRoute {
		t.Fatal("Library session did not survive reopening")
	}
	if _, ok := a.visited[libraryRoute]; ok {
		t.Fatal("virtual route was recorded as a page visit")
	}
	tt.Click("Library 打开：甲")
	if a.selected != ids[0] || a.current().doc.Text().String() != "离开前尚未保存的内容" {
		t.Fatal("opening from Library lost the draft")
	}
	tt.Click("后退")
	if a.selected != libraryRoute {
		t.Fatal("back did not restore Library")
	}
	tt.Click("前进")
	if a.selected != ids[0] {
		t.Fatal("forward did not restore the page")
	}
	a.openTab(libraryRoute)
	a.switchTab(0)
	a.switchTab(1)
	if a.selected != libraryRoute {
		t.Fatal("Library tab did not survive switching")
	}
	tt.Frame()
	tt.Click("Library 新建页面")
	if len(a.pages) != count+1 || a.current() == nil || a.current().page.Parent != "" || !tt.Focused("页面标题") {
		t.Fatal("Library create should open a new top-level page and focus its title")
	}
}

func TestLibraryFavoritesPersistAndTrashRestore(t *testing.T) {
	a, tt := commandTestApp(t)
	parent := createPage(t, a.store, "", "资料")
	child := createPage(t, a.store, parent.ID, "阅读")
	if err := a.reload(); err != nil {
		t.Fatal(err)
	}
	a.selectPage(child.ID)
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	updated, revision := a.find(child.ID).Updated, a.find(child.ID).Revision
	tt.Frame()
	if err := tt.Click("添加到收藏"); err != nil {
		t.Fatal(err)
	}
	if !a.isFavorite(child.ID) || a.find(child.ID).Updated != updated || a.find(child.ID).Revision != revision {
		t.Fatal("favoriting should persist independently of page edits")
	}
	tt.Click("打开 Library")
	tt.Click("Library 收藏")
	if pages := a.libraryPages(); len(pages) != 1 || pages[0].ID != child.ID {
		t.Fatal("Favorites did not list the selected page")
	}
	if err := tt.Click("Library 来源：阅读"); err != nil {
		t.Fatal(err)
	}
	if a.selected != parent.ID {
		t.Fatal("source did not navigate to parent")
	}
	tt.Click("后退")
	reopened, err := newApp(a.store)
	if err != nil || !reopened.isFavorite(child.ID) {
		t.Fatalf("favorite was not persisted: %v", err)
	}
	a.deletePage(parent.ID)
	if a.selected != libraryRoute || len(a.libraryPages()) != 0 {
		t.Fatal("deleting a parent should hide its descendants without leaving Library")
	}
	a.mutate(func() error { return a.store.Restore(parent.ID) })
	if pages := a.libraryPages(); len(pages) != 1 || pages[0].ID != child.ID {
		t.Fatal("restored favorite was lost")
	}
	tt.Frame()
	row, ok := tt.Find("Library 打开：阅读")
	if !ok {
		t.Fatal("restored row missing")
	}
	tt.Move(row.X+10, row.Y+10)
	if err := tt.Click("Library 取消收藏：阅读"); err != nil {
		t.Fatal(err)
	}
	if a.isFavorite(child.ID) || len(a.libraryPages()) != 0 || a.selected != libraryRoute {
		t.Fatal("removing favorite should remove the row without opening the page")
	}
	if _, ok := tt.Find("还没有收藏的页面"); !ok {
		t.Fatal("missing empty Favorites state")
	}
	reopened, err = newApp(a.store)
	if err != nil || reopened.isFavorite(child.ID) {
		t.Fatalf("removing favorite did not persist: %v", err)
	}
	if a.failure != "" {
		t.Fatal(a.failure)
	}
}

func TestLibrarySmallWindowAndRelativeDates(t *testing.T) {
	a, tt := commandTestApp(t)
	ids := tabTestPages(t, a)
	a.selectPage(ids[0])
	a.selectPage(libraryRoute)
	tt.SetSize(850, 580)
	tt.Frame()
	for _, label := range []string{"页面名称", "来源", "最后编辑时间", "Library 新建页面", "Library 打开：甲"} {
		r, ok := tt.Find(label)
		if !ok || r.X < sidebarWidth || r.X+r.W > 850 || r.W <= 0 {
			t.Fatalf("small window clipped %s: %+v", label, r)
		}
	}
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		at   time.Time
		want string
	}{{now.Add(-20 * time.Second), "刚刚"}, {now.Add(-2 * time.Minute), "2 分钟前"}, {now.Add(-3 * time.Hour), "3 小时前"}, {now.AddDate(0, 0, -2), "2 天前"}, {now.AddDate(0, 0, -10), "10月24日"}, {now.AddDate(-1, 0, 0), "2025年11月3日"}} {
		if got := lastEditedLabel(tc.at.Format(time.RFC3339Nano), now); got != tc.want {
			t.Fatalf("date label %q, want %q", got, tc.want)
		}
	}
}
