package main

import (
	"testing"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func TestReferenceUnderlineDoesNotExtendToHeading(t *testing.T) {
	a, tt := commandTestApp(t)
	d := a.current()
	// Simulate legacy explicit underline as well as normal hyperlink rendering.
	d.doc.SetText(richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: "引用", Attributes: richtext.Attributes{Link: referenceURL(d.page.ID, ""), Underline: true}}}}))
	tt.Frame()
	a.editorHandle.Focus()
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Type("/h1")
	tt.Key(0, ui.KeyEnter)
	tt.Type("标题内容")
	text := d.doc.Text()
	if text.ParagraphStyleAt(3).Heading != 1 {
		t.Fatal("heading command not applied")
	}
	for _, r := range text.Runs(3, text.Len()) {
		if r.Attributes.Underline || r.Attributes.Link != "" || r.Attributes.Tag != "" {
			t.Fatal("reference format leaked into heading")
		}
	}
	// A deliberately underlined ordinary paragraph still keeps its formatting.
	d.doc.SetText(richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: "正文", Attributes: richtext.Attributes{Underline: true}}}}))
	d.doc.SetSelection(2, 2)
	tt.Frame()
	tt.Type("继续")
	if !d.doc.Text().AttributesAt(2).Underline {
		t.Fatal("ordinary underline no longer extends")
	}
}

func TestLegacyHeadingUnderlineRepairAndPersistence(t *testing.T) {
	a, _ := commandTestApp(t)
	d := a.current()
	link := referenceURL(d.page.ID, "")
	d.doc.SetText(richtext.TextOf(
		richtext.Paragraph{Style: richtext.ParagraphStyle{Heading: 1, ID: "heading"}, Runs: []richtext.Run{{Text: "标题", Attributes: richtext.Attributes{Underline: true}}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{Heading: 3}, Runs: []richtext.Run{{Text: "网页链接", Attributes: richtext.Attributes{Link: "https://example.com", Underline: true}}}},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "页面引用", Attributes: richtext.Attributes{Link: link, Tag: referenceTag, Underline: true}}}},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "正文", Attributes: richtext.Attributes{Underline: true}}}},
	))
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	b, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	repaired := b.current().doc.Text()
	if repaired.AttributesAt(0).Underline || repaired.ParagraphStyleAt(0).ID != "heading" {
		t.Fatal("heading not repaired or ID changed")
	}
	for _, r := range repaired.Runs(0, repaired.Len()) {
		if r.Attributes.Link == link && r.Attributes.Underline {
			t.Fatal("reference underline still persisted")
		}
		if r.Text == "网页链接" && (!r.Attributes.Underline || r.Attributes.Link == "") {
			t.Fatal("actual heading hyperlink lost")
		}
		if r.Text == "正文" && !r.Attributes.Underline {
			t.Fatal("ordinary text underline lost")
		}
	}
	if err := b.flush(); err != nil {
		t.Fatal(err)
	}
	c, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	if c.current().doc.Text().AttributesAt(0).Underline {
		t.Fatal("repair not saved")
	}
}
