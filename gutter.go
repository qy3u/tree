package main

import "github.com/egoist/mygo/treerichtext"

// Pointer movement chooses a hovered block; text/caret changes choose the
// editing block. No timer or save completion is involved in this decision.
type gutterInteraction struct {
	initialized       bool
	version           uint64
	anchor, caret     int
	x, y              float32
	over, followCaret bool
}

func (g *gutterInteraction) update(doc *richtext.Document, focused bool, x, y float32, over bool) int {
	anchor, caret := doc.Selection()
	version := doc.Version()
	moved := g.initialized && (x != g.x || y != g.y || over != g.over)
	edited := g.initialized && (version != g.version || anchor != g.anchor || caret != g.caret)
	if moved || !focused || anchor != caret {
		g.followCaret = false
	}
	if edited && focused && anchor == caret {
		g.followCaret = true
	}
	g.initialized = true
	g.version = version
	g.anchor = anchor
	g.caret = caret
	g.x = x
	g.y = y
	g.over = over
	if g.followCaret {
		start, _ := doc.Text().ParagraphRange(caret)
		return start
	}
	return -1
}
