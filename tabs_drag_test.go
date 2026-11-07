package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func tabIDs(a *app) []string {
	ids := make([]string, len(a.tabs))
	for i, tab := range a.tabs {
		ids[i] = tab.ID
	}
	return ids
}

func TestReorderTabsPreservesIdentityHistoryAndSession(t *testing.T) {
	a, _ := commandTestApp(t)
	pages := tabTestPages(t, a)
	a.selectPage(pages[0])
	a.selectPage(pages[1])
	a.openTab(pages[0])
	a.openTab(pages[1]) // Same page as the first tab, independent history.
	original := append([]pageTab(nil), a.tabs...)
	activeID, selected := a.tabs[a.activeTab].ID, a.selected
	draft := a.current()
	draft.doc.InsertText("正在编辑的内容")
	draft.doc.SetSelection(2, 5)
	a.reorderTab(original[0].ID, 3)
	if !reflect.DeepEqual(a.tabs, []pageTab{original[1], original[2], original[0]}) {
		t.Fatal("wrong rightward order/history")
	}
	if a.tabs[a.activeTab].ID != activeID || a.selected != selected {
		t.Fatal("reorder changed the active page")
	}
	anchor, caret := draft.doc.Selection()
	if a.current() != draft || draft.doc.Text().String() != "正在编辑的内容" || anchor != 2 || caret != 5 {
		t.Fatal("reordering changed the draft or selection")
	}
	a.reorderTab(original[0].ID, 0)
	if !reflect.DeepEqual(a.tabs, original) {
		t.Fatal("wrong leftward order/history")
	}
	a.reorderTab(activeID, 0)
	if a.activeTab != 0 || a.selected != selected {
		t.Fatal("moving active tab changed page")
	}
	saved := append([]pageTab(nil), a.tabs...)
	for _, slot := range []int{-1, 0, 1, 4} {
		a.reorderTab(activeID, slot)
	}
	a.reorderTab("missing", 2)
	if !reflect.DeepEqual(a.tabs, saved) {
		t.Fatal("invalid/self drop changed order")
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened.tabs, saved) || reopened.activeTab != a.activeTab {
		t.Fatal("reordered session not restored")
	}
	var persisted tabSession
	if err := json.Unmarshal([]byte(a.store.Setting("tabs")), &persisted); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted.Tabs, saved) {
		t.Fatal("incorrect persisted order")
	}
}

func dragTabRect(t *testing.T, tt *ui.Tester, index int, name string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(fmt.Sprintf("切换标签页 %d：%s", index+1, name))
	if !ok {
		t.Fatal("tab not found", index, name)
	}
	return r
}

func TestNativeTabDragReorderAndCancel(t *testing.T) {
	a, tt := commandTestApp(t)
	pages := tabTestPages(t, a)
	a.selectPage(pages[0])
	a.openTab(pages[1])
	a.openTab(pages[2])
	tt.Frame()
	original := tabIDs(a)
	selected := a.selected
	src := dragTabRect(t, tt, 0, "甲")
	dst := dragTabRect(t, tt, 2, "丙")
	tt.Press(src.X+35, src.Y+18)
	tt.Move(dst.X+120, dst.Y+18)
	if _, ok := tt.Find("标签页插入位置"); !ok {
		t.Fatal("missing insertion indicator")
	}
	tt.Release(dst.X+120, dst.Y+18)
	tt.Frame()
	if !reflect.DeepEqual(tabIDs(a), []string{original[1], original[2], original[0]}) {
		t.Fatal("native rightward drop did not reorder", tabIDs(a))
	}
	if a.selected != selected || a.activeTab != 1 {
		t.Fatal("drag changed active page")
	}
	src = dragTabRect(t, tt, 2, "甲")
	dst = dragTabRect(t, tt, 0, "乙")
	tt.Press(src.X+35, src.Y+18)
	tt.Move(dst.X+10, dst.Y+18)
	tt.Release(dst.X+10, dst.Y+18)
	tt.Frame()
	if !reflect.DeepEqual(tabIDs(a), original) {
		t.Fatal("native leftward drop did not reorder")
	}
	src = dragTabRect(t, tt, 0, "甲")
	dst = dragTabRect(t, tt, 2, "丙")
	tt.Press(src.X+35, src.Y+18)
	tt.Move(dst.X+120, dst.Y+18)
	tt.Key(0, ui.KeyEscape)
	tt.Release(dst.X+120, dst.Y+18)
	if !reflect.DeepEqual(tabIDs(a), original) {
		t.Fatal("escape did not cancel")
	}
	tt.Press(src.X+35, src.Y+18)
	tt.Move(src.X+100, src.Y+180)
	tt.Release(src.X+100, src.Y+180)
	if !reflect.DeepEqual(tabIDs(a), original) {
		t.Fatal("outside drop changed order")
	}
	if err := tt.Click("切换标签页 1：甲"); err != nil {
		t.Fatal(err)
	}
	if a.selected != pages[0] {
		t.Fatal("click no longer switches tabs")
	}
	if err := tt.Click("关闭标签页 2"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 2 || a.selected != pages[0] {
		t.Fatal("close broken after drag")
	}
}

func TestTabDragScrollsOverflow(t *testing.T) {
	a, tt := commandTestApp(t)
	pages := tabTestPages(t, a)
	a.selectPage(pages[0])
	for range 7 {
		a.openTab(pages[0])
	}
	a.switchTab(0)
	tt.Frame()
	original := tabIDs(a)
	src := dragTabRect(t, tt, 0, "甲")
	plus, _ := tt.Find("新建标签页")
	tt.Press(src.X+35, src.Y+18)
	tt.Move(plus.X-8, src.Y+18)
	for range 90 {
		tt.Advance(time.Second / 60)
	}
	if a.tabScroll.X < 400 {
		t.Fatalf("did not autoscroll: %v", a.tabScroll.X)
	}
	tt.Move(plus.X+10, plus.Y+18)
	tt.Release(plus.X+10, plus.Y+18)
	tt.Frame()
	if a.tabs[len(a.tabs)-1].ID != original[0] || a.activeTab != len(a.tabs)-1 {
		t.Fatal("drop at end of overflowing strip failed")
	}
	if a.tabPicker.Open {
		t.Fatal("dropping on plus opened picker")
	}
	// Return from the scrolled end, then release without moving the pointer
	// again. The insertion target must follow the newly visible tabs.
	src = dragTabRect(t, tt, len(a.tabs)-1, "甲")
	left := plus.X - (1120 - sidebarWidth - 1 - 69 - 42) + 8
	tt.Press(src.X+35, src.Y+18)
	tt.Move(left, src.Y+18)
	for range 120 {
		tt.Advance(time.Second / 60)
	}
	if a.tabScroll.X != 0 {
		t.Fatalf("did not scroll back to start: %v", a.tabScroll.X)
	}
	if _, ok := tt.Find("标签页插入位置"); !ok {
		t.Fatal("stationary drag lost indicator")
	}
	tt.Release(left, src.Y+18)
	tt.Frame()
	if !reflect.DeepEqual(tabIDs(a), original) || a.activeTab != 0 {
		t.Fatal("stationary drop after scrolling used stale boundary")
	}
}
