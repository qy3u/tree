package main

import (
	"fmt"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestPastePreservesExistingTodo(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		for _, command := range []string{"/todo", "/ttodo"} {
			for _, html := range []bool{false, true} {
				t.Run(fmt.Sprintf("leaf=%v/%s/html=%v", leaf, command, html), func(t *testing.T) {
					_, tt, d, _ := mathTestEditor(t, leaf)
					tt.Type(command)
					tt.Key(0, ui.KeyEnter)
					if command == "/ttodo" {
						tt.Key(0, ui.KeyEnter)
					}
					before := d.doc.Text().ParagraphStyleAt(0)
					if before.List != richtext.Tasks {
						t.Fatal("task not created")
					}
					if html {
						tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte(`<p><b>粘贴文字</b></p>`)))))
					} else {
						tt.SetClipboard("粘贴文字")
					}
					tt.Key(ui.Cmd, ui.KeyV)
					after := d.doc.Text().ParagraphStyleAt(0)
					if after.List != richtext.Tasks || after.Timer != before.Timer || after.ID != before.ID {
						t.Fatalf("paste changed task: before=%+v after=%+v", before, after)
					}
					if d.doc.Text().String() != "粘贴文字" {
						t.Fatal("wrong pasted text")
					}
					if html && d.doc.Text().AttributesAt(0).Weight < 600 {
						t.Fatal("lost inline formatting")
					}
					tt.Key(ui.Cmd, ui.KeyZ)
					if d.doc.Text().String() != "" || d.doc.Text().ParagraphStyleAt(0).List != richtext.Tasks {
						t.Fatal("undo did not restore empty task")
					}
					tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
					if d.doc.Text().String() != "粘贴文字" || d.doc.Text().ParagraphStyleAt(0).List != richtext.Tasks {
						t.Fatal("redo lost task")
					}
				})
			}
		}
	}
}

func TestMultilineRichPasteKeepsTaskIdentity(t *testing.T) {
	_, tt, d, _ := mathTestEditor(t, false)
	tt.Type("/ttodo")
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnter)
	original := d.doc.Text().ParagraphStyleAt(0)
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte(`<p>第一行</p><p>第二行</p>`)))))
	tt.Key(ui.Cmd, ui.KeyV)
	count := 0
	for pos, p := range d.doc.Text().Paragraphs() {
		if p.Style.ID == original.ID {
			count++
		}
		if pos == 0 && (p.Style.Timer != original.Timer || p.Style.List != richtext.Tasks) {
			t.Fatal("first line lost existing timed task")
		}
		if pos > 0 && p.Style.Timer.Enabled() {
			t.Fatal("pasted prose copied timer")
		}
	}
	if count != 1 {
		t.Fatalf("task identity duplicated or lost: %d", count)
	}
}
