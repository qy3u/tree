package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestTabPickerCancelSearchPreviewAndConfirm(t *testing.T) {
	a, tt := commandTestApp(t)
	ids := tabTestPages(t, a)
	a.selectPage(ids[1])
	a.current().doc.InsertText("可搜索的独特正文")
	a.selectPage(ids[0])
	tt.Frame()
	original := a.current().doc
	original.SetSelection(0, 0)
	initialTabs, initialPages := len(a.tabs), len(a.pages)
	tt.Click("新建标签页")
	if !tt.Focused("新标签页搜索") {
		t.Fatal("picker did not focus search")
	}
	tt.Key(0, ui.KeyEscape)
	if a.tabPicker.Open || len(a.tabs) != initialTabs || len(a.pages) != initialPages || a.current().doc != original {
		t.Fatal("cancel changed current page or tabs")
	}
	tt.Click("新建标签页")
	tt.Type("独特正文")
	if a.tabPicker.Selected != 1 || len(a.tabPickerPages()) != 1 || a.tabPicker.previewID != ids[1] {
		t.Fatal("body search or selected preview is wrong")
	}
	if a.tabPicker.preview == a.drafts[ids[1]].doc || a.selected != ids[0] {
		t.Fatal("preview navigated or reused editing document")
	}
	if _, ok := tt.Find("标签页打开：甲"); ok {
		t.Fatal("search did not filter results")
	}
	tt.Key(0, ui.KeyEnter)
	if a.tabPicker.Open || a.selected != ids[1] || len(a.tabs) != initialTabs+1 {
		t.Fatal("Enter did not open exactly one new tab")
	}
	a.showTabPicker()
	tt.Frame()
	tt.ClickAt(5, 500)
	if a.tabPicker.Open {
		t.Fatal("outside click did not dismiss picker")
	}
}

func TestTabPickerCreateNoResultsAndKeyboard(t *testing.T) {
	a, tt := commandTestApp(t)
	ids := tabTestPages(t, a)
	a.selectPage(ids[0])
	tt.Frame()
	before := len(a.pages)
	tt.Key(ui.Cmd, ui.KeyT)
	tt.Type("不存在的页面xyz")
	if len(a.tabPickerPages()) != 0 || a.tabPicker.Selected != 0 {
		t.Fatal("empty results did not keep create action")
	}
	tt.Key(0, ui.KeyEnter)
	if len(a.pages) != before+1 || len(a.tabs) != 2 || a.current().page.Title != "" || a.tabPicker.Open {
		t.Fatalf("create result: pages=%d want=%d tabs=%d title=%q open=%v error=%q", len(a.pages), before+1, len(a.tabs), a.current().page.Title, a.tabPicker.Open, a.failure)
	}
	if !tt.Focused("页面标题") {
		t.Fatal("new page title not focused")
	}
	tt.Key(ui.Cmd, ui.KeyT)
	row, ok := tt.Find("标签页创建新页面")
	if !ok {
		t.Fatal("missing create action")
	}
	tt.Move(row.X+10, row.Y+10)
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.tabPicker.Selected != 1 {
		t.Fatal("stationary pointer overrode keyboard selection")
	}
	expected := a.tabPickerPages()[0].ID
	tt.Key(0, ui.KeyEnter)
	if a.selected != expected || len(a.tabs) != 3 {
		t.Fatal("keyboard selected wrong page")
	}
}

func TestTabPickerRecentGroupsPersistenceAndDeletedPages(t *testing.T) {
	a, _ := commandTestApp(t)
	ids := tabTestPages(t, a)
	now := time.Now()
	a.visited = map[string]time.Time{ids[0]: now, ids[1]: now.AddDate(0, 0, -1), ids[2]: now.AddDate(0, 0, -40)}
	a.showTabPicker()
	pages := a.tabPickerPages()
	if pages[0].ID != ids[0] || pages[1].ID != ids[1] || pages[2].ID != ids[2] {
		t.Fatal("recents not ordered by visits")
	}
	for _, tc := range []struct {
		at   time.Time
		want string
	}{{now, "今天"}, {now.AddDate(0, 0, -1), "昨天"}, {now.AddDate(0, 0, -12), "过去 30 天"}, {now.AddDate(0, 0, -40), "更早"}, {time.Time{}, "其他页面"}} {
		if got := visitGroup(tc.at, now); got != tc.want {
			t.Fatalf("visit group %s, want %s", got, tc.want)
		}
	}
	a.recordVisit(ids[1])
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.visited[ids[1]].Equal(a.visited[ids[1]]) {
		t.Fatal("visit time lost on restart")
	}
	a.deletePage(ids[1])
	for _, p := range a.tabPickerPages() {
		if p.ID == ids[1] {
			t.Fatal("deleted page in picker")
		}
	}
}
