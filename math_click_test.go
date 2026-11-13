package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

func TestFormulaClicksUseRenderedGeometry(t *testing.T) {
	for _, leaf := range []bool{false, true} {
		for _, formula := range []string{`\[\frac{a}{b}\]`, `前面文字 \(x_i^2\) 后面文字`} {
			t.Run(fmt.Sprintf("leaf=%v/%s", leaf, formula), func(t *testing.T) {
				a, tt, d, _ := mathTestEditor(t, leaf)
				tt.Type(strings.Repeat("前文\n", 12))
				tt.SetClipboard(formula)
				tt.Key(ui.Cmd, ui.KeyV)
				tt.Frame()
				tt.Frame()
				for _, fraction := range []float32{.05, .5, .95} {
					label := "笔记正文"
					if leaf {
						label = "Leaf 正文"
					}
					editor, ok := tt.Find(label)
					if !ok {
						t.Fatal("editor missing")
					}
					boxes := d.doc.VisibleAttachments()
					if len(boxes) == 0 {
						t.Fatal("formula missing")
					}
					box := boxes[len(boxes)-1]
					x, y := editor.X+box.Rect.X+box.Rect.W*fraction, editor.Y+box.Rect.Y+box.Rect.H/2
					tt.Press(x, y)
					tt.Release(x, y)
					if a.mathEdit == nil || a.mathEdit.start != box.Start || !tt.Focused("公式 LaTeX") {
						t.Fatalf("click at %.2f did not edit formula", fraction)
					}
					panel, exists := tt.Find("编辑公式")
					if !exists {
						t.Fatal("formula panel missing")
					}
					bottom := editor.Y + box.Rect.Y + box.Rect.H
					top := editor.Y + box.Rect.Y
					gapBelow := panel.Y - bottom
					gapAbove := top - (panel.Y + panel.H)
					if (gapBelow < -1 || gapBelow > 20) && (gapAbove < -1 || gapAbove > 20) {
						t.Fatalf("panel detached from formula: formula y=%v..%v panel=%+v", top, bottom, panel)
					}
					tt.Key(0, ui.KeyEscape)
					tt.Frame()
				}
			})
		}
	}
}

func TestFormulaPanelInScrolledTallLeaf(t *testing.T) {
	a, tt, d, root := mathTestEditor(t, true)
	tt.Type(strings.Repeat("前面的内容\n", 24))
	tt.SetClipboard(`\[\frac{x}{y}\]`)
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Type(strings.Repeat("\n后面的内容", 32))
	tt.Frame()
	tt.Frame()
	root.doc.SetSelection(root.doc.Text().Len(), root.doc.Text().Len())
	root.doc.InsertText(strings.Repeat("\n", 40))
	tt.Frame()
	tt.Frame()
	root.doc.ScrollBy(-100000)
	tt.Frame()
	tt.Frame()
	leafOrigin := func() ui.Rect {
		editor, _ := tt.Find("Leaf 正文")
		parent, _ := tt.Find("笔记正文")
		for _, box := range root.doc.VisibleAttachments() {
			if leafID(box.Attachment) != "" {
				editor.Y = parent.Y + box.Rect.Y + 52
			}
		}
		return editor
	}
	editor := leafOrigin()
	var formulaY float32
	for _, box := range d.doc.VisibleAttachments() {
		if _, ok := mathSource(box.Attachment); ok {
			formulaY = editor.Y + box.Rect.Y
		}
	}
	root.doc.ScrollBy(formulaY - 300)
	tt.Frame()
	tt.Frame()
	editor = leafOrigin()
	for _, box := range d.doc.VisibleAttachments() {
		if _, ok := mathSource(box.Attachment); !ok {
			continue
		}
		y := editor.Y + box.Rect.Y
		tt.Press(editor.X+box.Rect.X+box.Rect.W/2, y+box.Rect.H/2)
		tt.Release(editor.X+box.Rect.X+box.Rect.W/2, y+box.Rect.H/2)
		if a.mathEdit == nil {
			t.Fatalf("did not open formula: editor=%+v formula=%+v scroll=%v", editor, box.Rect, root.doc.ScrollOffset())
		}
		panel, ok := tt.Find("编辑公式")
		if !ok || panel.Y < y+box.Rect.H-1 || panel.Y > y+box.Rect.H+20 {
			t.Fatalf("panel misplaced: formula y=%v height=%v panel=%+v", y, box.Rect.H, panel)
		}
		return
	}
	t.Fatal("formula not found")
}
