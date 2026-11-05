package main

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func createTestLeaf(t *testing.T) (*app, *ui.Tester, *draft, *leafView) {
	t.Helper()
	a, tt := commandTestApp(t)
	tt.Type("/leaf")
	if _, ok := tt.Find("命令：Leaf"); !ok {
		t.Fatal("Leaf command missing")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	d := a.current()
	if len(d.page.Body.Leaves) != 1 {
		t.Fatalf("leaf not created: %q", d.doc.Text().String())
	}
	var v *leafView
	for _, view := range d.leafViews {
		v = view
	}
	if v == nil {
		t.Fatal("leaf view absent")
	}
	return a, tt, d, v
}

func TestLeafCreateEditCollapseAndReload(t *testing.T) {
	a, tt, d, v := createTestLeaf(t)
	d.page.Title = "知识笔记"
	if !tt.Focused("Leaf 标题") {
		t.Fatal("new Leaf did not focus title")
	}
	tt.Type("关于间隔重复")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !tt.Focused("Leaf 正文") {
		t.Fatal("title Enter did not focus Leaf content")
	}
	tt.Type("在遗忘之前再次回忆，可以巩固长期记忆。")
	tt.Key(0, ui.KeyEnter)
	tt.Type("/todo")
	tt.Key(0, ui.KeyEnter)
	tt.Type("尝试主动回忆")
	if v.draft.doc.Text().ParagraphStyleAt(v.draft.doc.Text().Len()).List != richtext.Tasks {
		t.Fatal("Leaf slash command failed")
	}
	if strings.Contains(d.doc.Text().String(), "主动回忆") {
		t.Fatal("Leaf content leaked into parent editor")
	}
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	version := d.doc.Version()
	if err := tt.Click("收起 Leaf"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !v.collapsed || !strings.Contains(leafExcerpt(v.draft.page.Body), "遗忘") {
		t.Fatal("collapsed excerpt missing")
	}
	if d.doc.Version() != version {
		t.Fatal("collapse edited content")
	}
	if path := os.Getenv("TREE_LEAF_SCREENSHOT"); path != "" {
		f, e := os.Create(strings.TrimSuffix(path, ".png") + "-collapsed.png")
		if e != nil {
			t.Fatal(e)
		}
		if e = png.Encode(f, tt.Image()); e != nil {
			t.Fatal(e)
		}
		f.Close()
	}
	if err := tt.Click("展开 Leaf"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if rect, ok := tt.Find("Leaf 正文"); !ok || rect.W < 400 || rect.H < 56 {
		t.Fatalf("Leaf editor clipped: %+v", rect)
	}
	if len(v.draft.doc.VisibleParagraphs()) != 2 {
		t.Fatal("Leaf content not fully visible")
	}
	if v.collapsed {
		t.Fatal("Leaf not expanded")
	}
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	reopened.selected = d.page.ID
	restored := reopened.current().page.Body.Leaves[v.draft.leafID]
	if restored.Title != "关于间隔重复" || !strings.Contains(restored.Content.PlainText(), "主动回忆") {
		t.Fatalf("reload lost Leaf: %+v", restored)
	}
	if !strings.Contains(d.page.Body.Markdown(), "## 关于间隔重复") || !strings.Contains(d.page.Body.PlainText(), "主动回忆") {
		t.Fatal("export/search omitted Leaf")
	}
	if path := os.Getenv("TREE_LEAF_SCREENSHOT"); path != "" {

		f, e := os.Create(path)
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		if e = png.Encode(f, tt.Image()); e != nil {
			t.Fatal(e)
		}
	}
}

func TestLeafMoveDeleteUndoAndIndependentCopy(t *testing.T) {
	a, _, d, v := createTestLeaf(t)
	v.draft.page.Title = "知识"
	v.draft.doc.InsertText("保留内容")
	a.changed(v.draft)
	original := v.draft.leafID
	a.editLeafAwareBlock(d, 0, "duplicate")
	a.changed(d)
	if len(d.page.Body.Leaves) != 2 {
		t.Fatal("duplicate reused Leaf identity")
	}
	var copied string
	for id := range d.page.Body.Leaves {
		if id != original {
			copied = id
		}
	}
	copy := a.leafView(d, copied)
	copy.draft.doc.InsertText("修改副本")
	a.changed(copy.draft)
	if strings.Contains(v.draft.doc.Text().String(), "副本") {
		t.Fatal("copies share content")
	}
	if !moveParagraph(d.doc, 0, 1) {
		t.Fatal("move failed")
	}
	a.changed(d)
	if !bodyHasLeaf(d.doc.Text(), original) {
		t.Fatal("move lost identity")
	}
	editBlock(d.doc, 1, "delete")
	a.changed(d)
	if bodyHasLeaf(d.doc.Text(), original) {
		t.Fatal("delete failed")
	}
	d.doc.Undo()
	a.changed(d)
	if !bodyHasLeaf(d.doc.Text(), original) || d.page.Body.Leaves[original].Content.PlainText() != "保留内容" {
		t.Fatal("undo lost Leaf content")
	}
}

func TestLeafGitRoundTrip(t *testing.T) {
	a, _, d, v := createTestLeaf(t)
	v.draft.page.Title = "序列化知识"
	v.draft.doc.InsertText("正文第一行\n正文第二行")
	a.changed(v.draft)
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "snapshot")
	if err := a.store.ExportGit(dest); err != nil {
		t.Fatal(err)
	}
	restoredDir := filepath.Join(t.TempDir(), "restored")
	if err := RestoreGit(dest, restoredDir); err != nil {
		t.Fatal(err)
	}
	restored, err := openStore(restoredDir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	pages, err := restored.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		if p.ID == d.page.ID {
			leaf := p.Body.Leaves[v.draft.leafID]
			if leaf.Title != "序列化知识" || leaf.Content.PlainText() != "正文第一行\n正文第二行" {
				t.Fatal("Git restore lost Leaf")
			}
			return
		}
	}
	t.Fatal("restored page absent")
}

func TestLeafLayoutGrowthAndTitleBoundary(t *testing.T) {
	a, tt, d, v := createTestLeaf(t)
	tt.Type("知识标题")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Type("原来的第一行")
	if err := tt.Click("Leaf 标题"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyRight)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if v.draft.doc.Text().String() != "\n原来的第一行" {
		t.Fatal("title Enter overwrote first line")
	}
	tt.Key(0, ui.KeyBackspace)
	tt.Frame()
	if !tt.Focused("Leaf 标题") || v.draft.doc.Text().String() != "原来的第一行" {
		t.Fatal("Backspace did not return to Leaf title")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Type(strings.Repeat("这是一段用于检查长内容展开与滚动的知识。\n", 40))
	for i := 0; i < 5; i++ {
		tt.Frame()
	}
	rect, ok := tt.Find("Leaf 正文")
	if !ok || rect.W < 400 || rect.H < 600 {
		t.Fatalf("inline editor did not fit content: %+v", rect)
	}
	if v.height < v.draft.doc.ContentHeight()+60 {
		t.Fatalf("card clips content: height=%v content=%v", v.height, v.draft.doc.ContentHeight())
	}
	if d.doc.ScrollOffset() == 0 {
		t.Fatal("outer document did not reveal Leaf caret")
	}
	if v.draft.doc.ScrollOffset() != 0 {
		t.Fatal("Leaf introduced nested scrolling")
	}
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
}

func TestLeafDuplicateSingleUndo(t *testing.T) {
	a, _, d, v := createTestLeaf(t)
	v.draft.doc.InsertText("original")
	a.changed(v.draft)
	before := d.doc.Text()
	a.editLeafAwareBlock(d, 0, "duplicate")
	a.changed(d)
	if d.doc.Text().ParagraphCount() != before.ParagraphCount()+1 {
		t.Fatal("duplicate missing")
	}
	d.doc.Undo()
	a.changed(d)
	if !d.doc.Text().Equal(before) {
		t.Fatal("duplicate requires more than one undo")
	}
	d.doc.Redo()
	a.changed(d)
	ids := map[string]bool{}
	for _, at := range (Body{Text: d.doc.Text()}).attachments() {
		if id := leafID(at); id != "" {
			if ids[id] {
				t.Fatal("redo shares Leaf identity")
			}
			ids[id] = true
		}
	}
	if len(ids) != 2 {
		t.Fatal("redo lost copy")
	}
}

func TestLeafGitDiffAndRichContentBackup(t *testing.T) {
	s, page, _ := gitFixture(t)
	// Reuse the existing rich fixture: code, images, tables, links and a timer
	// whose historical intervals cross midnight.
	body := page.Body
	id := newID()
	page.Body = emptyBody()
	page.Body.Leaves = map[string]Leaf{id: {Title: "完整知识", Content: body}}
	page.Body.Text = richtext.TextOf(richtext.Paragraph{Style: richtext.ParagraphStyle{ID: newID()}, Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: leafAttachment(id)}}}})
	if err := s.Save(page); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "git")
	if err := s.ExportGit(dir); err != nil {
		t.Fatal(err)
	}
	before := gitFiles(t, dir)
	leaf := page.Body.Leaves[id]
	leaf.Title = "修改知识标题"
	page.Body.Leaves[id] = leaf
	if err := s.Save(page); err != nil {
		t.Fatal(err)
	}
	if err := s.ExportGit(dir); err != nil {
		t.Fatal(err)
	}
	after := gitFiles(t, dir)
	changed := changedGitFiles(before, after)
	if len(changed) != 2 || changed[0] != "pages/"+page.ID+"/leaves/"+id+".json" || changed[1] != "pages/"+page.ID+"/page.json" {
		t.Fatalf("unrelated Git churn: %v", changed)
	}
	restoredDir := filepath.Join(t.TempDir(), "restored")
	if err := RestoreGit(dir, restoredDir); err != nil {
		t.Fatal(err)
	}
	restored, err := openStore(restoredDir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	again := filepath.Join(t.TempDir(), "again")
	if err = restored.ExportGit(again); err != nil {
		t.Fatal(err)
	}
	if !sameGitFiles(after, gitFiles(t, again)) {
		t.Fatal("rich Leaf round trip changed snapshot")
	}
	backup, err := s.Backup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := testStore(t)
	if _, err = target.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	pages, err := target.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range pages {
		for restoredID, leaf := range p.Body.Leaves {
			if restoredID == id || !bodyHasLeaf(p.Body.Text, restoredID) {
				t.Fatal("backup did not remap Leaf identity and placement together")
			}
			found = true
			for _, at := range leaf.Content.attachments() {
				if strings.HasPrefix(at.URL, "asset:") {
					if _, err = os.Stat(filepath.Join(target.dir, "assets", strings.TrimPrefix(at.URL, "asset:"))); err != nil {
						t.Fatal("Leaf image lost during backup restore")
					}
				}
			}
			if !strings.Contains(leaf.Content.Markdown(), "func main()") {
				t.Fatal("Leaf code missing")
			}
		}
	}
	if !found {
		t.Fatal("backup omitted Leaf")
	}
}

func TestLeafCreationOwnParagraphAndUndo(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("# ")
	tt.Type("前文 /leaf")
	before := a.current().doc.Text()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	d := a.current()
	found := false
	for start, p := range d.doc.Text().Paragraphs() {
		if id := leafID(d.doc.Text().AttributesAt(start).Attachment); id != "" {
			found = true
			if p.Style.Heading != 0 || p.Style.List != richtext.NoList || p.Style.CodeBlock {
				t.Fatal("Leaf inherited surrounding block format")
			}
		}
	}
	if !found || !strings.Contains(d.doc.Text().String(), "前文 ") {
		t.Fatal("creating Leaf damaged surrounding content")
	}
	d.doc.Undo()
	a.changed(d)
	if !d.doc.Text().Equal(before) {
		t.Fatal("Leaf creation did not undo in one step")
	}
}
