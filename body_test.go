package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/treerichtext"
)

func TestTableJSONMarkdownAndUndo(t *testing.T) {
	b := emptyBody()
	id := newID()
	table := Table{Cells: [][]string{{"名称", "说明"}, {"中文", "a|b\n第二行"}}}
	b.Tables[id] = table
	d := richtext.NewDocument(b.Text)
	d.InsertAttachment(tableAttachment(id, table))
	d.InsertText("\n")
	d.InsertAttachment(dividerAttachment())
	b.Text = d.Text()
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var restored Body
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Tables[id], table) {
		t.Fatal("lost table rows")
	}
	md := restored.Markdown()
	if !strings.Contains(md, "| 名称 | 说明 |") || !strings.Contains(md, "a\\|b<br>第二行") || !strings.Contains(md, "---") {
		t.Fatalf("markdown: %s", md)
	}
	text := hydrate(restored, t.TempDir())
	for _, r := range text.Runs(0, text.Len()) {
		if r.Attributes.Attachment != nil && r.Attributes.Attachment.Draw == nil {
			t.Fatal("attachment drawing not restored")
		}
	}
}
func TestParagraphMovePreservesIDsAndUndo(t *testing.T) {
	d := richtext.NewDocument(richtext.ParseMarkdown("## 第一段\n\n- 第二段\n\n第三段"))
	d.IdentifyParagraphs(newID)
	before := d.Text()
	var ids []string
	for _, p := range before.Paragraphs() {
		ids = append(ids, p.Style.ID)
	}
	if !moveParagraph(d, 0, 2) {
		t.Fatal("move failed")
	}
	var after []string
	for _, p := range d.Text().Paragraphs() {
		after = append(after, p.Style.ID)
	}
	if !reflect.DeepEqual(after, []string{ids[1], ids[2], ids[0]}) {
		t.Fatalf("IDs: %v -> %v", ids, after)
	}
	d.Undo()
	if d.Text().Markdown() != before.Markdown() {
		t.Fatalf("undo changed contents: %s", d.Text().Markdown())
	}
}
func TestImageSaveRehydrateExport(t *testing.T) {
	s := testStore(t)
	img := image.NewRGBA(image.Rect(0, 0, 16, 8))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var raw bytes.Buffer
	png.Encode(&raw, img)
	at, err := s.ImportImage(raw.Bytes(), "图片.png")
	if err != nil {
		t.Fatal(err)
	}
	p := createPage(t, s, "", "图片页")
	d := richtext.NewDocument(p.Body.Text)
	d.InsertAttachment(at)
	p.Body.Text = d.Text()
	if err = s.Save(p); err != nil {
		t.Fatal(err)
	}
	pages, _ := s.List()
	h := hydrate(pages[0].Body, s.dir)
	for _, r := range h.Runs(0, h.Len()) {
		if r.Attributes.Attachment.Image == nil {
			t.Fatal("image not loaded")
		}
	}
	out, err := s.Export(p.ID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	asset := strings.TrimPrefix(at.URL, "asset:")
	copied, err := os.ReadFile(filepath.Join(out, "assets", asset))
	if err != nil || !bytes.Equal(copied, raw.Bytes()) {
		t.Fatal("export image mismatch", err)
	}
	if !strings.Contains(p.Body.Markdown(), "assets/"+asset) {
		t.Fatal("export URL not rewritten")
	}
}
func TestSearchOnlyActiveTableVersions(t *testing.T) {
	b := emptyBody()
	b.Tables["old"] = Table{Cells: [][]string{{"removed"}}}
	if strings.Contains(b.PlainText(), "removed") {
		t.Fatal("search includes undo history")
	}
}
