package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

// Leaf is a knowledge unit owned by a page. Its content uses the same native
// editing primitives, but its identity and lifecycle belong to the whole unit.
type Leaf struct {
	Locked  bool   `json:"locked,omitempty"`
	Title   string `json:"title"`
	Content Body   `json:"content"`
}

type leafView struct {
	draft      *draft
	collapsed  bool
	height     float32
	focusTitle bool
	focusBody  bool
	caret        int
	version      uint64
	reveal       bool
	centerReveal bool
}

func leafDisplayTitle(s string) string {
	if strings.TrimSpace(s) == "" {
		return "未命名 Leaf"
	}
	return s
}

func leafExcerpt(b Body) string {
	text := strings.ReplaceAll(b.PlainText(), string(richtext.ObjectReplacement), "")
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 150 {
		return string(runes[:150]) + "…"
	}
	return text
}

func leafAttachment(id string) *richtext.Attachment {
	return &richtext.Attachment{URL: "leaf:" + id, Name: "Leaf", Width: 720, Height: 180,
		Draw: func(p *ui.Painter, r ui.Rect) { p.Fill(r, paper, 8) }}
}

func leafID(at *richtext.Attachment) string {
	if at != nil && strings.HasPrefix(at.URL, "leaf:") {
		return strings.TrimPrefix(at.URL, "leaf:")
	}
	return ""
}

func (a *app) editing() *draft {
	if d := a.editorDraft; d != nil {
		if d.parent == nil && d.page.ID == a.selected {
			return d
		}
		if d.parent != nil && d.parent.page.ID == a.selected && bodyHasLeaf(d.parent.doc.Text(), d.leafID) {
			return d
		}
	}
	return a.current()
}

func bodyHasLeaf(text richtext.Text, id string) bool {
	for _, r := range text.Runs(0, text.Len()) {
		if leafID(r.Attributes.Attachment) == id {
			return true
		}
	}
	return false
}

func (a *app) leafView(d *draft, id string) *leafView {
	if d.leafViews == nil {
		d.leafViews = map[string]*leafView{}
	}
	if v := d.leafViews[id]; v != nil {
		return v
	}
	leaf, ok := d.page.Body.Leaves[id]
	if !ok {
		return nil
	}
	doc := richtext.NewDocument(hydrate(leaf.Content, a.store.dir))
	before := doc.Version()
	doc.IdentifyParagraphs(newID)
	child := &draft{parent: d, leafID: id, page: &Page{ID: d.page.ID + ":" + id, Title: leaf.Title, Body: leaf.Content}, doc: doc, title: leaf.Title, version: doc.Version(), dirty: doc.Version() != before}
	if child.page.Body.Tables == nil {
		child.page.Body.Tables = map[string]Table{}
	}
	v := &leafView{draft: child, height: 180}
	d.leafViews[id] = v
	a.syncLeaf(child)
	return v
}

func (a *app) syncLeaf(d *draft) {
	if d == nil || d.parent == nil {
		return
	}
	if !d.dirty && d.version == d.doc.Version() && d.title == d.page.Title {
		return
	}
	d.page.Body.Text = d.doc.Text()
	leaf := d.parent.page.Body.Leaves[d.leafID]
	leaf.Title, leaf.Content = d.page.Title, d.page.Body
	d.parent.page.Body.Leaves[d.leafID] = leaf
	d.parent.dirty = true
	d.version, d.title, d.dirty = d.doc.Version(), d.page.Title, false
}

func (a *app) toggleLeafLock(d *draft, id string) {
	v := a.leafView(d, id)
	if v == nil {
		return
	}
	a.syncLeaf(v.draft)
	leaf := d.page.Body.Leaves[id]
	leaf.Locked = !leaf.Locked
	d.page.Body.Leaves[id] = leaf
	d.dirty = true
	v.focusTitle, v.focusBody = false, false
	a.dismissSlash()
	a.editorDraft, a.editorHandle = d, d.editor
	d.editor.Focus()
	a.changed(d)
}

