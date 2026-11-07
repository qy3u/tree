package main

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func TestLinkHoverActions(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		name := "page"
		if leaf {
			name = "leaf"
		}
		t.Run(name, func(t *testing.T) {
			var a *app
			var tt *ui.Tester
			var d *draft
			label := "笔记正文"
			if leaf {
				var v *leafView
				a, tt, _, v = createTestLeaf(t)
				d, label = v.draft, "Leaf 正文"
			} else {
				a, tt = commandTestApp(t)
				d = a.current()
			}
			const target = "https://example.com/static/documents/exchange-procedures.pdf#page=37"
			const replacement = "https://example.com/updated.pdf#page=4"
			d.doc.SetText(richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{
				{Text: "[1] "},
				{Text: "Exchange ", Attributes: richtext.Attributes{Link: target}},
				{Text: "Procedures，第138–139条", Attributes: richtext.Attributes{Link: target, Weight: 700}},
				{Text: " 后面的正文"},
			}}))
			d.doc.SetSelection(0, 0)
			tt.Frame()
			tt.Frame()
			r, _ := tt.Find(label)
			p := d.doc.VisibleParagraphs()[0]
			x, y := r.X+48, r.Y+p.Top+p.Height/2
			tt.Move(x, y)
			tt.Frame()
			if _, ok := tt.Find("原始链接：" + target); !ok {
				t.Fatal("hover URL missing")
			}
			if a.linkOpen {
				t.Fatal("hover opened editor")
			}
			if anchor, caret := d.doc.Selection(); anchor != 0 || caret != 0 {
				t.Fatal("hover changed selection")
			}
			copyRect, ok := tt.Find("复制链接地址")
			if !ok {
				t.Fatal("copy missing")
			}
			tt.Move(copyRect.X+copyRect.W/2, copyRect.Y+copyRect.H/2)
			tt.Advance(time.Second)
			if err := tt.Click("复制链接地址"); err != nil {
				t.Fatal(err)
			}
			if tt.Clipboard() != target {
				t.Fatalf("clipboard: %q", tt.Clipboard())
			}
			if len(tt.OpenedURLs()) != 0 {
				t.Fatal("hover/copy navigated")
			}
			if path := os.Getenv("TREE_LINK_HOVER_SCREENSHOT"); path != "" && leaf {
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tt.Image())
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := tt.Click("编辑链接地址"); err != nil {
				t.Fatal(err)
			}
			if !a.linkOpen || a.linkURL != target || a.editing() != d {
				t.Fatal("wrong edit target")
			}
			if path := os.Getenv("TREE_LINK_EDIT_SCREENSHOT"); path != "" && leaf {
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tt.Image())
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			lo, hi := d.doc.Selection()
			if lo != 4 || hi != 4+len([]rune("Exchange Procedures，第138–139条")) {
				t.Fatalf("link selection %d:%d", lo, hi)
			}
			a.linkURL = replacement
			tt.Key(0, ui.KeyEnter)
			if got := d.doc.Text().AttributesAt(4).Link; got != replacement {
				t.Fatal(got)
			}
			if !d.doc.Text().AttributesAt(14).Bold() {
				t.Fatal("edit lost bold")
			}
			if d.doc.Text().AttributesAt(hi).Link != "" {
				t.Fatal("edit included adjacent prose")
			}
			d.doc.SetSelection(0, 0)
			tt.Frame()
			tt.Move(x, y)
			tt.Frame()
			if err := tt.Click("编辑链接地址"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("移除链接"); err != nil {
				t.Fatal(err)
			}
			if d.doc.Text().AttributesAt(4).Link != "" {
				t.Fatal("link not removed")
			}
			d.doc.Undo()
			d.doc.SetSelection(0, 0)
			tt.Frame()
			tt.Move(x, y)
			tt.Frame()
			if _, ok := tt.Find("链接浮层"); !ok {
				t.Fatal("undo link not hoverable")
			}
			tt.Move(10, 790)
			tt.Advance(time.Second)
			tt.Frame()
			if _, ok := tt.Find("复制链接地址"); ok {
				t.Fatal("popover did not dismiss")
			}
			tt.Move(r.X+r.W-5, y)
			tt.Frame()
			if _, ok := tt.Find("复制链接地址"); ok {
				t.Fatal("whitespace triggered popover")
			}
			if err := a.flush(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLinkHoverWrappedTextAndStaleRange(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	const target = "https://example.com/reference"
	text := "这是一段比较长的链接文字，用来验证换行之后仍然能看到完整地址，并且可以编辑整段链接。这里还需要继续写一些内容，让链接跨过至少两行。"
	d.doc.SetText(richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: text, Attributes: richtext.Attributes{Link: target}}}}))
	d.doc.SetSelection(0, 0)
	tt.Frame()
	tt.Frame()
	r, _ := tt.Find("笔记正文")
	p := d.doc.VisibleParagraphs()[0]
	hits := 0
	for y := p.Top + 2; y < p.Top+p.Height; y += 6 {
		hit, ok := d.doc.LinkAt(12, y)
		if !ok {
			continue
		}
		if hit.URL != target || hit.Start != 0 || hit.End != len([]rune(text)) {
			t.Fatalf("wrapped hit: %+v", hit)
		}
		hits++
	}
	if hits < 6 {
		t.Fatalf("too few wrapped-line hits: %d", hits)
	}
	tt.Move(r.X+12, r.Y+p.Top+12)
	tt.Frame()
	if _, ok := tt.Find("复制链接地址"); !ok {
		t.Fatal("popover absent")
	}
	d.doc.SetText(richtext.NewText("已经替换成普通文字"))
	tt.Frame()
	if _, ok := tt.Find("复制链接地址"); ok {
		t.Fatal("stale link survived replacement")
	}
}
