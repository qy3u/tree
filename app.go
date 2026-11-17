package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

type draft struct {
	linkHover      linkHover
	parent         *draft
	leafID         string
	leafViews      map[string]*leafView
	layoutRevision uint64
	editor         ui.Handle
	gutter         gutterInteraction
	page           *Page
	doc            *richtext.Document
	version        uint64
	title          string
	dirty          bool
}
type pageDrag string
type paragraphDrag struct {
	Page  string
	Index int
}
type app struct {
	mathEdit         *mathEditState
	urlPaste         *urlPasteState
	taskEstimate      *taskEstimatePrompt
	floatingTasksOpen bool
	taskChecks          map[string]taskCheck
	taskCheckTimer      *time.Timer
	taskCheckStopped    bool
	taskNotices         map[string]func()
	taskNotify          func(string, string, string, time.Time) func()
	taskNotifyPanel     *mygo.Window
	taskNotifyDismissed map[string]string
	taskNotifyWatering  map[string]taskWateringAnim
	taskWaterTimer      *time.Timer

	editorDraft                               *draft
	childMenu                                 string
	childMenuOpen                             bool
	iconPicker                                pageIconPicker
	blockJump                                 blockJump
	codeMenu                                  codeLanguageMenu
	tabPicker                                 tabPickerState
	visited                                   map[string]time.Time
	favorites                                 map[string]time.Time
	libraryFavorites                          bool
	focusTitlePage                            string
	focusTitleEndPage                         string
	titleBoundaryUndo                         titleBoundaryUndo
	store                                     *Store
	pages                                     []*Page
	drafts                                    map[string]*draft
	expanded                                  map[string]*bool
	selected                                  string
	tabs                                      []pageTab
	activeTab                                 int
	revealTab                                 bool
	tabScroll                                 ui.ScrollState
	tabDragTick                               time.Time
	win                                       *mygo.Window
	hide                                      func()
	quitting                                  bool
	timer                                     *time.Timer
	status, failure                           string
	errorOpen                                 bool
	searchOpen                                bool
	searchSelection                           int
	searchPointerX, searchPointerY            float32
	query                                     string
	trashOpen, moveOpen, pageMenu, insertOpen bool
	linkOpen                                  bool
	linkURL                                   string
	linkEdit                                  linkEditState
	tableOpen                                 bool
	table                                     Table
	tableStart, tableEnd                      int
	tablePage                                 string
	slash, pendingSlash                       *slashCommand
	slashDismissed                            string
	insertQuery                               string
	blockMenu                                 bool
	blockID                                   string
	blockMenuPage                             string
	formatHandle                              ui.Handle
	editorHandle                              ui.Handle
	titleHandle                               ui.Handle
}

