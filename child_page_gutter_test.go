package main

import (
	"math"
	"testing"
)

func TestChildPageHandleMenuDrag(t *testing.T) {
	for _, withIcon := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "icon"}[withIcon], func(t *testing.T) {
			a, tt := commandTestApp(t)
			parent := a.selected
			doc := a.current().doc
			tt.Type("正文保持不变")
			first, err := a.store.Create(parent, "子页面甲", emptyBody())
			if err != nil {
				t.Fatal(err)
			}
			second, err := a.store.Create(parent, "子页面乙", emptyBody())
			if err != nil {
				t.Fatal(err)
			}
			a.reload()
			if withIcon {
				a.setPageIcon(parent, "books")
			}
			tt.Frame()
			row, ok := tt.Find("打开子页面：子页面甲")
			if !ok {
				t.Fatal("missing child row")
			}
			tt.Move(row.X+60, row.Y+row.H/2)
			tt.Frame()
			grip, ok := tt.Find("移动子页面：子页面甲")
			if !ok {
				t.Fatal("missing child grip")
			}
			if math.Abs(float64(grip.Y+grip.H/2-row.Y-row.H/2)) > 1 {
				t.Fatalf("not centered: grip %+v row %+v", grip, row)
			}
			ed, _ := tt.Find("笔记正文")
			if math.Abs(float64(grip.X+grip.W/2-(ed.X-gutterWidth+24))) > 1 {
				t.Fatal("grip not aligned with paragraph handles")
			}
			tt.ClickAt(grip.X+grip.W/2, grip.Y+grip.H/2)
			tt.Move(500, 600)
			tt.Frame()
			if err := tt.Click("复制页面链接"); err != nil {
				t.Fatal(err)
			}
			if tt.Clipboard() != referenceURL(first.ID, "") {
				t.Fatal("menu copied wrong link")
			}
			if a.selected != parent || doc.Text().PlainText() != "正文保持不变" {
				t.Fatal("menu changed current page")
			}
			tt.Frame()
			row, _ = tt.Find("打开子页面：子页面甲")
			tt.Move(row.X+60, row.Y+row.H/2)
			grip, _ = tt.Find("移动子页面：子页面甲")
			destination, _ := tt.Find("打开子页面：子页面乙")
			x, y := grip.X+grip.W/2, grip.Y+grip.H/2
			tt.Press(x, y)
			tt.Move(x, y+7)
			tt.Move(destination.X+100, destination.Y+destination.H-3)
			tt.Release(destination.X+100, destination.Y+destination.H-3)
			tt.Frame()
			children := sortedChildren(a.pages, parent)
			if children[0].ID != second.ID || children[1].ID != first.ID {
				t.Fatal("child grip drag did not reorder pages")
			}
			if a.selected != parent || doc.Text().PlainText() != "正文保持不变" {
				t.Fatal("drag changed body or navigated")
			}
			// Delete through the handle after the pointer leaves the page entry.
			row, _ = tt.Find("打开子页面：子页面甲")
			tt.Move(row.X+60, row.Y+row.H/2)
			grip, _ = tt.Find("移动子页面：子页面甲")
			tt.ClickAt(grip.X+12, grip.Y+13)
			tt.Move(500, 600)
			tt.Frame()
			if err := tt.Click("删除页面"); err != nil {
				t.Fatal(err)
			}
			if a.find(first.ID).Deleted == "" || a.selected != parent {
				t.Fatal("child handle delete failed")
			}
			// Moving over a regular paragraph still reveals its native handle.
			ed, _ = tt.Find("笔记正文")
			para := doc.VisibleParagraphs()[0]
			tt.Move(ed.X+40, ed.Y+para.FirstLineCenter)
			if _, ok := tt.Find("移动段落 1"); !ok {
				t.Fatal("body handle regressed")
			}
		})
	}
}
