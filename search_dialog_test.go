package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSearchDialogRecentGroupsAndKeyboard(t *testing.T) {
	a, tt := commandTestApp(t)
	ids := tabTestPages(t, a)
	now := time.Now()
	a.visited = map[string]time.Time{ids[0]: now, ids[1]: now.AddDate(0, 0, -1), ids[2]: now.AddDate(0, 0, -40)}
	original, tabs := a.selected, len(a.tabs)
	tt.Key(ui.Cmd, ui.KeyP)
	if !tt.Focused("搜索关键词") {
		t.Fatal("search input did not receive focus")
	}
	for _, label := range []string{"今天", "昨天", "更早", "其他页面"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("missing recent group %s", label)
		}
	}
	row, ok := tt.Find("搜索结果：甲")
	if !ok {
		t.Fatal("missing recent page")
	}
	tt.Move(row.X+12, row.Y+12)
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.searchSelection != 1 {
		t.Fatal("stationary pointer overrode keyboard selection")
	}
	tt.Key(0, ui.KeyEnter)
	if a.searchOpen || a.selected != ids[1] || len(a.tabs) != tabs {
		t.Fatal("Enter did not open the selected page in the current tab")
	}
	a.selectPage(original)
	tt.Frame()
	tt.Click("搜索页面")
	tt.Type("no-such-page-xyz")
	if _, ok := tt.Find("没有找到匹配的页面"); !ok {
		t.Fatal("missing empty result state")
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if !a.searchOpen || a.selected != original {
		t.Fatal("empty search changed the current page")
	}
	tt.Key(0, ui.KeyEscape)
	if a.searchOpen {
		t.Fatal("Escape did not close search")
	}
	tt.Key(ui.Cmd, ui.KeyP)
	if a.query != "" || a.searchSelection != 0 {
		t.Fatal("reopening should reset to recent pages")
	}
	tt.ClickAt(5, 500)
	if a.searchOpen || a.selected != original {
		t.Fatal("outside click did not dismiss without navigation")
	}
}

func TestSearchDialogDraftMatchPathAndClick(t *testing.T) {
	a, tt := commandTestApp(t)
	original := a.selected
	parent, err := a.store.Create("", "学习资料", emptyBody())
	if err != nil {
		t.Fatal(err)
	}
	body := emptyBody()
	body.Icon = "books"
	child, err := a.store.Create(parent.ID, "读书笔记", body)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.reload(); err != nil {
		t.Fatal(err)
	}
	a.selectPage(child.ID)
	a.current().doc.InsertText("这里有尚未保存的中文正文命中词")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyP)
	tt.Type("正文命中词")
	pages := a.matchingPages(a.query)
	if len(pages) != 1 || pages[0].ID != child.ID {
		t.Fatal("search failed to match live draft")
	}
	for _, label := range []string{"— 学习资料", "页面图标：books", "这里有尚未保存的中文正文命中词"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("missing result detail %s", label)
		}
	}
	if err := tt.Click("搜索结果：读书笔记"); err != nil {
		t.Fatal(err)
	}
	start, end := a.current().doc.Selection()
	if a.searchOpen || a.current().doc.Text().Slice(start, end).String() != "正文命中词" {
		t.Fatal("click did not select the body match")
	}
	a.selectPage(original)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyP)
	tt.Type("读书笔记")
	tt.Key(0, ui.KeyEnter)
	if a.selected != child.ID || a.searchOpen {
		t.Fatal("title search did not navigate")
	}
	a.deletePage(child.ID)
	if pages := a.matchingPages("正文命中词"); len(pages) != 0 {
		t.Fatal("deleted draft appeared in results")
	}
	if a.failure != "" {
		t.Fatal(a.failure)
	}
}
