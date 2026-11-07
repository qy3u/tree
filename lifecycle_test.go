package main

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/treerichtext"
)

func TestWindowCloseSavesAndHidesWithoutDestroyingSession(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	tt.Type("保留编辑状态")
	before := tabSession{a.tabs, a.activeTab}
	hidden := 0
	a.hide = func() {
		hidden++
		pages, err := a.store.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range pages {
			if p.ID == d.page.ID && p.Body.Text.String() == "保留编辑状态" {
				return
			}
		}
		t.Fatal("window hidden before saving")
	}
	for i := 0; i < 2; i++ {
		e := &mygo.CloseEvent{}
		a.closeWindow(e)
		if !e.DefaultPrevented() {
			t.Fatal("native close was allowed to destroy window")
		}
		if a.quitting || a.current() != d || !reflect.DeepEqual(before, tabSession{a.tabs, a.activeTab}) {
			t.Fatal("hide changed live session")
		}
	}
	if hidden != 2 {
		t.Fatal("close did not hide")
	}
	// A genuine quit must pass through the same close handler without hiding.
	quit := &mygo.QuitEvent{}
	a.beforeQuit(quit)
	close := &mygo.CloseEvent{}
	a.closeWindow(close)
	if quit.DefaultPrevented() || close.DefaultPrevented() || !a.quitting || hidden != 2 {
		t.Fatal("explicit quit was intercepted")
	}
}

func TestWindowCloseAndQuitRejectSaveConflict(t *testing.T) {
	a, _ := commandTestApp(t)
	d := a.current()
	pages, err := a.store.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		if p.ID == d.page.ID {
			p.Title = "另一个版本"
			if err := a.store.Save(p); err != nil {
				t.Fatal(err)
			}
		}
	}
	d.doc.InsertText("必须保留的草稿")
	a.hide = func() { t.Fatal("save conflict hid window") }
	close := &mygo.CloseEvent{}
	a.closeWindow(close)
	if !close.DefaultPrevented() || !a.errorOpen {
		t.Fatal("save error not preserved")
	}
	quit := &mygo.QuitEvent{}
	a.beforeQuit(quit)
	if !quit.DefaultPrevented() || a.quitting || d.doc.Text().String() != "必须保留的草稿" {
		t.Fatal("save conflict allowed quit")
	}
}

func TestBackgroundKeepsTimedTaskRunning(t *testing.T) {
	a, tt := commandTestApp(t)
	// Use the document timer directly so hiding is tested independently of controls.
	d := a.current()
	d.doc.FormatParagraphs(func(p *richtext.ParagraphStyle) {
		p.List = richtext.Tasks
		p.Timer = (richtext.TaskTimer{State: richtext.TimerIdle}).Start(tt.Now())
	})
	before := d.doc.Text().ParagraphStyleAt(0).Timer
	a.hide = func() {}
	a.closeWindow(&mygo.CloseEvent{})
	if a.taskCheckStopped || d.doc.Text().ParagraphStyleAt(0).Timer != before {
		t.Fatal("hiding stopped timer")
	}
}