func (a *app) createLeaf(d *draft) {
	// Leaves have one page owner; this first version does not nest them.
	if d.parent != nil {
		return
	}
	id := newID()
	if d.page.Body.Leaves == nil {
		d.page.Body.Leaves = map[string]Leaf{}
	}
	d.page.Body.Leaves[id] = Leaf{Content: emptyBody()}
	a.editorDraft = d
	a.insertAttachment(leafAttachment(id))
	v := a.leafView(d, id)
	v.focusTitle = true
	a.changed(d)
}

func (a *app) leafSize(d *draft, at *richtext.Attachment, width float32) (float32, float32) {
	if id := leafID(at); id != "" {
		if v := a.leafView(d, id); v != nil {
			if v.collapsed {
				return width, 132
			}
			return width, v.height
		}
	}
	return 0, 0
}

func (a *app) leafControls(c *ui.Context, d *draft) {
	// Link callbacks run between frames; retain window services, not the frame context.
	services := c.Services()
	for _, box := range d.doc.VisibleAttachments() {
		id := leafID(box.Attachment)
		if id == "" {
			continue
		}
		v := a.leafView(d, id)
		if v == nil {
			continue
		}
		child := v.draft
		locked := d.page.Body.Leaves[id].Locked
		r := box.Rect
		card := ui.Column(c.Key("leaf:"+id)).Label("Leaf 知识单元").Absolute().Left(gutterWidth+r.X).Top(r.Y).Width(r.W).Height(r.H).AlignItems(ui.Stretch).Background(paper).Border(1, line).Radius(8).Clip()
		card.Children(func() {
			ui.Row(c).Height(52).Shrink(0).Padding(10, 12).Gap(6).AlignItems(ui.Center).Children(func() {
				icon, label := "chevron-down", "收起 Leaf"
				if v.collapsed {
					icon, label = "chevron-right", "展开 Leaf"
				}
				if iconButton(c, icon, label).Size(24, 28).Clicked() {
					a.syncLeaf(child)
					v.collapsed = !v.collapsed
					d.layoutRevision++
					a.dismissSlash()
					if v.collapsed {
						a.editorDraft = d
						a.editorHandle = d.editor
						d.editor.Focus()
					}
					c.After(0)
				}
				lockLabel, color := "锁定 Leaf", muted
				if locked {
					lockLabel, color = "解锁 Leaf", ui.Hex("#38965b")
				}
				if iconButton(c, "leaf", lockLabel).Size(24, 28).TextColor(color).Clicked() {
					a.toggleLeafLock(d, id)
					locked = d.page.Body.Leaves[id].Locked
				}
				if locked {
					ui.Text(c, leafDisplayTitle(child.page.Title)).FontSize(18).Bold().Height(32).Grow(1).MinWidth(0)
					return
				}
				title := ui.TextInputBase(c.Key("leaf-title:"+id), &child.page.Title).Label("Leaf 标题").Placeholder("新 Leaf").PlaceholderColor(muted).FontSize(18).Bold().Height(32).Grow(1).MinWidth(0).FocusRing(false)
				if v.focusTitle {
					title.Focus()
					v.focusTitle = false
				}
				if a.focusTitleEndPage == child.page.ID {
					n := len([]rune(child.page.Title))
					title.Focus().SetTextSelection(n, n)
					a.focusTitleEndPage = ""
				}
				title.OnTextKey(func(k ui.TextKey) bool { return a.undoTitleBoundary(child, k) })
				if title.Focused() {
					a.editorDraft = child
					a.editorHandle = child.editor
				}
				if title.Changed() {
					a.changed(child)
				}
				if title.Submitted() {
					v.collapsed = false
					d.layoutRevision++
					a.editorHandle = child.editor
					a.enterBodyFromTitle(child)
					v.focusBody = true
				}
			})
			if v.collapsed {
				excerpt := leafExcerpt(child.page.Body)
				if excerpt == "" {
					excerpt = "尚未填写内容"
				}
				// A fixed three-line preview keeps a collapsed Leaf visibly substantial.
				ui.Text(c, excerpt).FontSize(14).TextColor(ui.Hex("#787773")).Padding(0, 18, 12, 48).Grow(1).Clip()
				return
			}
			area := ui.Row(c.Key("leaf-content:"+id)).Width(r.W).AlignItems(ui.Stretch).Grow(1).MinHeight(0).Padding(0, 16, 12, 0).Clip()
			x, y, over := area.PointerPosition()
			area.Children(func() {
				gutter := ui.Box(c).Width(gutterWidth)
				editor := richtext.EditorBase(c.Key("leaf-body:"+id), child.doc, richtext.EditorOptions{
					OnAttachmentClick: func(start int, at *richtext.Attachment) bool { return a.clickMath(child, start, at) },
					AttachmentSize: func(at *richtext.Attachment, w float32) (float32, float32) {
						if _, ok := mathSource(at); ok && !isInlineMath(at) {
							return w, at.Height
						}
						return 0, 0
					},
					ReadOnly: locked, FitContent: true, Label: "Leaf 正文", Placeholder: "写下这段知识…", Font: notebookFont, Theme: &notebookTheme,
					CodeHeaderHeight: 32, ParagraphTrailingSpace: timedTaskTrailingSpace,
					OnTyped:     func(s string) bool { return a.typedMath(child, s) },
					BeforePaste: func(data transfer.Data) bool { return a.pasteMath(child, data) || a.pasteWebURL(child, data) },
					BeforeKey: func(key ui.Key, mods ui.Modifiers) bool {
						if locked {
							return false
						}
						if a.mathEnter(child, key, mods) || a.urlPasteKey(child, key, mods) {
							return true
						}
						return a.backspaceToTitle(child, key, mods)
					},
					OnTaskToggle: func(start int, at time.Time) bool { return a.toggleTimedTask(child, start, at) },
					OnPasteText:  a.pasteReference, OnPasteRichText: a.leafPasteText, TagStyle: referenceSpanStyle, DrawAttachment: a.drawPageReferenceIcon,
					OnPasteImage: func() bool { return a.nativePasteImage(child, services) },
					OnLink: func(link string) {
						if !a.openReference(link) && ui.SafeToOpen(link) {
							services.OpenURL(link)
						}
					}, OnLinkCommand: func() {
						if !locked {
							a.openLink()
						}
					},
				}).Grow(1).MinWidth(0).FocusRing(false).Bind(&child.editor)
				if v.focusBody {
					editor.Focus()
					v.focusBody = false
				}
				if editor.Focused() && !locked {
					a.editorDraft = child
					a.editorHandle = child.editor
				}
				if locked {
					return
				}
				active := child.gutter.update(child.doc, editor.Focused(), x, y, over)
				gutter.Children(func() { a.blockGutter(c, child, y, over, active) })
				a.blockReferenceHighlight(c, child, editor)
				a.urlPasteView(c, child, editor)
				a.linkHoverView(c, child, editor)
				a.attachmentControls(c, child)
				a.mathControls(c, child, editor)
				a.codeControls(c, child)
				a.timedTaskControls(c, child, editor)
				_, caret := child.doc.Selection()
				if editor.Focused() && (caret != v.caret || child.doc.Version() != v.version) {
					v.reveal = true
				}
				v.caret, v.version = caret, child.doc.Version()
				if editor.Focused() || a.slash != nil && a.slash.Page == child.page.ID {
					a.syncSlash(child, editor)
					a.slashKeys(child, editor)
				}
				if a.slash != nil && a.slash.Page == child.page.ID {
					for _, p := range child.doc.VisibleParagraphs() {
						if p.Start == a.slash.ParagraphStart {
							anchor := ui.Box(c.Key("leaf-slash")).Absolute().Left(gutterWidth).Top(max(0, p.Top+p.Height)).Size(1, 1)
							a.slashMenu(c, child, anchor)
							break
						}
					}
				}
				start, end := child.doc.Selection()
				if a.editorDraft == child && start != end && a.slash == nil && !a.linkOpen && (a.taskEstimate == nil || !a.taskEstimate.open) && (editor.Focused() || a.formatHandle.FocusWithin(c)) {
					anchor := ui.Box(c).Absolute().Left(gutterWidth).Top(0).Size(1, 1)
					ui.Overlay(c, func() {
						ui.Row(c.Key("leaf-format")).Bind(&a.formatHandle).AttachTo(anchor, ui.AnchorTopStart, ui.AnchorBottomStart).Padding(4).Gap(2).Background(paper).Border(1, line).Radius(6).Children(func() { a.toolbar(c, child) })
					})
				}
			})
			next := max(float32(180), child.doc.ContentHeight()+72)
			if next != v.height {
				v.height = next
				d.layoutRevision++
				c.After(0)
			} else if v.reveal {
				if v.centerReveal {
					boxes := child.doc.VisibleParagraphs()
					if len(boxes) == 0 {
						c.After(0)
					} else {
						viewport := d.doc.ViewportHeight()
						_, caretPos := child.doc.Selection()
						paraStart, _ := child.doc.Text().ParagraphRange(caretPos)
						var centerY float32
						found := false
						for _, box := range boxes {
							if box.Start == paraStart {
								centerY = r.Y + 52 + box.Top + min(box.Height, viewport)/2
								found = true
								break
							}
						}
						if !found {
							caret := child.doc.CaretRect()
							centerY = r.Y + 52 + caret.Y + caret.H/2
						}
						d.doc.ScrollBy(centerY - viewport/2)
						v.reveal = false
						v.centerReveal = false
					}
				} else {
					caret := child.doc.CaretRect()
					y := r.Y + 52 + caret.Y
					viewport := d.doc.ViewportHeight()
					if y+caret.H > viewport-16 {
						d.doc.ScrollBy(y + caret.H - viewport + 16)
					} else if y < 8 {
						d.doc.ScrollBy(y - 8)
					}
					v.reveal = false
				}
			}
			a.changed(child)
		})
	}
}

