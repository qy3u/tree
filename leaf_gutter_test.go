package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func clickLeafGrip(t *testing.T, tt *ui.Tester, d *draft) {
	t.Helper()
	ed, ok := tt.Find("笔记正文")
	if !ok {
		t.Fatal("page editor missing")
	}
	var y float32
	found := false
	for _, at := range d.doc.VisibleAttachments() {
		if leafID(at.Attachment) != "" {
			y = ed.Y + at.Rect.Y + 26
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Leaf missing")
	}
	x := ed.X - gutterWidth + 24
	tt.Move(x, y)
	tt.Frame()
	tt.ClickAt(x, y)
	tt.Frame()
	tt.Frame()
}

func TestLeafGripMenuWhileEditingLeaf(t *testing.T) {
	for _, focus := range []string{"title", "body", "collapsed"} {
		t.Run(focus, func(t *testing.T) {
			a, tt, d, v := createTestLeaf(t)
			tt.Type("知识单元")
			if focus == "body" {
				tt.Key(0, ui.KeyEnter)
				tt.Type("内部文字")
			} else if focus == "collapsed" {
				if err := tt.Click("收起 Leaf"); err != nil {
					t.Fatal(err)
				}
			}
			before := v.draft.doc.Text()
			clickLeafGrip(t, tt, d)
			if !a.blockMenu {
				t.Fatal("Leaf grip menu closed immediately")
			}
			if _, ok := tt.Find("删除 Leaf"); !ok {
				t.Fatal("Leaf menu missing")
			}
			if _, ok := tt.Find("转为一级标题"); ok {
				t.Fatal("opened inner block menu")
			}
			if err := tt.Click("在下方插入"); err != nil {
				t.Fatal(err)
			}
			if !v.draft.doc.Text().Equal(before) {
				t.Fatal("Leaf action changed its contents")
			}
			if !tt.Focused("笔记正文") {
				t.Fatal("action focused inner editor instead of page")
			}
			tt.Type("Leaf 后的文字")
			if !v.draft.doc.Text().Equal(before) {
				t.Fatal("typing after action entered Leaf")
			}
		})
	}
}

func TestLeafGripMenuRepeatedAndInnerMenu(t *testing.T) {
	a, tt, d, v := createTestLeaf(t)
	tt.Type("Leaf 标题")
	tt.Key(0, ui.KeyEnter)
	tt.Type("第一段\n第二段")
	for i := 0; i < 3; i++ {
		clickLeafGrip(t, tt, d)
		if !a.blockMenu || a.blockMenuPage != d.page.ID {
			t.Fatal("outer menu lost owner")
		}
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if a.blockMenu {
			t.Fatal("Escape failed")
		}
	}
	// The inner paragraph menu has its own owner and still edits only that paragraph.
	ed, _ := tt.Find("Leaf 正文")
	para := v.draft.doc.VisibleParagraphs()[0]
	x, y := ed.X-gutterWidth+24, ed.Y+para.FirstLineCenter
	tt.Move(x, y)
	tt.Frame()
	tt.ClickAt(x, y)
	tt.Frame()
	tt.Frame()
	if !a.blockMenu || a.blockMenuPage != v.draft.page.ID {
		t.Fatal("inner menu lost owner")
	}
	if _, ok := tt.Find("删除 Leaf"); ok {
		t.Fatal("inner grip opened Leaf menu")
	}
	if err := tt.Click("转为一级标题"); err != nil {
		t.Fatal(err)
	}
	if v.draft.doc.Text().ParagraphStyleAt(0).Heading != 1 {
		t.Fatal("inner conversion failed")
	}
	if d.doc.Text().ParagraphStyleAt(0).Heading != 0 {
		t.Fatal("inner action changed outer Leaf")
	}
}

func TestLeafGripDuplicateDeleteWithAnotherLeafFocused(t *testing.T) {
	for _, action := range []string{"复制 Leaf", "删除 Leaf"} {
		t.Run(action, func(t *testing.T) {
			a, tt, d, first := createTestLeaf(t)
			tt.Type("第一个 Leaf")
			// Build a sibling and keep its title focused while opening the first Leaf's menu.
			a.editorDraft = d
			d.doc.SetSelection(d.doc.Text().Len(), d.doc.Text().Len())
			a.createLeaf(d)
			tt.Frame()
			tt.Frame()
			tt.Type("第二个 Leaf")
			before := d.doc.Text()
			clickLeafGrip(t, tt, d)
			if !a.blockMenu {
				t.Fatal("sibling focus dismissed menu")
			}
			if err := tt.Click(action); err != nil {
				t.Fatal(err)
			}
			if action == "删除 Leaf" && bodyHasLeaf(d.doc.Text(), first.draft.leafID) {
				t.Fatal("first Leaf not deleted")
			}
			count := 0
			for _, r := range d.doc.Text().Runs(0, d.doc.Text().Len()) {
				if leafID(r.Attributes.Attachment) != "" {
					count++
				}
			}
			want := 1
			if action == "复制 Leaf" {
				want = 3
			}
			if count != want {
				t.Fatalf("Leaf count %d, want %d", count, want)
			}
			tt.Key(ui.Cmd, ui.KeyZ)
			if !d.doc.Text().Equal(before) {
				t.Fatal("Leaf menu action did not undo in parent")
			}
		})
	}
}
