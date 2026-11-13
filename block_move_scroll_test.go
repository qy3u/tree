package main

import (
	"github.com/egoist/mygo/treerichtext"
	"math"
	"strings"
	"testing"
)

func TestBlockMoveKeepsViewport(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	d.doc.InsertText(strings.Repeat("正文内容\n", 100))
	d.doc.SetSelection(0, 0)
	tt.Frame()
	tt.Frame()
	d.doc.ScrollBy(1200)
	tt.Frame()
	for _, move := range [][2]int{{40, 48}, {48, 35}, {2, 90}} {
		before := d.doc.ScrollOffset()
		if !moveParagraph(d.doc, move[0], move[1]) {
			t.Fatal("move failed")
		}
		tt.Frame()
		tt.Frame()
		if math.Abs(float64(d.doc.ScrollOffset()-before)) > 1 {
			t.Fatalf("viewport jumped: before=%v after=%v", before, d.doc.ScrollOffset())
		}
	}
	// Typing still reveals the caret normally after the one-shot suppression.
	d.doc.SetSelection(d.doc.Text().Len(), d.doc.Text().Len())
	tt.Frame()
	if d.doc.ScrollOffset() < 2000 {
		t.Fatal("ordinary caret reveal was disabled")
	}
}

func TestLeafBlockMoveKeepsParentViewport(t *testing.T) {
	a, tt, root, v := createTestLeaf(t)
	child := v.draft
	child.doc.InsertText(strings.Repeat("知识内容\n", 60))
	root.doc.SetSelection(root.doc.Text().Len(), root.doc.Text().Len())
	root.doc.InsertText(strings.Repeat("\n", 40))
	tt.Frame()
	tt.Frame()
	root.doc.ScrollBy(-100000)
	tt.Frame()
	root.doc.ScrollBy(400)
	tt.Frame()
	before := root.doc.ScrollOffset()
	a.dragParagraph(child, 0, 50)
	tt.Frame()
	tt.Frame()
	if math.Abs(float64(root.doc.ScrollOffset()-before)) > 1 {
		t.Fatalf("Leaf reorder scrolled page: %v -> %v", before, root.doc.ScrollOffset())
	}
	if child.doc.Text().ParagraphStyleAt(0).List != richtext.NoList {
		t.Fatal("unexpected style change")
	}
}

func TestNativeBlockDropKeepsViewport(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	d.doc.InsertText(strings.Repeat("段落\n", 100))
	d.doc.SetSelection(0, 0)
	tt.Frame()
	tt.Frame()
	d.doc.ScrollBy(1200)
	tt.Frame()
	editor, _ := tt.Find("笔记正文")
	boxes := d.doc.VisibleParagraphs()
	if len(boxes) < 6 {
		t.Fatal("too few paragraphs")
	}
	source, target := boxes[1], boxes[5]
	x := editor.X - gutterWidth + 24
	sy := editor.Y + source.FirstLineCenter
	ty := editor.Y + target.FirstLineCenter
	tt.Move(x, sy)
	tt.Press(x, sy)
	tt.Move(x, ty)
	before := d.doc.ScrollOffset()
	version := d.doc.Version()
	tt.Release(x, ty)
	tt.Frame()
	tt.Frame()
	if d.doc.Version() == version {
		t.Fatal("native drop did not move block")
	}
	if math.Abs(float64(d.doc.ScrollOffset()-before)) > 1 {
		t.Fatalf("drop jumped: %v -> %v", before, d.doc.ScrollOffset())
	}
}
