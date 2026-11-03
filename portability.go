package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/egoist/mygo/treerichtext"
)

type noteBackup struct {
	Version int     `json:"version"`
	Pages   []*Page `json:"pages"`
}

func (s *Store) Backup(dir string) (string, error) {
	pages, err := s.List()
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "Tree-backup-*.tree.zip")
	if err != nil {
		return "", err
	}
	name := f.Name()
	success := false
	defer func() {
		f.Close()
		if !success {
			os.Remove(name)
		}
	}()
	zw := zip.NewWriter(f)
	w, err := zw.Create("notes.json")
	if err != nil {
		return "", err
	}
	if err = json.NewEncoder(w).Encode(noteBackup{1, pages}); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, "assets"))
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		r, e := os.Open(filepath.Join(s.dir, "assets", entry.Name()))
		if e != nil {
			return "", e
		}
		w, e := zw.Create("assets/" + entry.Name())
		if e == nil {
			_, e = io.Copy(w, r)
		}
		r.Close()
		if e != nil {
			return "", e
		}
	}
	if err = zw.Close(); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	success = true
	return name, nil
}

// Restore imports a backup as a new page tree, preserving the current library.
func (s *Store) RestoreBackup(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	files := map[string][]byte{}
	var total uint64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		total += f.UncompressedSize64
		if total > 256<<20 || len(files) > 10000 {
			return "", errors.New("备份内容过大")
		}
		if f.Name != "notes.json" && (!strings.HasPrefix(f.Name, "assets/") || filepath.Base(f.Name) != strings.TrimPrefix(f.Name, "assets/")) {
			return "", errors.New("备份包含无效路径")
		}
		if _, exists := files[f.Name]; exists {
			return "", errors.New("备份包含重复文件")
		}
		r, e := f.Open()
		if e != nil {
			return "", e
		}
		data, e := io.ReadAll(io.LimitReader(r, 257<<20))
		r.Close()
		if e != nil {
			return "", e
		}
		if len(data) > 256<<20 {
			return "", errors.New("备份内容过大")
		}
		files[f.Name] = data
	}
	var backup noteBackup
	if err = json.Unmarshal(files["notes.json"], &backup); err != nil {
		return "", err
	}
	if backup.Version != 1 {
		return "", errors.New("不支持此备份版本")
	}
	byID := map[string]*Page{}
	ids := map[string]string{}
	for _, p := range backup.Pages {
		if p == nil || p.ID == "" || byID[p.ID] != nil || p.Body.Schema != 1 {
			return "", errors.New("备份页面数据无效")
		}
		byID[p.ID] = p
		ids[p.ID] = newID()
	}
	for _, p := range backup.Pages {
		seen := map[string]bool{p.ID: true}
		for parent := p.Parent; parent != ""; {
			q := byID[parent]
			if q == nil || seen[parent] {
				return "", errors.New("备份页面层级无效")
			}
			seen[parent] = true
			parent = q.Parent
		}
		if p.Deleted != "" && byID[p.Deleted] == nil {
			return "", errors.New("备份回收站数据无效")
		}
	}
	root := newID()
	assets := map[string]string{}
	var created []string
	success := false
	defer func() {
		if !success {
			for _, name := range created {
				os.Remove(name)
			}
		}
	}()
	for name, data := range files {
		if name == "notes.json" {
			continue
		}
		old := strings.TrimPrefix(name, "assets/")
		next := newID() + filepath.Ext(old)
		dest := filepath.Join(s.dir, "assets", next)
		if err = os.WriteFile(dest, data, 0600); err != nil {
			return "", err
		}
		created = append(created, dest)
		assets[old] = next
	}
	var stmts []sqlite.Statement
	insert := func(p *Page) error {
		b, e := json.Marshal(p.Body)
		if e != nil {
			return e
		}
		stmts = append(stmts, sqlite.Statement{SQL: `INSERT INTO pages(id,parent,title,position,body,revision,updated,deleted) VALUES(?,?,?,?,?,1,?,?)`, Args: []any{p.ID, p.Parent, p.Title, p.Position, string(b), time.Now().UTC().Format(time.RFC3339Nano), p.Deleted}})
		return nil
	}
	if err = insert(&Page{ID: root, Title: "恢复的笔记 · " + time.Now().Format("2006-01-02 15:04"), Body: emptyBody()}); err != nil {
		return "", err
	}
	for _, p := range backup.Pages {
		var paras []richtext.Paragraph
		for _, para := range p.Body.Text.Paragraphs() {
			for i, r := range para.Runs {
				if at := r.Attributes.Attachment; at != nil && strings.HasPrefix(at.URL, "asset:") {
					old := strings.TrimPrefix(at.URL, "asset:")
					next, ok := assets[old]
					if !ok {
						return "", fmt.Errorf("备份缺少附件 %s", old)
					}
					copy := *at
					copy.URL = "asset:" + next
					para.Runs[i].Attributes.Attachment = &copy
				}
			}
			paras = append(paras, para)
		}
		p.Body.Text = richtext.TextOf(paras...)
		p.ID = ids[p.ID]
		if p.Parent == "" {
			p.Parent = root
		} else {
			p.Parent = ids[p.Parent]
		}
		if p.Deleted != "" {
			p.Deleted = ids[p.Deleted]
		}
		if err = insert(p); err != nil {
			return "", err
		}
	}
	for old, next := range assets {
		stmts = append(stmts, sqlite.Statement{SQL: `INSERT INTO assets(id,name,path) VALUES(?,?,?)`, Args: []any{next, old, next}})
	}
	s.mu.Lock()
	_, err = s.db.Transaction(context.Background(), stmts)
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	success = true
	return root, nil
}
func (s *Store) ImportMarkdown(path string) (*Page, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 16<<20 {
		return nil, errors.New("Markdown 文件大于 16 MB")
	}
	body := bodyFromMarkdown(string(data))
	var paras []richtext.Paragraph
	base, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	for _, para := range body.Text.Paragraphs() {
		for i, r := range para.Runs {
			at := r.Attributes.Attachment
			if at == nil || strings.Contains(at.URL, ":") || filepath.IsAbs(at.URL) {
				continue
			}
			file, e := filepath.EvalSymlinks(filepath.Join(base, at.URL))
			if e != nil {
				continue
			}
			rel, e := filepath.Rel(base, file)
			if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			img, e := os.ReadFile(file)
			if e != nil {
				continue
			}
			copy, e := s.ImportImage(img, filepath.Base(file))
			if e != nil {
				return nil, e
			}
			para.Runs[i].Attributes.Attachment = copy
		}
		paras = append(paras, para)
	}
	body.Text = richtext.TextOf(paras...)
	return s.Create("", strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), body)
}
func (a *app) importMarkdown() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "导入 Markdown", Filters: []mygo.FileFilter{{Name: "Markdown", Extensions: []string{"md", "markdown"}}}})
	if err != nil {
		a.report(err)
		return
	}
	if len(paths) == 0 {
		return
	}
	if err = a.flush(); err != nil {
		a.report(err)
		return
	}
	p, err := a.store.ImportMarkdown(paths[0])
	if err != nil {
		a.report(err)
		return
	}
	a.report(a.reload())
	a.selectPage(p.ID)
}
func (a *app) backupLibrary() {
	if err := a.flush(); err != nil {
		a.report(err)
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择备份保存位置", Directory: true})
	if err != nil {
		a.report(err)
		return
	}
	if len(paths) == 0 {
		return
	}
	path, err := a.store.Backup(paths[0])
	if err != nil {
		a.report(err)
		return
	}
	a.status = "备份已保存：" + path
}
func (a *app) restoreLibrary() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "从备份导入（保留现有笔记）", Filters: []mygo.FileFilter{{Name: "Tree 备份", Extensions: []string{"zip"}}}})
	if err != nil {
		a.report(err)
		return
	}
	if len(paths) == 0 {
		return
	}
	if err = a.flush(); err != nil {
		a.report(err)
		return
	}
	id, err := a.store.RestoreBackup(paths[0])
	if err != nil {
		a.report(err)
		return
	}
	a.report(a.reload())
	a.selectPage(id)
}