// A pasted Leaf inside a Leaf becomes its readable content, not a hidden
// nested knowledge unit. The normal editor keeps rich formatting intact.
func (a *app) leafPasteText(text richtext.Text) richtext.Text {
	var paras []richtext.Paragraph
	for _, p := range text.Paragraphs() {
		for i, r := range p.Runs {
			if id := leafID(r.Attributes.Attachment); id != "" {
				for _, page := range a.pages {
					if leaf, ok := page.Body.Leaves[id]; ok {
						p.Runs[i] = richtext.Run{Text: leaf.Title + "\n" + leaf.Content.PlainText()}
						break
					}
				}
			}
		}
		paras = append(paras, p)
	}
	return hydrateMath(pasteReferenceStyles(richtext.TextOf(paras...)))
}

func cloneLeaf(leaf Leaf) Leaf {
	data, _ := json.Marshal(leaf)
	var copy Leaf
	_ = json.Unmarshal(data, &copy)
	copy.Content.Text = copy.Content.Text.FormatParagraphs(0, copy.Content.Text.Len(), func(st *richtext.ParagraphStyle) { st.ID = newID(); st.Timer = st.Timer.FreshCopy() })
	copy.Content.TaskTimes = nil
	return copy
}

func commandsForDraft(d *draft, query string) []blockCommand {
	items := matchingCommands(query)
	if d.parent == nil {
		return items
	}
	out := make([]blockCommand, 0, len(items))
	for _, cmd := range items {
		if cmd.ID != "leaf" && cmd.ID != "page" {
			out = append(out, cmd)
		}
	}
	return out
}

