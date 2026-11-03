package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func (s *Store) ImportImage(data []byte, name string) (*richtext.Attachment, error) {
	if len(data) > 30<<20 {
		return nil, errors.New("图片大于 30 MB")
	}
	img, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".webp" {
		ext = ".png"
	}
	id := newID() + ext
	dest := filepath.Join(s.dir, "assets", id)
	if err = os.WriteFile(dest, data, 0600); err != nil {
		return nil, err
	}
	_, err = s.db.Execute(context.Background(), `INSERT INTO assets(id,name,path) VALUES(?,?,?)`, id, name, id)
	if err != nil {
		os.Remove(dest)
		return nil, err
	}
	w, h := ui.ImageSize(img)
	scale := float32(1)
	if w > 560 {
		scale = 560 / w
	}
	return &richtext.Attachment{Image: img, Name: name, URL: "asset:" + id, Width: w * scale, Height: h * scale}, nil
}
func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_' || r == ' ' {
			return r
		}
		return '-'
	}, s)
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 60 {
		s = string(r[:60])
	}
	if s == "" {
		s = "未命名"
	}
	return s
}
func (s *Store) Export(rootID, dir string) (string, error) {
	pages, err := s.List()
	if err != nil {
		return "", err
	}
	byID := map[string]*Page{}
	for _, p := range pages {
		byID[p.ID] = p
	}
	root := byID[rootID]
	if root == nil || root.Deleted != "" {
		return "", errors.New("页面不存在")
	}
	selected := map[string]bool{rootID: true}
	for changed := true; changed; {
		changed = false
		for _, p := range pages {
			if p.Deleted == "" && selected[p.Parent] && !selected[p.ID] {
				selected[p.ID] = true
				changed = true
			}
		}
	}
	out, err := os.MkdirTemp(dir, safeName(root.Title)+"-export-")
	if err != nil {
		return "", err
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(out)
		}
	}()
	names := map[string]string{}
	for _, p := range pages {
		if selected[p.ID] {
			names[p.ID] = safeName(p.Title) + "-" + p.ID[:8] + ".md"
		}
	}
	for _, p := range pages {
		if !selected[p.ID] {
			continue
		}
		md := "# " + strings.ReplaceAll(p.Title, "\n", " ") + "\n\n" + p.Body.Markdown() + "\n"
		for _, child := range sortedChildren(pages, p.ID) {
			md += fmt.Sprintf("\n[%s](%s)\n", strings.ReplaceAll(child.Title, "]", "\\]"), strings.ReplaceAll(names[child.ID], " ", "%20"))
		}
		if err = os.WriteFile(filepath.Join(out, names[p.ID]), []byte(md), 0600); err != nil {
			return "", err
		}
		for _, run := range p.Body.Text.Runs(0, p.Body.Text.Len()) {
			at := run.Attributes.Attachment
			if at == nil || !strings.HasPrefix(at.URL, "asset:") {
				continue
			}
			id := strings.TrimPrefix(at.URL, "asset:")
			if filepath.Base(id) != id {
				return "", errors.New("图片路径无效")
			}
			data, e := os.ReadFile(filepath.Join(s.dir, "assets", id))
			if e != nil {
				return "", e
			}
			if e = os.MkdirAll(filepath.Join(out, "assets"), 0700); e != nil {
				return "", e
			}
			if e = os.WriteFile(filepath.Join(out, "assets", id), data, 0600); e != nil {
				return "", e
			}
		}
	}
	success = true
	return out, nil
}
