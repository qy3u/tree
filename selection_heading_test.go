package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSelectionToolbarHeadingClick(t *testing.T) {
	const first = "第一段要保留很长的文字"
	const second = "第二段标题"
	const content = first + "\n" + second + "\n第三段正文"
	secondStart := len([]rune(first)) + 1
	thirdStart := secondStart + len([]rune(second)) + 1
	for _, tc := range []struct {
		name                          string
		start, end                    int
		firstSelected, secondSelected bool
	}{
		{"first", 0, 4, true, false},
		{"later", secondStart + 1, secondStart + 4, false, true},
		{"backward", secondStart + 4, secondStart + 1, false, true},
		{"multiple", 3, thirdStart, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, tt := commandTestApp(t)
			tt.Type(content)
			d := a.current()
			d.doc.SetSelection(tc.start, tc.end)
			tt.Frame()
			check := func(level int) {
				t.Helper()
				text := d.doc.Text()
				if text.String() != content {
					t.Fatal("formatting changed the text")
				}
				for _, para := range []struct {
					pos      int
					selected bool
				}{{0, tc.firstSelected}, {secondStart, tc.secondSelected}, {thirdStart, false}} {
					want := 0
					if para.selected {
						want = level
					}
					if got := text.ParagraphStyleAt(para.pos).Heading; got != want {
						t.Fatalf("paragraph at %d heading=%d, want=%d", para.pos, got, want)
					}
				}
				anchor, caret := d.doc.Selection()
				if anchor != tc.start || caret != tc.end {
					t.Fatalf("formatting lost selection: %d,%d", anchor, caret)
				}
			}
			for _, level := range []int{1, 2, 3, 4, 5, 6, 0, 2} {
				if err := tt.Click("段落样式"); err != nil {
					t.Fatal(err)
				}
				// The menu must keep its owner and selection when the pointer leaves.
				tt.Move(1080, 760)
				tt.Frame()
				label := "正文"
				if level > 0 {
					label = fmt.Sprintf("H%d", level)
				}
				if err := tt.ChooseMenuItem(label); err != nil {
					t.Fatal(err)
				}
				check(level)
				if !tt.Focused("笔记正文") {
					t.Fatal("formatting did not restore editor focus")
				}
			}
			tt.Key(ui.Cmd, ui.KeyZ)
			check(0)
			tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
			check(2)
			if err := a.flush(); err != nil {
				t.Fatal(err)
			}
			reopened, err := newApp(a.store)
			if err != nil {
				t.Fatal(err)
			}
			d = reopened.current()
			if d.doc.Text().ParagraphStyleAt(min(tc.start, tc.end)).Heading != 2 || d.doc.Text().String() != content {
				t.Fatal("heading or text lost after reopening")
			}
		})
	}
}