func identifyBody(b *Body) {
	if len(b.Leaves) > 0 {
		b.Schema = 2
	}
	b.Text = identifyStoredText(b.Text)
	for id, leaf := range b.Leaves {
		identifyBody(&leaf.Content)
		b.Leaves[id] = leaf
	}
}

// Visit only live knowledge units; deleted anchors can remain in the undo
// cache without polluting exports or search.
func (b Body) attachments() []*richtext.Attachment {
	var result []*richtext.Attachment
	for _, r := range b.Text.Runs(0, b.Text.Len()) {
		if at := r.Attributes.Attachment; at != nil {
			result = append(result, at)
			if id := leafID(at); id != "" {
				result = append(result, b.Leaves[id].Content.attachments()...)
			}
		}
	}
	return result
}

func remapBodyAssets(body *Body, assets map[string]string, mappings ...map[string]string) error {
	leafIDs := map[string]string{}
	for id := range body.Leaves {
		leafIDs[id] = newID()
		if len(mappings) > 0 {
			mappings[0][id] = leafIDs[id]
		}
	}
	var paras []richtext.Paragraph
	for _, p := range body.Text.Paragraphs() {
		for i, r := range p.Runs {
			if id := leafID(r.Attributes.Attachment); id != "" {
				if leafIDs[id] == "" {
					return fmt.Errorf("备份缺少 Leaf %s", id)
				}
				p.Runs[i].Attributes.Attachment = leafAttachment(leafIDs[id])
			}
			if at := r.Attributes.Attachment; at != nil && strings.HasPrefix(at.URL, "asset:") {
				name := strings.TrimPrefix(at.URL, "asset:")
				next, ok := assets[name]
				if !ok {
					return fmt.Errorf("备份缺少附件 %s", name)
				}
				copy := *at
				copy.URL = "asset:" + next
				p.Runs[i].Attributes.Attachment = &copy
			}
		}
		paras = append(paras, p)
	}
	body.Text = richtext.TextOf(paras...)
	leaves := map[string]Leaf{}
	for id, leaf := range body.Leaves {
		if err := remapBodyAssets(&leaf.Content, assets, mappings...); err != nil {
			return err
		}
		leaves[leafIDs[id]] = leaf
	}
	body.Leaves = leaves
	return nil
}

