package main

import "testing"

func TestSidebarDeleteUnopenedPageAfterPointerLeavesRow(t *testing.T) {
	for _, method := range []string{"button", "context"} {
		t.Run(method, func(t *testing.T) {
			a, tt := commandTestApp(t)
			ids := tabTestPages(t, a)
			a.selectPage(ids[0])
			tt.Frame()
			doc := a.current().doc
			doc.InsertText("当前页面草稿")
			before := a.tabs[a.activeTab]
			row, ok := tt.Find("乙")
			if !ok {
				t.Fatal("missing other page")
			}
			tt.Move(row.X+70, row.Y+row.H/2)
			if method == "button" {
				// Tree rows expose a single accessibility item; click their trailing
				// action button at its visible position.
				tt.ClickAt(row.X+row.W-12, row.Y+row.H/2)
			} else {
				tt.RightClickAt(row.X+70, row.Y+row.H/2)
			}
			if len(tt.Menu()) == 0 {
				t.Fatal("page actions menu did not open")
			}
			tt.Move(500, 500)
			tt.Frame()
			if err := tt.ChooseMenuItem("删除页面"); err != nil {
				t.Fatal(err)
			}
			if a.find(ids[1]).Deleted == "" {
				t.Fatal("other page was not deleted")
			}
			if a.selected != ids[0] || a.current().doc != doc || doc.Text().String() != "当前页面草稿" {
				t.Fatal("deleting another page disturbed current draft")
			}
			if a.tabs[a.activeTab].page() != before.page() {
				t.Fatal("deletion navigated current tab")
			}
		})
	}
}
