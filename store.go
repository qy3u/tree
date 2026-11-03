package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

var errConflict = errors.New("页面已被更新，请重新打开页面后重试")

type Page struct {
	ID, Parent, Title string
	Position          int64
	Body              Body
	Revision          int64
	Updated           string
	Deleted           string
}

type Store struct {
	mu  sync.Mutex
	db  *sqlite.DB
	dir string
}

func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0700); err != nil {
		return nil, err
	}
	db, err := sqlite.Open(context.Background(), filepath.Join(dir, "notes.db"), sqlite.OpenOptions{})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, dir: dir}
	version, err := db.Query(context.Background(), "PRAGMA user_version")
	if err != nil {
		db.Close()
		return nil, err
	}
	if len(version.Values) > 0 && version.Values[0][0].(int64) > 1 {
		db.Close()
		return nil, errors.New("数据库来自较新的应用版本，请使用新版应用打开")
	}
	for _, q := range []string{
		"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON",
		`CREATE TABLE IF NOT EXISTS pages(id TEXT PRIMARY KEY,parent TEXT NOT NULL DEFAULT '',title TEXT NOT NULL,position INTEGER NOT NULL,body TEXT NOT NULL,revision INTEGER NOT NULL DEFAULT 1,updated TEXT NOT NULL,deleted TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS assets(id TEXT PRIMARY KEY,name TEXT NOT NULL,path TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS page_parent ON pages(parent,position)`,
		"PRAGMA user_version=1",
	} {
		if _, err = db.Execute(context.Background(), q); err != nil {
			db.Close()
			return nil, err
		}
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func newID() string           { return strings.ToLower(rand.Text()) }
func (s *Store) list() ([]*Page, error) {
	rows, err := s.db.Query(context.Background(), `SELECT id,parent,title,position,body,revision,updated,deleted FROM pages ORDER BY position,id`)
	if err != nil {
		return nil, err
	}
	out := make([]*Page, 0, len(rows.Values))
	for _, r := range rows.Values {
		p := &Page{ID: r[0].(string), Parent: r[1].(string), Title: r[2].(string), Position: r[3].(int64), Revision: r[5].(int64), Updated: r[6].(string), Deleted: r[7].(string)}
		if err = json.Unmarshal([]byte(r[4].(string)), &p.Body); err != nil {
			return nil, fmt.Errorf("读取页面 %s: %w", p.Title, err)
		}
		if p.Body.Schema != 1 {
			return nil, fmt.Errorf("页面 %s 使用不支持的正文版本 %d", p.Title, p.Body.Schema)
		}
		if p.Body.Tables == nil {
			p.Body.Tables = map[string]Table{}
		}
		out = append(out, p)
	}
	return out, nil
}
func (s *Store) List() ([]*Page, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.list() }
func (s *Store) Create(parent, title string, body Body) (*Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pages, err := s.list()
	if err != nil {
		return nil, err
	}
	valid := parent == ""
	var pos int64
	for _, p := range pages {
		if p.ID == parent && p.Deleted == "" {
			valid = true
		}
		if p.Parent == parent {
			pos = max(pos, p.Position+1)
		}
	}
	if !valid {
		return nil, errors.New("父页面不存在")
	}
	if strings.TrimSpace(title) == "" {
		title = ""
	}
	p := &Page{ID: newID(), Parent: parent, Title: title, Position: pos, Body: body, Revision: 1, Updated: time.Now().UTC().Format(time.RFC3339Nano)}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Execute(context.Background(), `INSERT INTO pages(id,parent,title,position,body,updated) VALUES(?,?,?,?,?,?)`, p.ID, parent, title, pos, string(b), p.Updated)
	return p, err
}
func (s *Store) Save(p *Page) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(p.Body)
	if err != nil {
		return err
	}
	updated := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.Execute(context.Background(), `UPDATE pages SET title=?,body=?,revision=revision+1,updated=? WHERE id=? AND revision=? AND deleted=''`, p.Title, string(b), updated, p.ID, p.Revision)
	if err != nil {
		return err
	}
	if result.Changes != 1 {
		return errConflict
	}
	p.Revision++
	p.Updated = updated
	return nil
}
func (s *Store) Move(id, parent string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pages, err := s.list()
	if err != nil {
		return err
	}
	byID := map[string]*Page{}
	var pos int64
	for _, p := range pages {
		byID[p.ID] = p
		if p.Parent == parent {
			pos = max(pos, p.Position+1)
		}
	}
	if byID[id] == nil || byID[id].Deleted != "" {
		return errors.New("页面不存在")
	}
	seen := map[string]bool{}
	for at := parent; at != ""; {
		if at == id || seen[at] {
			return errors.New("不能将页面移到自身或自己的子页面内")
		}
		seen[at] = true
		p := byID[at]
		if p == nil || p.Deleted != "" {
			return errors.New("父页面不存在")
		}
		at = p.Parent
	}
	_, err = s.db.Execute(context.Background(), `UPDATE pages SET parent=?,position=?,revision=revision+1 WHERE id=?`, parent, pos, id)
	return err
}
func (s *Store) Reorder(id string, delta int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pages, err := s.list()
	if err != nil {
		return err
	}
	var current *Page
	for _, p := range pages {
		if p.ID == id {
			current = p
		}
	}
	if current == nil {
		return errors.New("页面不存在")
	}
	var siblings []*Page
	for _, p := range pages {
		if p.Parent == current.Parent && p.Deleted == "" {
			siblings = append(siblings, p)
		}
	}
	for i, p := range siblings {
		if p.ID == id {
			j := i + delta
			if j < 0 || j >= len(siblings) {
				return nil
			}
			siblings[i], siblings[j] = siblings[j], siblings[i]
			break
		}
	}
	var stmts []sqlite.Statement
	for i, p := range siblings {
		stmts = append(stmts, sqlite.Statement{SQL: `UPDATE pages SET position=?,revision=revision+1 WHERE id=?`, Args: []any{int64(i), p.ID}})
	}
	_, err = s.db.Transaction(context.Background(), stmts)
	return err
}
func (s *Store) Trash(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pages, err := s.list()
	if err != nil {
		return err
	}
	selected := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for _, p := range pages {
			if p.Deleted == "" && selected[p.Parent] && !selected[p.ID] {
				selected[p.ID] = true
				changed = true
			}
		}
	}
	var stmts []sqlite.Statement
	for _, p := range pages {
		if selected[p.ID] && p.Deleted == "" {
			stmts = append(stmts, sqlite.Statement{SQL: `UPDATE pages SET deleted=?,revision=revision+1 WHERE id=?`, Args: []any{id, p.ID}})
		}
	}
	_, err = s.db.Transaction(context.Background(), stmts)
	return err
}
func (s *Store) Restore(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pages, err := s.list()
	if err != nil {
		return err
	}
	byID := map[string]*Page{}
	for _, p := range pages {
		byID[p.ID] = p
	}
	p := byID[id]
	if p == nil || p.Deleted == "" {
		return errors.New("没有可恢复的页面")
	}
	group := p.Deleted
	root := byID[group]
	var stmts []sqlite.Statement
	if root != nil && root.Parent != "" && (byID[root.Parent] == nil || byID[root.Parent].Deleted != "") {
		stmts = append(stmts, sqlite.Statement{SQL: `UPDATE pages SET parent='' WHERE id=?`, Args: []any{root.ID}})
	}
	stmts = append(stmts, sqlite.Statement{SQL: `UPDATE pages SET deleted='',revision=revision+1 WHERE deleted=?`, Args: []any{group}})
	_, err = s.db.Transaction(context.Background(), stmts)
	return err
}
func (s *Store) Setting(key string) string {
	rows, err := s.db.Query(context.Background(), `SELECT value FROM settings WHERE key=?`, key)
	if err == nil && len(rows.Values) > 0 {
		return rows.Values[0][0].(string)
	}
	return ""
}
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Execute(context.Background(), `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
func sortedChildren(pages []*Page, parent string) []*Page {
	var out []*Page
	for _, p := range pages {
		if p.Parent == parent && p.Deleted == "" {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}

// Place moves a page next to another page in one transaction.
func (s *Store) Place(id, target string, after bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pages, err := s.list()
	if err != nil {
		return err
	}
	byID := map[string]*Page{}
	for _, p := range pages {
		byID[p.ID] = p
	}
	src, dst := byID[id], byID[target]
	if src == nil || dst == nil || src.Deleted != "" || dst.Deleted != "" || id == target {
		return errors.New("无效的页面移动")
	}
	for p := dst.Parent; p != ""; p = byID[p].Parent {
		if p == id {
			return errors.New("不能移到自己的子页面内")
		}
	}
	var ordered []*Page
	for _, p := range sortedChildren(pages, dst.Parent) {
		if p.ID == id {
			continue
		}
		if p.ID == target && !after {
			ordered = append(ordered, src)
		}
		ordered = append(ordered, p)
		if p.ID == target && after {
			ordered = append(ordered, src)
		}
	}
	var stmts []sqlite.Statement
	for i, p := range ordered {
		stmts = append(stmts, sqlite.Statement{SQL: `UPDATE pages SET parent=?,position=?,revision=revision+1 WHERE id=?`, Args: []any{dst.Parent, int64(i), p.ID}})
	}
	_, err = s.db.Transaction(context.Background(), stmts)
	return err
}
