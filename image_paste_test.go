package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func clipboardImageFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 160, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 160; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 40, G: 130, B: 90, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestImagePasteIntoPageAndLeaf(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		for _, position := range []string{"empty", "middle", "selection"} {
			name := "page"
			if leaf {
				name = "leaf"
			}
			t.Run(name+"/"+position, func(t *testing.T) {
				var a *app
				var tt *ui.Tester
				var d, root *draft
				if leaf {
					var v *leafView
					a, tt, root, v = createTestLeaf(t)
					d = v.draft
					tt.Key(0, ui.KeyEnter)
				} else {
					a, tt = commandTestApp(t)
					d = a.current()
					root = d
				}
				switch position {
				case "middle":
					tt.Type("前文后文")
					d.doc.SetSelection(2, 2)
				case "selection":
					tt.Type("前文替换后文")
					d.doc.SetSelection(2, 4)
				}
				tt.Frame()
				original := d.doc.Text().String()
				data := clipboardImageFixture(t)
				tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, data))))
				tt.Key(ui.Cmd, ui.KeyV)
				tt.Frame()
				if a.errorOpen {
					t.Fatal(a.failure)
				}
				attachments := (Body{Text: d.doc.Text()}).attachments()
				if len(attachments) != 1 {
					t.Fatalf("image not inserted: %q", d.doc.Text().String())
				}
				at := attachments[0]
				if at.Image == nil || at.Width != 160 || at.Height != 80 {
					t.Fatal("image not decoded")
				}
				if position != "empty" && d.doc.Text().String() != "前文\n"+string(richtext.ObjectReplacement)+"\n后文" {
					t.Fatal("paste damaged surrounding text")
				}
				path := filepath.Join(a.store.dir, "assets", strings.TrimPrefix(at.URL, "asset:"))
				saved, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(saved, data) {
					t.Fatal("asset not saved")
				}
				tt.Key(ui.Cmd, ui.KeyZ)
				if d.doc.Text().String() != original {
					t.Fatal("image paste did not undo in one step")
				}
				tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
				if len((Body{Text: d.doc.Text()}).attachments()) != 1 {
					t.Fatal("image redo failed")
				}
				if err := a.flush(); err != nil {
					t.Fatal(err)
				}
				reopened, err := newApp(a.store)
				if err != nil {
					t.Fatal(err)
				}
				p := reopened.find(root.page.ID)
				body := p.Body
				if leaf {
					body = body.Leaves[d.leafID].Content
				}
				ats := (Body{Text: hydrate(body, a.store.dir)}).attachments()
				if len(ats) != 1 || ats[0].Image == nil {
					t.Fatal("saved image failed to reload")
				}
			})
		}
	}
}

func TestInvalidClipboardImagePreservesSelection(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	tt.Type("不应该丢失的文字")
	d.doc.SetSelection(0, d.doc.Text().Len())
	tt.Frame()
	before := d.doc.Text()
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, []byte("not a PNG")))))
	tt.Key(ui.Cmd, ui.KeyV)
	if !a.errorOpen || !d.doc.Text().Equal(before) {
		t.Fatal("invalid screenshot erased text or failed silently")
	}
}
