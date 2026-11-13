package main

import (
	"strings"
	"testing"
)

func TestNotebookOverflowHasNoInternalScrollbar(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("正文" + strings.Repeat("\n", 40))
	d := a.current()
	r, ok := tt.Find("笔记正文")
	if !ok {
		t.Fatal("editor missing")
	}
	tt.Move(5, 100)
	before := tt.Image()
	tt.Move(r.X+r.W-3, r.Y+r.H/2)
	after := tt.Image()
	for x := int(r.X+r.W) - 6; x < int(r.X+r.W)-1; x++ {
		for y := int(r.Y) + 5; y < int(r.Y+r.H)-5; y++ {
			if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
				t.Fatal("hover painted an internal scrollbar")
			}
		}
	}
	offset := d.doc.ScrollOffset()
	tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 0, -150)
	if d.doc.ScrollOffset() >= offset {
		t.Fatal("wheel scrolling stopped working")
	}
	version := d.doc.Version()
	tt.Press(r.X+r.W-3, r.Y+r.H/2)
	tt.Release(r.X+r.W-3, r.Y+r.H/2)
	tt.Type("继续")
	if d.doc.Version() == version {
		t.Fatal("hidden scrollbar still intercepts editing")
	}
}
