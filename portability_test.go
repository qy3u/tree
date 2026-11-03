package main

import (
	"archive/zip"
	"bytes"
	"github.com/egoist/mygo/treerichtext"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupRestorePreservesExistingLibrary(t *testing.T) {
	s := testStore(t)
	root := createPage(t, s, "", "父")
	child := createPage(t, s, root.ID, "子")
	child.Body = bodyFromMarkdown("## 标题\n\n|甲|乙|\n|---|---|\n|一|二|")
	if err := s.Save(child); err != nil {
		t.Fatal(err)
	}
	backup, err := s.Backup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.RestoreBackup(backup)
	if err != nil {
		t.Fatal(err)
	}
	pages, _ := s.List()
	if len(pages) != 5 {
		t.Fatalf("page count %d", len(pages))
	}
	restored := sortedChildren(pages, id)
	if len(restored) != 1 || restored[0].Title != "父" {
		t.Fatal("root missing")
	}
	kids := sortedChildren(pages, restored[0].ID)
	if len(kids) != 1 || !strings.Contains(kids[0].Body.Markdown(), "| 一 | 二 |") {
		t.Fatal("table or hierarchy lost")
	}
	for _, p := range pages {
		if p.ID == child.ID && p.Revision != child.Revision {
			t.Fatal("existing data was changed")
		}
	}
}
func TestBackupRejectsUnsafeArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.zip")
	f, _ := os.Create(path)
	w := zip.NewWriter(f)
	entry, _ := w.Create("../outside")
	entry.Write([]byte("bad"))
	w.Close()
	f.Close()
	s := testStore(t)
	if _, err := s.RestoreBackup(path); err == nil {
		t.Fatal("unsafe path accepted")
	}
	pages, _ := s.List()
	if len(pages) != 0 {
		t.Fatal("failed import changed library")
	}
}
func TestMarkdownImportTablesAndCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "学习.md")
	os.WriteFile(path, []byte("# 学习\n\n|任务|状态|\n|---|---|\n|阅读|完成|\n\n```\n|a|b|\n|---|---|\n```"), 0600)
	p, err := testStore(t).ImportMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "学习" || len(p.Body.Tables) != 1 || !strings.Contains(p.Body.Markdown(), "| 阅读 | 完成 |") {
		t.Fatal("Markdown structures not imported")
	}
}

func TestBackupRestoresImageData(t *testing.T) {
	s := testStore(t)
	p := createPage(t, s, "", "图片")
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	at, err := s.ImportImage(buf.Bytes(), "test.png")
	if err != nil {
		t.Fatal(err)
	}
	p.Body.Text = richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: at}}}})
	if err = s.Save(p); err != nil {
		t.Fatal(err)
	}
	path, err := s.Backup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dest := testStore(t)
	root, err := dest.RestoreBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	pages, _ := dest.List()
	restored := sortedChildren(pages, root)[0]
	for _, r := range hydrate(restored.Body, dest.dir).Runs(0, 1) {
		if r.Attributes.Attachment == nil || r.Attributes.Attachment.Image == nil {
			t.Fatal("restored image cannot render")
		}
	}
}