func newApp(s *Store) (*app, error) {
	a := &app{store: s, drafts: map[string]*draft{}, expanded: map[string]*bool{}, status: "已保存到本机", hide: mygo.App.Hide}
	if err := a.reload(); err != nil {
		return nil, err
	}
	if len(a.pages) == 0 {
		if _, err := s.Create("", "开始使用", bodyFromMarkdown("这是一份保存在本机的笔记。\n\n## 从这里开始\n\n- 在左侧创建页面，也可以在页面内添加子页面。\n- 选中文字设置格式，或输入 Markdown 快捷标记。\n- 在新段落输入 /，插入标题、列表、图片和表格。\n\n### 试着写一点什么\n\n- [ ] 写下第一个想法\n- [ ] 为它创建一个子页面\n\n所有修改会自动保存。")); err != nil {
			return nil, err
		}
		if err := a.reload(); err != nil {
			return nil, err
		}
	}
	a.selected = s.Setting("selected")
	if p := a.find(a.selected); p == nil || p.Deleted != "" {
		a.selected = ""
		for _, p := range a.pages {
			if p.Deleted == "" {
				a.selected = p.ID
				break
			}
		}
	}
	a.loadTaskChecks()
	a.restoreTabs()
	a.loadFavorites()
	a.recordVisit(a.selected)
	a.expandAncestors(a.selected)
	return a, nil
}
func (a *app) reload() error {
	pages, err := a.store.List()
	if err != nil {
		return err
	}
	a.pages = pages
	for _, p := range pages {
		if d := a.drafts[p.ID]; d != nil {
			d.page = p
		}
	}
	return nil
}
func (a *app) find(id string) *Page {
	for _, p := range a.pages {
		if p.ID == id {
			return p
		}
	}
	return nil
}
func (a *app) current() *draft { return a.draftForPage(a.selected) }
func (a *app) draftForPage(id string) *draft {
	p := a.find(id)
	if p == nil || p.Deleted != "" {
		return nil
	}
	if d := a.drafts[p.ID]; d != nil {
		return d
	}
	text, restyled := styleReferences(hydrate(p.Body, a.store.dir))
	d := &draft{page: p, doc: richtext.NewDocument(text), title: p.Title}
	before := d.doc.Version()
	d.doc.IdentifyParagraphs(newID)
	d.dirty = restyled || d.doc.Version() != before // Persist IDs assigned to older notes, even without typing.
	d.version = d.doc.Version()
	a.drafts[p.ID] = d
	return d
}
func (a *app) expandedState(id string) *bool {
	if a.expanded[id] == nil {
		v := false
		a.expanded[id] = &v
	}
	return a.expanded[id]
}
func (a *app) expandAncestors(id string) {
	for p := a.find(id); p != nil && p.Parent != ""; p = a.find(p.Parent) {
		*a.expandedState(p.Parent) = true
	}
}
func (a *app) report(err error) {
	if err != nil {
		a.failure = err.Error()
		a.errorOpen = true
		a.status = "操作失败"
	}
}
func (a *app) changed(d *draft) {
	if d == nil {
		return
	}
	if d.doc.Version() == d.version && d.title == d.page.Title {
		return
	}
	// Native rich copy keeps the attachment ID. Copy the referenced table
	// definition as well when content is pasted into a different page.
	for _, r := range d.doc.Text().Runs(0, d.doc.Text().Len()) {
		at := r.Attributes.Attachment
		if at == nil || !strings.HasPrefix(at.URL, "table:") {
			continue
		}
		id := strings.TrimPrefix(at.URL, "table:")
		if _, ok := d.page.Body.Tables[id]; ok {
			continue
		}
		for _, p := range a.pages {
			if t, ok := p.Body.Tables[id]; ok {
				d.page.Body.Tables[id] = copyTable(t)
				break
			}
		}
	}
	d.version = d.doc.Version()
	d.title = d.page.Title
	d.dirty = true
	if d.parent != nil {
		a.syncLeaf(d)
	}
	a.status = "待保存…"
	if a.timer != nil {
		a.timer.Stop()
	}
	if a.win != nil {
		a.timer = time.AfterFunc(450*time.Millisecond, func() { a.win.Update(func() { a.report(a.flush()) }) })
	}
}
func (a *app) flush() error {
	for _, d := range a.drafts {
		for _, v := range d.leafViews {
			a.syncLeaf(v.draft)
		}
		if d.doc.Version() != d.version || d.title != d.page.Title {
			d.dirty = true
		}
		if !d.dirty {
			continue
		}
		d.page.Body.Text = d.doc.Text()
		if err := a.store.Save(d.page); err != nil {
			a.status = "保存失败 · 内容仍在窗口中"
			return err
		}
		d.version = d.doc.Version()
		d.title = d.page.Title
		d.dirty = false
	}
	a.status = "已保存到本机"
	return nil
}
func (a *app) selectPage(id string) {
	if p := a.find(id); id != libraryRoute && (p == nil || p.Deleted != "") {
		return
	}
	if err := a.flush(); err != nil {
		a.report(err)
		return
	}
	tab := &a.tabs[a.activeTab]
	if tab.page() != id {
		tab.History = append(tab.History[:tab.Position+1], id)
		tab.Position = len(tab.History) - 1
	}
	a.showTab()
}
func (a *app) showPage(id string) {
	if a.mathEdit != nil {
		a.finishMathEdit(true)
	}
	a.urlPaste = nil
	if a.linkOpen {
		a.finishLinkEdit(false, false)
	}
	a.editorDraft = nil
	a.selected = id
	a.focusTitleEndPage = ""
	a.titleBoundaryUndo = titleBoundaryUndo{}
	a.iconPicker.Open = false
	a.childMenuOpen = false
	a.pageMenu = false
	a.blockMenu = false
	a.insertOpen = false
	a.slash, a.pendingSlash = nil, nil
	a.slashDismissed = ""
	a.codeMenu.Open = false
	a.linkOpen, a.tableOpen, a.moveOpen = false, false, false
	a.expandAncestors(id)
}
func (a *app) addPage(parent string) {
	if parent == libraryRoute {
		parent = ""
	}
	if err := a.flush(); err != nil {
		a.report(err)
		return
	}
	p, err := a.store.Create(parent, "", emptyBody())
	if err != nil {
		a.report(err)
		return
	}
	if err = a.reload(); err != nil {
		a.report(err)
		return
	}
	a.selectPage(p.ID)
	if parent != "" {
		*a.expandedState(parent) = true
	}
	// Focus the new page's title when it mounts, not the previous page's handle.
	a.focusTitlePage = p.ID
}
func (a *app) mutate(fn func() error) {
	if err := a.flush(); err != nil {
		a.report(err)
		return
	}
	if err := fn(); err != nil {
		a.report(err)
		return
	}
	a.report(a.reload())
	a.pruneTabs()
	a.showTab()
}
func (a *app) moveCurrent(delta int) {
	if d := a.current(); d != nil {
		_, caret := d.doc.Selection()
		index := 0
		for pos := range d.doc.Text().Paragraphs() {
			if pos > caret {
				break
			}
			if pos != 0 {
				index++
			}
		}
		moveParagraph(d.doc, index, index+delta)
		a.changed(d)
	}
}
func (a *app) openLink() {
	if d := a.editing(); d != nil {
		a.beginLinkEdit(d)
	}
}
func (a *app) insertAttachment(at *richtext.Attachment) {
	if d := a.editing(); d != nil {
		anchor, caret := d.doc.Selection()
		lo, hi := min(anchor, caret), max(anchor, caret)
		if start, end, ok := a.pendingCommandRange(d); ok {
			lo, hi = start, end
			d.doc.SetSelection(lo, hi)
		}
		a.pendingSlash = nil
		prefix, suffix := "", "\n"
		if lo > 0 && d.doc.Text().Slice(lo-1, lo).PlainText() != "\n" {
			prefix = "\n"
		}
		if hi < d.doc.Text().Len() && d.doc.Text().Slice(hi, hi+1).PlainText() == "\n" {
			suffix = ""
		}
		text := richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: prefix}, {Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: at}}, {Text: suffix}}})
		d.doc.Insert(text)
		a.changed(d)
	}
}
func (a *app) importImage() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "插入图片", Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp"}}}})
	if err != nil {
		a.report(err)
		return
	}
	if len(paths) == 0 {
		return
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		a.report(err)
		return
	}
	at, err := a.store.ImportImage(data, filepath.Base(paths[0]))
	if err != nil {
		a.report(err)
		return
	}
	a.insertAttachment(at)
}
func (a *app) pasteImage() {
	data := mygo.Clipboard.ReadImage()
	if len(data) == 0 {
		a.report(fmt.Errorf("剪贴板中没有图片"))
		return
	}
	at, err := a.store.ImportImage(data, "粘贴的图片.png")
	if err != nil {
		a.report(err)
		return
	}
	a.insertAttachment(at)
}
func (a *app) export() {
	if err := a.flush(); err != nil {
		a.report(err)
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择 Markdown 导出位置", Directory: true, CreateDirectories: true})
	if err != nil {
		a.report(err)
		return
	}
	if len(paths) == 0 {
		return
	}
	out, err := a.store.Export(a.selected, paths[0])
	if err != nil {
		a.report(err)
		return
	}
	a.status = "已导出到 " + out
}
func (a *app) openTable() {
	d := a.editing()
	if d == nil {
		return
	}
	a.table = Table{Cells: [][]string{{"", ""}, {"", ""}, {"", ""}}}
	a.tableStart = -1
	a.tableEnd = -1
	a.tablePage = d.page.ID
	anchor, caret := d.doc.Selection()
	lo, hi := min(anchor, caret), max(anchor, caret)
	for pos, r := range d.doc.Text().Runs(0, d.doc.Text().Len()) {
		at := r.Attributes.Attachment
		if at != nil && strings.HasPrefix(at.URL, "table:") && pos <= hi && pos+1 >= lo {
			if table, ok := d.page.Body.Tables[strings.TrimPrefix(at.URL, "table:")]; ok {
				a.table = copyTable(table)
				a.tableStart = pos
				a.tableEnd = pos + 1
				break
			}
		}
	}
	a.tableOpen = true
}
func (a *app) saveTable() {
	d := a.editing()
	if d == nil || d.page.ID != a.tablePage {
		return
	}
	id := newID()
	d.page.Body.Tables[id] = copyTable(a.table)
	at := tableAttachment(id, a.table)
	if a.tableStart >= 0 {
		d.doc.SetSelection(a.tableStart, a.tableEnd)
		d.doc.InsertAttachment(at)
		a.changed(d)
	} else {
		a.insertAttachment(at)
	}
	a.tableOpen = false
}
