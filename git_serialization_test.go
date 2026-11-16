package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func gitFixture(t *testing.T) (*Store, *Page, *Page) {
	t.Helper()
	s := testStore(t)
	root := createPage(t, s, "", "读书笔记")
	child := createPage(t, s, root.ID, "子页面")
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	img, err := s.ImportImage(pngData.Bytes(), "图片🌲.png")
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	start := time.Date(2026, 11, 3, 23, 50, 0, 0, loc)
	timer := richtext.TaskTimer{State: richtext.TimerIdle, EstimatedNS: int64(30 * time.Minute), HasEstimate: true}.Start(start)
	segments := splitTaskTime(timer, start.Add(20*time.Minute))
	timer = timer.Stop(richtext.TimerPaused, start.Add(20*time.Minute))
	timer.HistoryID = "active-history"
	running := timer.Start(start.Add(10 * time.Hour))
	root.Body = Body{Schema: 1, Icon: "books", Tables: map[string]Table{"table-current": {Cells: [][]string{{"A", "B"}, {"中文\n换行", " x "}}}, "old-unused-table": {Cells: [][]string{{"不应导出"}}}}, TaskTimes: map[string][]TaskTimeSegment{"active-history": segments, "old-unused-history": segments[:1]}}
	ref := referenceURL(child.ID, "child-block")
	root.Body.Text = richtext.TextOf(
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "heading", Heading: 2}, Runs: []richtext.Run{{Text: "标题🌲"}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "text"}, Runs: []richtext.Run{
			{Text: "粗体", Attributes: richtext.Attributes{Weight: 700, Color: ui.Hex("#aabbcc"), Background: ui.Hex("#ffeecc"), Font: "Helvetica", Size: 18}},
			{Text: "斜体", Attributes: richtext.Attributes{Italic: true, Underline: true, Strikethrough: true}},
			{Text: "inline", Attributes: richtext.Attributes{Code: true}},
			{Text: "引用", Attributes: richtext.Attributes{Link: ref, Tag: referenceTag}},
			{Text: "missing", Attributes: richtext.Attributes{Link: referenceURL("deleted-target", "old-block"), Tag: referenceTag}},
			{Text: "链接", Attributes: richtext.Attributes{Link: "https://example.com/?a=1&b=2"}},
		}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "code", CodeBlock: true, CodeLanguage: "go"}, Runs: []richtext.Run{{Text: "func main() {\u2028\tprintln(\"中文\")  \u2028}\u2028"}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "table"}, Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: tableAttachment("table-current", root.Body.Tables["table-current"])}}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "image"}, Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: img}}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "timer", List: richtext.Tasks, Timer: timer}, Runs: []richtext.Run{{Text: "跨午夜计时"}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "running", List: richtext.Tasks, Timer: running}, Runs: []richtext.Run{{Text: "正在运行"}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "todo", List: richtext.Tasks, Checked: true, Indent: 2}, Runs: []richtext.Run{{Text: "已完成"}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "quote", Quote: true, Align: ui.Center}, Runs: []richtext.Run{{Text: "引用段落\u2028下一行"}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "bullet", List: richtext.Bullets}, Runs: []richtext.Run{{Text: "项目"}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "number", List: richtext.Numbers}, Runs: []richtext.Run{{Text: "编号"}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "divider"}, Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: dividerAttachment()}}}},
		richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "empty"}},
	)
	if err = s.Save(root); err != nil {
		t.Fatal(err)
	}
	child.Body.Text = richtext.TextOf(richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "child-block"}, Runs: []richtext.Run{{Text: "child target"}}})
	if err = s.Save(child); err != nil {
		t.Fatal(err)
	}
	deleted := createPage(t, s, "", "已删除")
	if err = s.Trash(deleted.ID); err != nil {
		t.Fatal(err)
	}
	settings := map[string]string{
		"favorites":      `{"` + root.ID + `":"2026-11-04T09:00:00+08:00"}`,
		"pageVisits":     `{"` + root.ID + `":"2026-11-04T09:00:01+08:00"}`,
		"selected":       root.ID,
		"tabs":           `{"tabs":[{"id":"tab-a","history":["` + root.ID + `","` + child.ID + `"],"position":0}],"active":0}`,
		"custom-setting": "原样保存中文和\n空行",
	}
	for k, v := range settings {
		if err = s.SetSetting(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return s, root, child
}
func exportFixture(t *testing.T, s *Store) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tree-data")
	if err := s.ExportGit(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}
func gitFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files, err := readGitDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	return files
}
func changedGitFiles(a, b map[string][]byte) []string {
	var changed []string
	for k, v := range a {
		if !bytes.Equal(v, b[k]) {
			changed = append(changed, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			changed = append(changed, k)
		}
	}
	slices.Sort(changed)
	return changed
}
func TestGitSerializationRoundTripAndStableExport(t *testing.T) {
	s, root, child := gitFixture(t)
	dir := exportFixture(t, s)
	before := gitFiles(t, dir)
	info, _ := os.Stat(filepath.Join(dir, "pages", root.ID, "page.json"))
	if err := s.ExportGit(dir); err != nil {
		t.Fatal(err)
	}
	after := gitFiles(t, dir)
	info2, _ := os.Stat(filepath.Join(dir, "pages", root.ID, "page.json"))
	if !sameGitFiles(before, after) || !info.ModTime().Equal(info2.ModTime()) {
		t.Fatal("unchanged export rewrote content")
	}
	all := []byte{}
	for _, data := range before {
		all = append(all, data...)
	}
	if bytes.Contains(all, []byte("old-unused")) || bytes.Contains(all, []byte("historyId")) || bytes.Contains(all, []byte("active-history")) {
		t.Fatal("internal histories leaked")
	}
	code := before["pages/"+root.ID+"/blocks/code.json"]
	if !bytes.Contains(code, []byte(`"lines": [`)) || !bytes.Contains(code, []byte("中文")) || bytes.Contains(code, []byte(`\u4e2d`)) {
		t.Fatal("code is not line-oriented UTF-8")
	}
	if len(bytes.Split(bytes.TrimSpace(before["time/2026-11-03.jsonl"]), []byte("\n"))) != 2 {
		t.Fatal("active timer history did not export once per task")
	}
	dest := filepath.Join(t.TempDir(), "restored")
	if err := RestoreGit(dir, dest); err != nil {
		t.Fatal(err)
	}
	restored, err := openStore(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	out := exportFixture(t, restored)
	if diff := changedGitFiles(before, gitFiles(t, out)); len(diff) > 0 {
		t.Fatalf("roundtrip diff: %v", diff)
	}
	pages, err := restored.List()
	if err != nil {
		t.Fatal(err)
	}
	var got *Page
	for _, p := range pages {
		if p.ID == root.ID {
			got = p
		}
	}
	if got == nil || got.Title != root.Title || got.Updated != root.Updated {
		t.Fatal("page metadata changed")
	}
	if len(got.Body.Tables) != 1 || len(got.Body.TaskTimes) != 2 {
		t.Fatal("effective tables/timer histories not reconstructed")
	}
	if _, ok := blockPosition(got.Body.Text, "text"); !ok {
		t.Fatal("block identity changed")
	}
	if !strings.Contains(string(before["pages/"+root.ID+"/blocks/text.json"]), referenceURL(child.ID, "child-block")) {
		t.Fatal("link target changed")
	}
	for _, r := range hydrate(got.Body, restored.dir).Runs(0, got.Body.Text.Len()) {
		if at := r.Attributes.Attachment; at != nil && strings.HasPrefix(at.URL, "asset:") && at.Image == nil {
			t.Fatal("restored image is not renderable")
		}
	}
}
func TestGitSerializationLocalDiff(t *testing.T) {
	s, p, _ := gitFixture(t)
	dir := exportFixture(t, s)
	before := gitFiles(t, dir)
	start, _ := blockPosition(p.Body.Text, "heading")
	_, end := p.Body.Text.ParagraphRange(start)
	p.Body.Text = p.Body.Text.ReplaceString(start, end, "新的标题", richtext.Attributes{})
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := s.ExportGit(dir); err != nil {
		t.Fatal(err)
	}
	after := gitFiles(t, dir)
	want := []string{"pages/" + p.ID + "/blocks/heading.json", "pages/" + p.ID + "/page.json"}
	if diff := changedGitFiles(before, after); !reflect.DeepEqual(diff, want) {
		t.Fatalf("edit changed unrelated files: %v", diff)
	}
	before = after
	doc := richtext.NewDocument(p.Body.Text)
	moveParagraph(doc, 0, 2)
	p.Body.Text = doc.Text()
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := s.ExportGit(dir); err != nil {
		t.Fatal(err)
	}
	after = gitFiles(t, dir)
	want = []string{"pages/" + p.ID + "/block-order.json", "pages/" + p.ID + "/page.json"}
	if diff := changedGitFiles(before, after); !reflect.DeepEqual(diff, want) {
		t.Fatalf("move rewrote block content: %v", diff)
	}
	before = after
	p.Title = "改名不改文件路径"
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := s.ExportGit(dir); err != nil {
		t.Fatal(err)
	}
	if diff := changedGitFiles(before, gitFiles(t, dir)); !reflect.DeepEqual(diff, []string{"pages/" + p.ID + "/page.json"}) {
		t.Fatal(diff)
	}
	// Removing a block removes its file; no stale content survives export.
	doc = richtext.NewDocument(p.Body.Text)
	editBlock(doc, 0, "delete")
	p.Body.Text = doc.Text()
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := s.ExportGit(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pages", p.ID, "blocks", "text.json")); !os.IsNotExist(err) {
		t.Fatal("deleted block file retained")
	}
}
func TestGitSerializationRejectsInvalidSnapshots(t *testing.T) {
	s, p, _ := gitFixture(t)
	dir := exportFixture(t, s)
	original := gitFiles(t, dir)
	cases := map[string]func(map[string][]byte){
		"version": func(f map[string][]byte) {
			f["format.json"] = bytes.Replace(f["format.json"], []byte(`"version": 1`), []byte(`"version": 999`), 1)
		},
		"unknown field": func(f map[string][]byte) {
			path := "pages/" + p.ID + "/page.json"
			f[path] = bytes.Replace(f[path], []byte("{\n"), []byte("{\n  \"future\": true,\n"), 1)
		},
		"duplicate key": func(f map[string][]byte) {
			f["format.json"] = bytes.Replace(f["format.json"], []byte("{\n"), []byte("{\n  \"version\": 1,\n"), 1)
		},
		"missing block": func(f map[string][]byte) { delete(f, "pages/"+p.ID+"/blocks/text.json") },
		"orphan block":  func(f map[string][]byte) { f["pages/"+p.ID+"/blocks/unused.json"] = []byte(`{}`) },
		"duplicate order": func(f map[string][]byte) {
			path := "pages/" + p.ID + "/block-order.json"
			f[path] = bytes.Replace(f[path], []byte(`"heading",`), []byte(`"heading", "heading",`), 1)
		},
		"hierarchy cycle": func(f map[string][]byte) {
			path := "pages/" + p.ID + "/page.json"
			f[path] = bytes.Replace(f[path], []byte(`"parent": ""`), []byte(`"parent": "`+p.ID+`"`), 1)
		},
		"bad style": func(f map[string][]byte) {
			path := "pages/" + p.ID + "/blocks/heading.json"
			f[path] = bytes.Replace(f[path], []byte(`"heading": 2`), []byte(`"heading": 99`), 1)
		},
		"bad timer": func(f map[string][]byte) {
			path := "pages/" + p.ID + "/blocks/timer.json"
			f[path] = bytes.Replace(f[path], []byte(`"elapsedNs": 1200000000000`), []byte(`"elapsedNs": 1`), 1)
		},
		"bad time date": func(f map[string][]byte) {
			f["time/2026-11-03.jsonl"] = bytes.ReplaceAll(f["time/2026-11-03.jsonl"], []byte(`"date":"2026-11-03"`), []byte(`"date":"2026-11-02"`))
		},
		"bad image": func(f map[string][]byte) {
			for path := range f {
				if strings.HasSuffix(path, ".png") {
					f[path] = []byte("broken")
				}
			}
		},
		"future run attr": func(f map[string][]byte) {
			path := "pages/" + p.ID + "/blocks/text.json"
			f[path] = bytes.Replace(f[path], []byte(`"weight": 700`), []byte(`"weight": 700, "future": true`), 1)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			files := map[string][]byte{}
			for path, data := range original {
				files[path] = bytes.Clone(data)
			}
			mutate(files)
			bad, err := writeGitStage(t.TempDir(), files)
			if err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "restored")
			if err = RestoreGit(bad, dest); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
			if _, err = os.Stat(dest); !os.IsNotExist(err) {
				t.Fatal("failed restore published destination")
			}
		})
	}
}
func TestGitSerializationProtectsExistingDirectories(t *testing.T) {
	s, _, _ := gitFixture(t)
	dir := exportFixture(t, s)
	occupied := t.TempDir()
	os.WriteFile(filepath.Join(occupied, "keep.txt"), []byte("keep"), 0600)
	if err := s.ExportGit(occupied); err == nil {
		t.Fatal("unrelated directory overwritten")
	}
	if err := RestoreGit(dir, occupied); err == nil {
		t.Fatal("nonempty restore target accepted")
	}
	if data, _ := os.ReadFile(filepath.Join(occupied, "keep.txt")); string(data) != "keep" {
		t.Fatal("unrelated file lost")
	}
	if err := s.ExportGit(filepath.Join(s.dir, "snapshot")); err == nil {
		t.Fatal("data dir overlap allowed")
	}
	other := testStore(t)
	createPage(t, other, "", "other")
	if err := other.ExportGit(dir); err == nil {
		t.Fatal("different library overwritten")
	}
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(dir, link)
	if err := RestoreGit(link, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("symlink source allowed")
	}
	os.Symlink(filepath.Join(occupied, "keep.txt"), filepath.Join(dir, "escape"))
	if err := RestoreGit(dir, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("symlink inside snapshot allowed")
	}
}
func TestGitSerializationLegacyIDsArePersistedBeforeExport(t *testing.T) {
	s := testStore(t)
	p := createPage(t, s, "", "旧页面")
	body := bodyFromMarkdown("第一段\n\n第二段")
	raw, _ := json.Marshal(body)
	if _, err := s.db.Execute(context.Background(), `UPDATE pages SET body=? WHERE id=?`, string(raw), p.ID); err != nil {
		t.Fatal(err)
	}
	s.Close()
	reopened, err := openStore(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	pages, _ := reopened.List()
	if pages[0].Updated != p.Updated {
		t.Fatal("identity migration touched modified time")
	}
	for _, para := range pages[0].Body.Text.Paragraphs() {
		if para.Style.ID == "" {
			t.Fatal("unopened page missing ID")
		}
	}
	a := exportFixture(t, reopened)
	b := exportFixture(t, reopened)
	if !sameGitFiles(gitFiles(t, a), gitFiles(t, b)) {
		t.Fatal("export regenerated IDs")
	}
}
func TestGitSerializationActualGitDiffAndCLI(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	s := testStore(t)
	p := createPage(t, s, "", "中文页面")
	repo := t.TempDir()
	dir := filepath.Join(repo, "tree-data")
	var output bytes.Buffer
	if _, handled, err := runDataCommand([]string{"--data-dir", s.dir, "--export-git", dir}, "", &output); err != nil || !handled {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
		return string(out)
	}
	git("init", "-q")
	git("add", "tree-data")
	git("-c", "user.name=Tree Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "snapshot")
	if err := s.ExportGit(dir); err != nil {
		t.Fatal(err)
	}
	if git("status", "--porcelain") != "" {
		t.Fatal("repeat export dirties git")
	}
	p.Title = "新标题"
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := s.ExportGit(dir); err != nil {
		t.Fatal(err)
	}
	diff := git("diff", "--name-only")
	if strings.TrimSpace(diff) != "tree-data/pages/"+p.ID+"/page.json" {
		t.Fatal(diff)
	}
	dest := filepath.Join(t.TempDir(), "restored")
	if _, handled, err := runDataCommand([]string{"--restore-git", dir, "--data-dir", dest}, "", &output); err != nil || !handled {
		t.Fatal(err)
	}
	if _, _, err := runDataCommand([]string{"--restore-git", dir}, s.dir, &output); err == nil {
		t.Fatal("implicit restore destination allowed")
	}
}
