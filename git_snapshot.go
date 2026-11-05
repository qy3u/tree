package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func (s *Store) gitSnapshot() (*gitSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A single SELECT gives all three tables the same SQLite read snapshot.
	rows, err := s.db.Query(context.Background(), `SELECT 'page',id,parent,title,position,body,updated,deleted FROM pages
 UNION ALL SELECT 'setting',key,value,'',0,'','','' FROM settings
 UNION ALL SELECT 'asset',id,name,path,0,'','','' FROM assets`)
	if err != nil {
		return nil, err
	}
	snap := &gitSnapshot{Settings: map[string]string{}, Assets: map[string]gitAsset{}, Binary: map[string][]byte{}}
	for _, r := range rows.Values {
		switch r[0].(string) {
		case "page":
			p := &Page{ID: r[1].(string), Parent: r[2].(string), Title: r[3].(string), Position: r[4].(int64), Updated: r[6].(string), Deleted: r[7].(string), Revision: 1}
			if err = strictJSON([]byte(r[5].(string)), &p.Body); err != nil {
				return nil, err
			}
			// Body.Text has its own decoder; separately reject future native fields.
			var raw struct {
				Icon      string                       `json:"icon"`
				Schema    int                          `json:"schema"`
				Text      json.RawMessage              `json:"text"`
				Tables    map[string]Table             `json:"tables"`
				TaskTimes map[string][]TaskTimeSegment `json:"taskTimes"`
			}
			if err = strictJSON([]byte(r[5].(string)), &raw); err != nil {
				return nil, err
			}
			var text storedText
			if err = strictJSON(raw.Text, &text); err != nil {
				return nil, err
			}
			if p.Body.Schema != 1 {
				return nil, errors.New("不支持的正文版本")
			}
			snap.Pages = append(snap.Pages, p)
		case "setting":
			snap.Settings[r[1].(string)] = r[2].(string)
		case "asset":
			id, name, path := r[1].(string), r[2].(string), r[3].(string)
			if !gitName(id) || !gitName(path) || path != id || path == "index.json" {
				return nil, errors.New("无效附件路径")
			}
			file := filepath.Join(s.dir, "assets", path)
			data, err := readGitRegular(file)
			if err != nil {
				return nil, err
			}
			snap.Assets[id] = gitAsset{Name: name, Path: path, SHA256: assetHash(data)}
			snap.Binary[path] = data
		}
	}
	snap.Format = gitFormat{"tree-git", gitFormatVersion, snap.Settings["libraryID"]}
	delete(snap.Settings, "libraryID")
	return snap, nil
}

func snapshotFiles(s *gitSnapshot) (map[string][]byte, error) {
	files := map[string][]byte{}
	add := func(path string, v any) error {
		b, err := gitJSON(v, true)
		if err == nil {
			files[path] = b
		}
		return err
	}
	if !gitName(s.Format.LibraryID) {
		return nil, errors.New("缺少稳定笔记库 ID")
	}
	if err := add("format.json", s.Format); err != nil {
		return nil, err
	}
	if err := add("assets/index.json", s.Assets); err != nil {
		return nil, err
	}
	for path, data := range s.Binary {
		files["assets/"+path] = data
	}
	times := map[string][]gitTime{}
	for _, p := range s.Pages {
		if !gitName(p.ID) {
			return nil, errors.New("无效页面 ID")
		}
		base := "pages/" + p.ID + "/"
		if _, ok := files[base+"page.json"]; ok {
			return nil, errors.New("重复页面 ID")
		}
		if err := add(base+"page.json", gitPage{p.Title, p.Parent, p.Position, p.Body.Icon, p.Updated, p.Deleted}); err != nil {
			return nil, err
		}
		order := []string{}
		paras, err := storedParagraphs(p.Body.Text)
		if err != nil {
			return nil, err
		}
		for _, para := range paras {
			id := para.Style.ID
			if !gitName(id) {
				return nil, fmt.Errorf("页面 %s 缺少有效 Block ID", p.ID)
			}
			path := base + "blocks/" + id + ".json"
			if _, ok := files[path]; ok {
				return nil, errors.New("重复 Block ID")
			}
			order = append(order, id)
			block, err := exportGitBlock(para, p.Body)
			if err != nil {
				return nil, err
			}
			if err = add(path, block); err != nil {
				return nil, err
			}
			if para.Style.Timer.Enabled() {
				for _, segment := range p.Body.TaskTimes[para.Style.Timer.HistoryID] {
					segment.StartedAt = segment.StartedAt.UTC()
					segment.EndedAt = segment.EndedAt.UTC()
					r := gitTime{gitTimeID(p.ID, id, segment), p.ID, id, segment}
					times[segment.Date] = append(times[segment.Date], r)
				}
			}
		}
		if err := add(base+"block-order.json", order); err != nil {
			return nil, err
		}
	}
	for date, records := range times {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return nil, err
		}
		slices.SortFunc(records, func(a, b gitTime) int {
			if c := a.StartedAt.Compare(b.StartedAt); c != 0 {
				return c
			}
			return strings.Compare(a.ID, b.ID)
		})
		var out bytes.Buffer
		for _, r := range records {
			raw, err := gitJSON(r, false)
			if err != nil {
				return nil, err
			}
			out.Write(raw)
		}
		files["time/"+date+".jsonl"] = out.Bytes()
	}
	favorites := map[string]json.RawMessage{}
	session := map[string]json.RawMessage{}
	other := map[string]string{}
	for key, value := range s.Settings {
		switch key {
		case "favorites", "pageVisits", "tabs":
			raw, err := canonicalSetting(key, []byte(value))
			if err != nil {
				return nil, fmt.Errorf("设置 %s: %w", key, err)
			}
			if key == "favorites" {
				favorites[key] = raw
			} else {
				session[key] = raw
			}
		case "selected":
			raw, _ := json.Marshal(value)
			session[key] = raw
		default:
			other[key] = value
		}
	}
	for path, v := range map[string]any{"settings/favorites.json": favorites, "settings/session.json": session, "settings/other.json": other} {
		if err := add(path, v); err != nil {
			return nil, err
		}
	}
	// Use the exact same validator for exports and imports. Incomplete or
	// inconsistent input never replaces an already usable snapshot.
	if _, err := decodeGitFiles(files); err != nil {
		return nil, err
	}
	return files, nil
}

