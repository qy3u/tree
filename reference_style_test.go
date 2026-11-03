package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func TestReferenceStyleRichPasteExportAndReopen(t *testing.T) {
	a, tt := commandTestApp(t)
	target := a.current()
	target.page.Title = "页面示例"
	tt.Type("aa")
	blockLink, err := a.copyBlockLink(target, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ link, label, icon string }{
		{blockLink, "aa", "paragraph"}, {referenceURL(target.page.ID, ""), "页面示例", "page"},
	} {
		text, ok := a.pasteReference(tc.link)
		if !ok {
			t.Fatal("not resolved")
		}
		if text.PlainText() != tc.label {
			t.Fatalf("label %q", text.PlainText())
		}
		at := text.AttributesAt(0).Attachment
		if at == nil || at.URL != "tree-reference-icon:"+tc.icon || at.Draw == nil || !at.Decorative {
			t.Fatal("missing inline icon")
		}
		span := ui.Span{}
		referenceSpanStyle(text.AttributesAt(1).Tag, &span)
		if span.Color != ink || span.Weight != 600 || !span.Underline || span.DecorationColor != ui.Hex("#d3d2ce") {
			t.Fatal("wrong reference typography")
		}
		md, html := text.Markdown(), text.HTML()
		if strings.Contains(md, "tree-reference-icon:") || strings.Contains(html, "<img") || !strings.Contains(md, tc.link) || !strings.Contains(html, tc.label) {
			t.Fatalf("decoration leaked to export: %q / %q", md, html)
		}
		// Simulate rich clipboard coming from a different process (no Draw closure).
		data, err := json.Marshal(text)
		if err != nil {
			t.Fatal(err)
		}
		a.addPage("")
		tt.Frame()
		tt.Click("笔记正文")
		tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(richtext.Format, data))))
		tt.Key(ui.Cmd, ui.KeyV)
		d := a.current()
		if d.doc.Text().AttributesAt(0).Attachment.Draw == nil || d.doc.Text().Len() != text.Len() {
			t.Fatal("rich paste lost icon or duplicated it")
		}
		if err := a.flush(); err != nil {
			t.Fatal(err)
		}
		reopened, err := newApp(a.store)
		if err != nil {
			t.Fatal(err)
		}
		got := reopened.current().doc.Text()
		if got.AttributesAt(0).Attachment.Draw == nil || got.PlainText() != tc.label {
			t.Fatal("style lost after reopening")
		}
	}
}

func TestLegacyReferenceStyleKeepsTargets(t *testing.T) {
	a, _ := commandTestApp(t)
	destination := a.current()
	legacy := richtext.TextOf(
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "before"}, Runs: []richtext.Run{{Text: "旧标题 › aa", Attributes: richtext.Attributes{Link: referenceURL(destination.page.ID, "before"), Underline: true}}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "target"}, Runs: []richtext.Run{{Text: "定位这里"}}},
	)
	destination.doc.SetText(legacy)
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	page := destination.page.ID
	a.addPage("")
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	reopened.openReference(referenceURL(page, "target"))
	if reopened.errorOpen {
		t.Fatal(reopened.failure)
	}
	doc := reopened.current().doc
	if doc.Text().PlainText() != "aa\n定位这里" {
		t.Fatalf("legacy labels: %q", doc.Text().PlainText())
	}
	pos, ok := blockPosition(doc.Text(), "target")
	if !ok {
		t.Fatal("target ID changed")
	}
	if caret, _ := doc.Selection(); caret != pos {
		t.Fatal("jump used offset before restyling")
	}
	again, changed := styleReferences(doc.Text())
	if changed || again.String() != doc.Text().String() {
		t.Fatal("restyling is not idempotent")
	}
}
