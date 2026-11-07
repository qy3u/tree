package richtext

import "github.com/egoist/mygo/ui"

// LinkHit describes a contiguous link and the visual line under the pointer.
// Rect uses editor-local coordinates; Start and End are document rune offsets.
type LinkHit struct {
	URL        string
	Start, End int
	Rect       ui.Rect
}

// LinkAt hit-tests actual linked text, excluding the whitespace around it.
// Call on the UI thread after building the editor.
func (d *Document) LinkAt(x, y float32) (LinkHit, bool) {
	d.lock()
	v := d.shownIn.Value()
	d.mu.Unlock()
	if v == nil || len(v.paras) == 0 || v.dragging {
		return LinkHit{}, false
	}
	v.sync()
	cx, cy := v.contentPoint(x, y)
	p, _, _ := v.layoutPositionAt(cx, cy)
	if p < 0 {
		return LinkHit{}, false
	}
	pv := v.paraLayout(p)
	ox, oy := v.origin(p)
	start := v.shown.paraStart(p)
	end := start + v.para(p).n
	for i := start; i < end; {
		attrs := v.shown.AttributesAt(i)
		if attrs.Link == "" || attrs.Attachment != nil {
			i++
			continue
		}
		z := i + 1
		for z < end && v.shown.AttributesAt(z).Link == attrs.Link && v.shown.AttributesAt(z).Attachment == nil {
			z++
		}
		for _, r := range pv.layout.SelectionRects(ui.TextInputRange{Start: pv.units[v.local(p, i)], End: pv.units[v.local(p, z)]}) {
			r.X += v.padX + ox
			r.Y += v.padY + float32(oy-v.scroll)
			if x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
				return LinkHit{URL: attrs.Link, Start: i, End: z, Rect: r}, true
			}
		}
		i = z
	}
	return LinkHit{}, false
}