func canonicalSetting(key string, raw []byte) (json.RawMessage, error) {
	var value any
	switch key {
	case "favorites", "pageVisits":
		var stamps map[string]time.Time
		if err := strictJSON(raw, &stamps); err != nil {
			return nil, err
		}
		value = stamps
	case "tabs":
		var session tabSession
		if err := strictJSON(raw, &session); err != nil {
			return nil, err
		}
		if len(session.Tabs) == 0 || session.Active < 0 || session.Active >= len(session.Tabs) {
			return nil, errors.New("标签会话无效")
		}
		seen := map[string]bool{}
		for _, t := range session.Tabs {
			if !gitName(t.ID) || seen[t.ID] || t.Position < 0 || t.Position >= len(t.History) {
				return nil, errors.New("标签历史无效")
			}
			seen[t.ID] = true
		}
		value = session
	default:
		return nil, errors.New("不支持的设置字段")
	}
	return gitJSON(value, false)
}

func decodeGitFiles(files map[string][]byte) (*gitSnapshot, error) {
	used := map[string]bool{}
	read := func(path string, out any) error {
		raw, ok := files[path]
		if !ok {
			return fmt.Errorf("缺少文件 %s", path)
		}
		used[path] = true
		if err := strictJSON(raw, out); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return nil
	}
	s := &gitSnapshot{Settings: map[string]string{}, Assets: map[string]gitAsset{}, Binary: map[string][]byte{}}
	if err := read("format.json", &s.Format); err != nil {
		return nil, err
	}
	if s.Format.Format != "tree-git" || s.Format.Version != gitFormatVersion || !gitName(s.Format.LibraryID) {
		return nil, errors.New("不支持的序列化版本或笔记库 ID")
	}
	if err := read("assets/index.json", &s.Assets); err != nil {
		return nil, err
	}
	for id, a := range s.Assets {
		if !gitName(id) || !gitName(a.Path) || a.Path != id || a.Path == "index.json" {
			return nil, errors.New("附件 ID 或路径无效")
		}
		path := "assets/" + a.Path
		data, ok := files[path]
		if !ok || assetHash(data) != a.SHA256 {
			return nil, fmt.Errorf("附件丢失或校验失败：%s", id)
		}
		used[path] = true
		s.Binary[a.Path] = data
	}
	for _, group := range []string{"favorites", "session"} {
		var settings map[string]json.RawMessage
		if err := read("settings/"+group+".json", &settings); err != nil {
			return nil, err
		}
		for key, value := range settings {
			if group == "favorites" && key != "favorites" || group == "session" && key != "selected" && key != "tabs" && key != "pageVisits" {
				return nil, fmt.Errorf("不支持的设置字段 %s", key)
			}
			if key == "selected" {
				var selected string
				if err := strictJSON(value, &selected); err != nil {
					return nil, err
				}
				s.Settings[key] = selected
			} else {
				raw, err := canonicalSetting(key, value)
				if err != nil {
					return nil, err
				}
				s.Settings[key] = strings.TrimSuffix(string(raw), "\n")
			}
		}
	}
	var other map[string]string
	if err := read("settings/other.json", &other); err != nil {
		return nil, err
	}
	for k, v := range other {
		if k == "libraryID" || k == "favorites" || k == "tabs" || k == "pageVisits" || k == "selected" {
			return nil, errors.New("重复设置字段")
		}
		s.Settings[k] = v
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	byID := map[string]*Page{}
	for _, path := range paths {
		parts := strings.Split(path, "/")
		if len(parts) != 3 || parts[0] != "pages" || parts[2] != "page.json" {
			continue
		}
		id := parts[1]
		if !gitName(id) {
			return nil, errors.New("无效页面 ID")
		}
		var meta gitPage
		if err := read(path, &meta); err != nil {
			return nil, err
		}
		if _, err := time.Parse(time.RFC3339Nano, meta.Updated); err != nil {
			return nil, errors.New("页面修改时间无效")
		}
		if meta.Position < 0 {
			return nil, errors.New("页面排序无效")
		}
		p := &Page{ID: id, Parent: meta.Parent, Title: meta.Title, Position: meta.Position, Updated: meta.Updated, Deleted: meta.Deleted, Revision: 1, Body: Body{Schema: 1, Icon: meta.Icon, Tables: map[string]Table{}, TaskTimes: map[string][]TaskTimeSegment{}}}
		var order []string
		base := "pages/" + id + "/"
		if err := read(base+"block-order.json", &order); err != nil {
			return nil, err
		}
		if len(order) == 0 {
			return nil, errors.New("页面至少需要一个 Block")
		}
		seen := map[string]bool{}
		doc := storedText{Paragraphs: []storedParagraph{}}
		for _, blockID := range order {
			if !gitName(blockID) || seen[blockID] {
				return nil, errors.New("重复或无效的 Block ID")
			}
			seen[blockID] = true
			var b gitBlock
			if err := read(base+"blocks/"+blockID+".json", &b); err != nil {
				return nil, err
			}
			para, err := importGitBlock(blockID, b, &p.Body)
			if err != nil {
				return nil, fmt.Errorf("Block %s: %w", blockID, err)
			}
			for _, r := range para.Runs {
				if at := r.Attachment; at != nil && strings.HasPrefix(at.URL, "asset:") {
					if _, ok := s.Assets[strings.TrimPrefix(at.URL, "asset:")]; !ok {
						return nil, errors.New("Block 引用了缺失的附件")
					}
				}
			}
			doc.Paragraphs = append(doc.Paragraphs, para)
		}
		raw, err := json.Marshal(doc)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &p.Body.Text); err != nil {
			return nil, err
		}
		s.Pages = append(s.Pages, p)
		byID[id] = p
	}
	for _, p := range s.Pages {
		seen := map[string]bool{p.ID: true}
		for parent := p.Parent; parent != ""; {
			q := byID[parent]
			if q == nil || seen[parent] {
				return nil, errors.New("页面层级缺失或成环")
			}
			seen[parent] = true
			parent = q.Parent
		}
		if p.Deleted != "" {
			root := byID[p.Deleted]
			if root == nil || root.Deleted != root.ID {
				return nil, errors.New("回收站分组无效")
			}
		}
	}
	var records []gitTime
	for _, path := range paths {
		if !strings.HasPrefix(path, "time/") || !strings.HasSuffix(path, ".jsonl") {
			continue
		}
		date := strings.TrimSuffix(strings.TrimPrefix(path, "time/"), ".jsonl")
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return nil, err
		}
		for _, line := range bytes.Split(bytes.TrimSuffix(files[path], []byte("\n")), []byte("\n")) {
			var r gitTime
			if err := strictJSON(line, &r); err != nil {
				return nil, err
			}
			if r.Date != date {
				return nil, errors.New("计时日期与文件名不一致")
			}
			records = append(records, r)
		}
		used[path] = true
	}
	if err := validateGitTimes(s.Pages, records); err != nil {
		return nil, err
	}
	for path := range files {
		if !used[path] {
			return nil, fmt.Errorf("未知或未被引用的文件：%s", path)
		}
	}
	return s, nil
}

// Refuse symlinks and special files, including a substituted attachment.
func readGitRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64<<20 {
		return nil, fmt.Errorf("不是普通文件或超过 64 MiB：%s", path)
	}
	return os.ReadFile(path)
}
