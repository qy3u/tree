package main

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func tabTestPages(t *testing.T, a *app) []string {
	t.Helper()
	var ids []string
	for _, title := range []string{"甲", "乙", "丙"} {
		p, err := a.store.Create("", title, emptyBody())
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	if err := a.reload(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestTabHistoryIndependentBranchAndRestart(t *testing.T) {
	a, _ := commandTestApp(t)
	ids := tabTestPages(t, a)
	a.selectPage(ids[0])
	a.selectPage(ids[1])
	first := a.tabs[0]
	a.openTab(ids[2])
	if a.canNavigate(-1) {
		t.Fatal("new tab inherited history")
	}
	a.selectPage(ids[0])
	a.navigateTab(-1)
	if a.selected != ids[2] {
		t.Fatal("back did not restore page")
	}
	a.switchTab(0)
	if !reflect.DeepEqual(a.tabs[0], first) || a.selected != ids[1] {
		t.Fatal("tab history was shared")
	}
	a.navigateTab(-1)
	if a.selected != ids[0] {
		t.Fatal("wrong previous page")
	}
	a.navigateTab(1)
	if a.selected != ids[1] {
		t.Fatal("forward failed")
	}
	a.navigateTab(-1)
	a.selectPage(ids[2])
	if a.canNavigate(1) {
		t.Fatal("new navigation did not discard forward history")
	}
	saved := tabSession{a.tabs, a.activeTab}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved, tabSession{reopened.tabs, reopened.activeTab}) || reopened.selected != a.selected {
		t.Fatal("session did not restore")
	}
}

func TestNativeTabsOpenSwitchCloseAndShortcuts(t *testing.T) {
	a, tt := commandTestApp(t)
	ids := tabTestPages(t, a)
	a.selectPage(ids[0])
	tt.Frame()
	count := len(a.pages)
	if err := tt.Click("新建标签页"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 1 || a.selected != ids[0] || !a.tabPicker.Open || len(a.pages) != count {
		t.Fatal("plus should open picker without changing page or tabs")
	}
	if err := tt.Click("标签页打开：乙"); err != nil {
		t.Fatal(err)
	}
	if a.selected != ids[1] {
		t.Fatal("page chooser did not navigate")
	}
	a.selectPage(ids[2])
	tt.Frame()
	if err := tt.Click("后退"); err != nil {
		t.Fatal(err)
	}
	if a.selected != ids[1] {
		t.Fatal("back button did not return to previous page")
	}
	if err := tt.Click("前进"); err != nil {
		t.Fatal(err)
	}
	if a.selected != ids[2] {
		t.Fatal("forward button did not restore page")
	}
	a.navigateTab(-1)
	tt.Frame()
	if err := tt.Click("切换标签页 1：甲"); err != nil {
		t.Fatal(err)
	}
	if a.selected != ids[0] {
		t.Fatal("tab click did not switch")
	}
	if err := tt.Click("笔记正文"); err != nil {
		t.Fatal(err)
	}
	tt.Type("保留的正文")
	tt.Key(ui.Ctrl, ui.KeyTab)
	if a.selected != ids[1] {
		t.Fatal("next tab shortcut failed")
	}
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyTab)
	if a.selected != ids[0] || a.current().doc.Text().String() != "保留的正文" {
		t.Fatal("previous tab lost draft")
	}
	tt.Key(ui.Cmd, ui.KeyT)
	if len(a.tabs) != 2 || !a.tabPicker.Open {
		t.Fatal("new tab shortcut failed")
	}
	if err := tt.Click("标签页打开：丙"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyW)
	if len(a.tabs) != 2 {
		t.Fatal("close shortcut failed")
	}
	if err := tt.Click("关闭标签页 2"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 1 || a.selected != ids[0] {
		t.Fatal("close did not select remaining tab")
	}
	a.closeTab(0)
	tt.Frame()
	if len(a.tabs) != 1 || a.current() != nil || len(a.pages) != count {
		t.Fatal("last tab close should leave an empty tab without deleting pages")
	}
	pages, err := a.store.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		if p.ID == ids[0] && p.Body.Text.String() != "保留的正文" {
			t.Fatal("closing lost edits")
		}
	}
}

func TestTabDeletionAndSharedDraft(t *testing.T) {
	a, _ := commandTestApp(t)
	ids := tabTestPages(t, a)
	a.selectPage(ids[0])
	doc := a.current().doc
	doc.InsertText("共享正文")
	a.openTab(ids[0])
	if a.current().doc != doc {
		t.Fatal("duplicate tab created a competing draft")
	}
	a.selectPage(ids[1])
	a.openTab(ids[0])
	a.deletePage(ids[0])
	for _, tab := range a.tabs {
		for _, id := range tab.History {
			if id == ids[0] {
				t.Fatal("deleted page remains in history")
			}
		}
	}
	a.switchTab(1)
	if a.selected != ids[1] {
		t.Fatal("deleting another page changed this tab")
	}
	a.closeTab(0)
	if a.activeTab != 0 || a.selected != ids[1] {
		t.Fatal("closing earlier tab changed active page")
	}
}

func TestTabsRejectSaveConflictAndRepairInvalidSession(t *testing.T) {
	a, _ := commandTestApp(t)
	id := a.selected
	d := a.current()
	other, err := a.store.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range other {
		if p.ID == id {
			p.Title = "外部修改"
			if err = a.store.Save(p); err != nil {
				t.Fatal(err)
			}
		}
	}
	d.doc.InsertText("尚未保存")
	a.openTab("")
	a.closeTab(0)
	if len(a.tabs) != 1 || a.selected != id || d.doc.Text().String() != "尚未保存" || !a.errorOpen {
		t.Fatal("save conflict discarded tab or draft")
	}
	for _, raw := range []string{`{`, `{"tabs":[{"history":[],"position":900}],"active":100}`, `{"tabs":[{"history":["deleted"],"position":-5}]}`} {
		if err := a.store.SetSetting("tabs", raw); err != nil {
			t.Fatal(err)
		}
		b, err := newApp(a.store)
		if err != nil {
			t.Fatal(err)
		}
		_ = b.tabs[b.activeTab].page()
	}
}

func TestTabOverflowKeepsActiveTabReachable(t *testing.T) {
	a, tt := commandTestApp(t)
	for i := 0; i < 10; i++ {
		a.openTab("")
	}
	tt.Frame()
	tt.Frame()
	rect, ok := tt.Find(fmt.Sprintf("切换标签页 %d：新标签页", len(a.tabs)))
	if !ok || rect.X < sidebarWidth || rect.X+rect.W > 1120 {
		t.Fatalf("active tab not visible: %+v", rect)
	}
	if err := tt.Click("关闭标签页 11"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 10 {
		t.Fatal("overflow tab cannot close")
	}
	if err := tt.Click("新建标签页"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 10 || !a.tabPicker.Open {
		t.Fatal("new-tab button became unreachable on overflow")
	}
	tt.Key(0, ui.KeyEscape)
	a.openTab("")
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	restoredUI := ui.NewTester(reopened.view, 850, 580)
	restoredUI.Frame()
	last, ok := restoredUI.Find("切换标签页 11：新标签页")
	if !ok || last.X < sidebarWidth || last.X+last.W > 850 {
		t.Fatalf("restored active tab is offscreen: %+v", last)
	}
}
