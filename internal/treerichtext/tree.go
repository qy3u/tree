package richtext

import "github.com/egoist/mygo/ui"

// ScrollOffset reports the native editor's current scroll, for page chrome.
func (d *Document) ScrollOffset() float32 {
	d.lock()
	v := d.shownIn.Value()
	d.mu.Unlock()
	if v == nil {
		return 0
	}
	return float32(v.scroll)
}

// AttachmentBox locates an attachment in editor-local coordinates.
type AttachmentBox struct {
	Start      int
	Rect       ui.Rect
	Attachment *Attachment
}

func (d *Document) VisibleAttachments() []AttachmentBox {
	d.VisibleParagraphs()
	d.lock()
	v := d.shownIn.Value()
	d.mu.Unlock()
	if v == nil {
		return nil
	}
	var out []AttachmentBox
	for i := v.first; i <= v.last && i < len(v.paras); i++ {
		pv := v.paraLayout(i)
		x, y := v.origin(i)
		for _, o := range pv.layout.Objects() {
			start := v.global(i, pv.runeOfUnit(o.Index))
			at := v.shown.AttributesAt(start).Attachment
			if at == nil {
				continue
			}
			r := o.Rect
			r.X += v.padX + x
			r.Y += v.padY + float32(y-v.scroll)
			out = append(out, AttachmentBox{start, r, at})
		}
	}
	return out
}

// ScrollBy lets title and attachment controls share the editor's scroll view.
func (d *Document) ScrollBy(dy float32) bool {
	d.lock()
	v := d.shownIn.Value()
	d.mu.Unlock()
	if v == nil {
		return false
	}
	before := v.scroll
	v.setScroll(max(0, min(v.scroll+float64(dy), v.hs.top(len(v.paras))-float64(v.viewH))))
	if before != v.scroll {
		v.services.Invalidate()
		return true
	}
	return false
}
