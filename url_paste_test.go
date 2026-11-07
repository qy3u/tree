package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func TestURLPasteChoicesInPageAndLeaf(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		name := "page"
		if leaf {
			name = "leaf"
		}
		for _, choice := range []string{"Plain Text", "URL"} {
			t.Run(name+"/"+choice, func(t *testing.T) {
				var a *app
				var tt *ui.Tester
				var d *draft
				if leaf {
					var v *leafView
					a, tt, _, v = createTestLeaf(t)
					d = v.draft
					tt.Key(0, ui.KeyEnter)
				} else {
					a, tt = commandTestApp(t)
					d = a.current()
				}
				tt.Type("前文 待替换 后文")
				d.doc.SetSelection(3, 6)
				tt.Frame()
				const target = "https://www.binance.com/zh-CN/support/faq/detail/360033525071"
				tt.SetClipboard(target)
				tt.Key(ui.Cmd, ui.KeyV)
				tt.Frame()
				if got := d.doc.Text().String(); got != "前文 "+target+" 后文" {
					t.Fatalf("paste lost content: %q", got)
				}
				if a.urlPaste == nil || a.urlPaste.draft != d {
					t.Fatal("missing paste choice")
				}
				if _, ok := tt.Find("Paste as"); !ok {
					t.Fatal("menu missing")
				}
				if path := os.Getenv("TREE_URL_PASTE_SCREENSHOT"); path != "" && !leaf && choice == "URL" {
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
				if err := tt.Click("粘贴为 " + choice); err != nil {
					t.Fatal(err)
				}
				if a.urlPaste != nil {
					t.Fatal("menu remained open")
				}
				want := target
				if choice == "Plain Text" {
					want = ""
				}
				for i := 3; i < 3+len([]rune(target)); i++ {
					if d.doc.Text().AttributesAt(i).Link != want {
						t.Fatal("wrong link attributes")
					}
				}
				if d.doc.Text().AttributesAt(3+len([]rune(target))).Link != "" {
					t.Fatal("linked surrounding text")
				}
				if err := a.flush(); err != nil {
					t.Fatal(err)
				}
				tt.Key(ui.Cmd, ui.KeyZ)
				if choice == "Plain Text" {
					tt.Key(ui.Cmd, ui.KeyZ)
				}
				if d.doc.Text().String() != "前文 待替换 后文" {
					t.Fatal("undo did not restore selection")
				}
			})
		}
	}
}

func TestURLPasteKeyboardDismissalAndStaleRange(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	const target = "https://example.com/path?q=你好#section"
	paste := func() { tt.SetClipboard(target); tt.Key(ui.Cmd, ui.KeyV); tt.Frame() }
	paste()
	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyEnter)
	if a.urlPaste != nil || d.doc.Text().AttributesAt(0).Link != "" || d.doc.Text().String() != target {
		t.Fatal("keyboard Plain Text failed")
	}
	d.doc.SetSelection(d.doc.Text().Len(), d.doc.Text().Len())
	tt.Frame()
	paste()
	tt.Key(0, ui.KeyEscape)
	if a.urlPaste != nil || d.doc.Text().AttributesAt(len([]rune(target))).Link != target {
		t.Fatal("Escape lost pasted URL")
	}
	d.doc.SetText(richtext.NewText(""))
	d.doc.SetSelection(0, 0)
	tt.Frame()
	paste()
	tt.Type(" 后续文字")
	tt.Frame()
	if a.urlPaste != nil {
		t.Fatal("typing left stale popup")
	}
	d.doc.SetText(richtext.NewText(""))
	d.doc.SetSelection(0, 0)
	tt.Frame()
	paste()
	tt.Key(ui.Cmd, ui.KeyZ)
	if d.doc.Text().String() != "" || a.urlPaste != nil {
		t.Fatal("immediate undo failed")
	}
}

func TestURLPasteSkipsCodePlainPasteAndProse(t *testing.T) {
	for _, kind := range []string{"code", "plain-paste", "prose"} {
		t.Run(kind, func(t *testing.T) {
			a, tt := commandTestApp(t)
			raw := "https://example.com"
			if kind == "code" {
				tt.Type("/code")
				tt.Key(0, ui.KeyEnter)
			}
			if kind == "prose" {
				raw = "See https://example.com for details"
			}
			tt.SetClipboard(raw)
			mods := ui.Cmd
			if kind == "plain-paste" {
				mods |= ui.Shift | ui.Alt
			}
			tt.Key(mods, ui.KeyV)
			if a.urlPaste != nil {
				t.Fatal("unexpected URL menu")
			}
			if a.current().doc.Text().String() != raw {
				t.Fatalf("paste dropped text: %q", a.current().doc.Text().String())
			}
		})
	}
}

func TestURLPasteClipboardFormats(t *testing.T) {
	const target = "https://example.com/doc#page=4"
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{target, true}, {" https://example.com/doc#page=4\n", true}, {"https://example.com/a b", false}, {"javascript:alert(1)", false}, {"tree://page/id", false}, {"https://", false}, {"不是链接", false}, {"https://one.com\nhttps://two.com", false},
	} {
		data := transfer.New(transfer.NewItem(transfer.Bytes(transfer.Text, []byte(tc.raw))))
		if got := pastedWebURL(data); (got != "") != tc.want {
			t.Fatalf("%q => %q", tc.raw, got)
		}
	}
	a, tt := commandTestApp(t)
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte(`<a href="`+target+`">`+target+`</a>`)))))
	tt.Key(ui.Cmd, ui.KeyV)
	if a.urlPaste == nil || a.current().doc.Text().String() != target {
		t.Fatal("HTML URL not handled")
	}
}

func TestURLPasteNativeURIClipboard(t *testing.T) {
	a, tt := commandTestApp(t)
	const target = "https://example.com/native#page=4"
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.URIList, []byte("# URL\r\n"+target+"\r\n")))))
	tt.Key(ui.Cmd, ui.KeyV)
	if a.urlPaste == nil || a.current().doc.Text().String() != target {
		t.Fatal("native URL clipboard pasted nothing")
	}
}
