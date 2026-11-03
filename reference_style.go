package main

import (
	"strings"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

const referenceTag = "tree-reference"
const referenceIconTag = "tree-reference-icon"

func referenceIcon(block string) *richtext.Attachment {
	kind := "page"
	if block != "" {
		kind = "paragraph"
	}
	return &richtext.Attachment{URL: "tree-reference-icon:" + kind, Decorative: true, Width: 22, Height: 16, Draw: func(p *ui.Painter, r ui.Rect) {
		p.Icon(icons[kind], ui.Rect{X: r.X, Y: r.Y + 1, W: 16, H: 16}, ui.Hex("#91918e"))
	}}
}

func referenceSpanStyle(tag string, s *ui.Span) {
	switch tag {
	case referenceTag:
		s.Color = ink
		s.Weight = 600
		s.Underline = true
		s.DecorationColor = ui.Hex("#d3d2ce")
		s.DecorationThickness = 1
	case referenceIconTag:
		s.Underline = false
	}
}

// Upgrade old plain links and rehydrate decorative icons on load/rich paste.
// Keep paragraph IDs, link targets and user-authored text formatting intact.
func styleReferences(text richtext.Text) (richtext.Text, bool) {
	var paras []richtext.Paragraph
	changed := false
	for _, para := range text.Paragraphs() {
		var runs []richtext.Run
		previous := ""
		for _, run := range para.Runs {
			_, block, ok := parseReference(run.Attributes.Link)
			if !ok {
				// Ordinary headings must not retain the underline inherited
				// from legacy references. Actual linked text stays linked.
				if para.Style.Heading > 0 && run.Attributes.Link == "" && run.Attributes.Underline {
					run.Attributes.Underline = false
					changed = true
				}
				runs = append(runs, run)
				previous = ""
				continue
			}
			if run.Attributes.Tag == referenceIconTag && run.Attributes.Attachment != nil {
				run.Attributes.Attachment = referenceIcon(block)
				runs = append(runs, run)
				previous = run.Attributes.Link
				continue
			}
			// Attachments other than our icon (e.g. linked images) remain images.
			if run.Attributes.Attachment != nil {
				runs = append(runs, run)
				previous = ""
				continue
			}
			if run.Attributes.Tag != referenceTag {
				changed = true
				if block != "" {
					if _, label, found := strings.Cut(run.Text, " › "); found {
						run.Text = label
					}
				}
			}
			if previous != run.Attributes.Link {
				runs = append(runs, richtext.Run{Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Link: run.Attributes.Link, Tag: referenceIconTag, Attachment: referenceIcon(block)}})
				changed = true
			}
			run.Attributes.Tag = referenceTag
			if run.Attributes.Underline {
				run.Attributes.Underline = false
				changed = true
			}
			runs = append(runs, run)
			previous = run.Attributes.Link
		}
		para.Runs = runs
		paras = append(paras, para)
	}
	return richtext.TextOf(paras...), changed
}

func pasteReferenceStyles(text richtext.Text) richtext.Text {
	styled, _ := styleReferences(text)
	return styled
}
