package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestSlashPageStartsWithEmptyFocusedTitle(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		t.Run(map[bool]string{false: "enter", true: "click"}[mouse], func(t *testing.T) {
			a, tt := commandTestApp(t)
			parent := a.selected
			tt.Type("/page")
			if mouse {
				if err := tt.Click("命令：子页面"); err != nil {
					t.Fatal(err)
				}
			} else {
				tt.Key(0, ui.KeyEnter)
			}
			d := a.current()
			if d.page.ID == parent || d.page.Parent != parent {
				t.Fatal("did not create child")
			}
			if d.page.Title != "" || pageTitle(d.page) != "无标题" {
				t.Fatal("placeholder stored as actual title")
			}
			if !tt.Focused("页面标题") {
				t.Fatal("new title was not focused")
			}
			tt.Type("阅读笔记")
			if d.page.Title != "阅读笔记" || d.doc.Text().Len() != 0 {
				t.Fatal("typing did not directly name new page")
			}
			tt.Key(0, ui.KeyEnter)
			if !tt.Focused("笔记正文") {
				t.Fatal("title Enter did not enter body")
			}
			tt.Type("第一条记录")
			if err := a.flush(); err != nil {
				t.Fatal(err)
			}
			reopened, err := newApp(a.store)
			if err != nil {
				t.Fatal(err)
			}
			if reopened.current().page.Title != "阅读笔记" || reopened.current().doc.Text().PlainText() != "第一条记录" {
				t.Fatal("new page not saved")
			}
		})
	}
}
