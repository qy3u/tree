package main

import (
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

// Tracks only the last body deletion that handed focus to the title. This
// lets an immediate undo restore the removed block rather than undo title text.
type titleBoundaryUndo struct {
	page, title string
	version     uint64
}

func (a *app) backspaceToTitle(d *draft, key ui.Key, mods ui.Modifiers) bool {
	if key != ui.KeyBackspace || mods != 0 {
		return false
	}
	anchor, caret := d.doc.Selection()
	if anchor != 0 || caret != 0 {
		return false
	}
	text := d.doc.Text()
	_, end := text.ParagraphRange(0)
	style := text.ParagraphStyleAt(0)
	style.ID = ""
	// Selections, text deletion, and unstyling list/code/heading blocks stay
	// with the editor. Only an empty, plain first block crosses this boundary.
	if end != 0 || style != (richtext.ParagraphStyle{}) {
		return false
	}
	a.titleBoundaryUndo = titleBoundaryUndo{}
	if text.ParagraphCount() > 1 {
		d.doc.Edit(func(e *richtext.Edit) {
			e.ReplaceString(0, 1, "", richtext.Attributes{})
			e.Select(0, 0)
		})
		a.changed(d)
		a.titleBoundaryUndo = titleBoundaryUndo{page: d.page.ID, title: d.page.Title, version: d.doc.Version()}
	}
	d.doc.ScrollBy(-d.doc.ScrollOffset())
	a.focusTitleEndPage = d.page.ID
	return true
}

func (a *app) undoTitleBoundary(d *draft, k ui.TextKey) bool {
	u := a.titleBoundaryUndo
	if k.Key != ui.KeyZ || k.Mods != ui.Cmd || u.page != d.page.ID || u.version != d.doc.Version() || u.title != k.Text() {
		return false
	}
	a.titleBoundaryUndo = titleBoundaryUndo{}
	d.doc.Undo()
	a.changed(d)
	a.editorHandle.Focus()
	return true
}