// Clone at the boundary of the native paste/duplicate transaction, so one
// undo removes the entire copy rather than merely rewinding its identity.
func (a *app) pasteLeaves(d *draft, text richtext.Text) richtext.Text {
	var paras []richtext.Paragraph
	for _, p := range text.Paragraphs() {
		for i, r := range p.Runs {
			id := leafID(r.Attributes.Attachment)
			if id == "" {
				continue
			}
			var source Leaf
			found := false
			for _, page := range a.pages {
				if value, ok := page.Body.Leaves[id]; ok {
					if owner := a.drafts[page.ID]; owner != nil {
						if v := owner.leafViews[id]; v != nil {
							a.syncLeaf(v.draft)
							value = owner.page.Body.Leaves[id]
						}
					}
					source, found = value, true
					break
				}
			}
			if !found { // Keep the unresolved reference explicit instead of dropping it.
				p.Runs[i] = richtext.Run{Text: "[Leaf 内容不可用]"}
				continue
			}
			if d.page.Body.Leaves == nil {
				d.page.Body.Leaves = map[string]Leaf{}
			}
			next := newID()
			d.page.Body.Leaves[next] = cloneLeaf(source)
			p.Runs[i].Attributes.Attachment = leafAttachment(next)
		}
		paras = append(paras, p)
	}
	return hydrateMath(pasteReferenceStyles(richtext.TextOf(paras...)))
}

func (a *app) editLeafAwareBlock(d *draft, index int, action string) {
	start, ok := paragraphStart(d.doc.Text(), index)
	if !ok {
		return
	}
	_, end := d.doc.Text().ParagraphRange(start)
	hasLeaf := false
	for _, r := range d.doc.Text().Runs(start, end) {
		if leafID(r.Attributes.Attachment) != "" {
			hasLeaf = true
			break
		}
	}
	if !hasLeaf {
		editBlock(d.doc, index, action)
		return
	}
	editBlockTransform(d.doc, index, action, func(p *richtext.Paragraph) {
		copy := a.pasteLeaves(d, richtext.TextOf(*p))
		for _, para := range copy.Paragraphs() {
			p.Runs = para.Runs
			break
		}
	})
}

// A standalone Leaf is an entire knowledge unit, not a line of editable
// parent text. Both positions beside its attachment share this paragraph.
func leafBoundaryCaret(text richtext.Text, caret int) bool {
	start, end := text.ParagraphRange(caret)
	return end-start == 1 && leafID(text.AttributesAt(start).Attachment) != ""
}
