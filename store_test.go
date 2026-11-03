package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func createPage(t *testing.T, s *Store, parent, title string) *Page {
	t.Helper()
	p, err := s.Create(parent, title, emptyBody())
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestStoreReopenAndConflict(t *testing.T) {
	dir := t.TempDir()
	s, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := createPage(t, s, "", "中文标题")
	stale := *p
	p.Body = bodyFromMarkdown("## 标题\n\n**加粗** 和中文正文")
	if err = s.Save(p); err != nil {
		t.Fatal(err)
	}
	if err = s.Save(&stale); !errors.Is(err, errConflict) {
		t.Fatalf("stale save = %v", err)
	}
	s.Close()
	s, err = openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	pages, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Title != "中文标题" || !strings.Contains(pages[0].Body.Markdown(), "**加粗**") {
		t.Fatalf("reopened: %#v", pages)
	}
}
func TestMoveCycleAndTrashRestore(t *testing.T) {
	s := testStore(t)
	root := createPage(t, s, "", "父")
	child := createPage(t, s, root.ID, "子")
	leaf := createPage(t, s, child.ID, "孙")
	if err := s.Move(root.ID, leaf.ID); err == nil {
		t.Fatal("cycle allowed")
	}
	if err := s.Move(root.ID, root.ID); err == nil {
		t.Fatal("self move allowed")
	}
	if err := s.Trash(leaf.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(root.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(root.ID); err != nil {
		t.Fatal(err)
	}
	pages, _ := s.List()
	for _, p := range pages {
		if p.ID == leaf.ID && p.Deleted == "" {
			t.Fatal("previously deleted page restored")
		}
		if p.ID != leaf.ID && p.Deleted != "" {
			t.Fatal("subtree not restored")
		}
	}
	if err := s.Trash(root.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(leaf.ID); err != nil {
		t.Fatal(err)
	}
	pages, _ = s.List()
	for _, p := range pages {
		if p.ID == leaf.ID && (p.Parent != "" || p.Deleted != "") {
			t.Fatal("restore under deleted parent must become root")
		}
	}
}
func TestReorderAndSafeExport(t *testing.T) {
	s := testStore(t)
	a := createPage(t, s, "", "A/../../名字")
	b := createPage(t, s, "", "B")
	child := createPage(t, s, a.ID, "子页面")
	if err := s.Reorder(b.ID, -1); err != nil {
		t.Fatal(err)
	}
	pages, _ := s.List()
	if sortedChildren(pages, "")[0].ID != b.ID {
		t.Fatal("wrong order")
	}
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.md")
	os.WriteFile(existing, []byte("keep"), 0600)
	out, err := s.Export(a.ID, dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(out) != dir {
		t.Fatal("escaped export directory")
	}
	files, _ := os.ReadDir(out)
	if len(files) != 2 {
		t.Fatalf("files: %v", files)
	}
	data, _ := os.ReadFile(filepath.Join(out, safeName(a.Title)+"-"+a.ID[:8]+".md"))
	if !strings.Contains(string(data), child.ID[:8]) {
		t.Fatal("missing child link")
	}
	data, _ = os.ReadFile(existing)
	if string(data) != "keep" {
		t.Fatal("overwrote existing file")
	}
}
func TestFutureSchemaRejected(t *testing.T) {
	dir := t.TempDir()
	s, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.db.Execute(context.Background(), "PRAGMA user_version=99")
	s.Close()
	s, err = openStore(dir)
	if err == nil {
		s.Close()
		t.Fatal("future schema accepted")
	}
}
