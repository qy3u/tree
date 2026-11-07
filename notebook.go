package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

var notebookFont = ui.Font{Size: 16, LineHeight: 1.75, Antialiased: true}
var notebookTheme = richtext.Theme{
	Text: ink, Muted: ui.Hex("#969590"), Caret: ink,
	Link: ui.Hex("#787774"), Checkbox: ui.Hex("#2e87dc"),
	ParagraphSpacing: .25, Indent: 1.9,
	Headings: [6]richtext.HeadingStyle{
		{Size: 1.875, Weight: 700, Top: .9, Bottom: .25},
		{Size: 1.5, Weight: 700, Top: 1, Bottom: .25},
		{Size: 1.25, Weight: 700, Top: 1, Bottom: .25},
	},
}

func (a *app) documentView(c *ui.Context, d *draft) {
	// Link callbacks run between frames; retain window services, not the frame context.
	services := c.Services()
	w, h := c.Size()
	mainWidth := w - sidebarWidth - 1
	width := min(float32(720), mainWidth-96)
	margin := (mainWidth - width) / 2
	top := min(float32(112), max(float32(56), h-620))
	ui.Column(c).Grow(1).MinHeight(0).Padding(0, margin, 0, margin-gutterWidth).Children(func() {
		row := ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Clip()
		row.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputScroll {
				return d.doc.ScrollBy(ev.DY)
			}
			return false
		})
		pointerX, pointerY, over := row.PointerPosition()
		row.Children(func() {
			gutter := ui.Box(c).Width(gutterWidth).Clip()
			editor := richtext.EditorBase(c.Key("body:"+d.page.ID), d.doc, richtext.EditorOptions{
				OnTyped:      func(s string) bool { return a.typedMath(d, s) },
				BeforePaste:  func(data transfer.Data) bool { return a.pasteMath(d, data) || a.pasteWebURL(d, data) },
				OnPasteImage: func() bool { return a.nativePasteImage(d, services) },
				BeforeKey: func(key ui.Key, mods ui.Modifiers) bool {
					if a.mathEnter(d, key, mods) || a.urlPasteKey(d, key, mods) {
						return true
					}
					return a.backspaceToTitle(d, key, mods)
				},
				HideCaretAt: leafBoundaryCaret,
				AttachmentSize: func(at *richtext.Attachment, w float32) (float32, float32) {
					if _, ok := mathSource(at); ok && !isInlineMath(at) {
						return w, at.Height
					}
					return a.leafSize(d, at, w)
				}, LayoutRevision: d.layoutRevision,
				CodeHeaderHeight:       32,
				ParagraphTrailingSpace: timedTaskTrailingSpace,
				OnTaskToggle:           func(start int, at time.Time) bool { return a.toggleTimedTask(d, start, at) },
				OnPasteText:            a.pasteReference,
				OnPasteRichText:        func(text richtext.Text) richtext.Text { return a.pasteLeaves(d, text) },
				TagStyle:               referenceSpanStyle,
				DrawAttachment:         a.drawPageReferenceIcon,
				OnLink: func(link string) {
					if !a.openReference(link) && ui.SafeToOpen(link) {
						services.OpenURL(link)
					}
				},
				LeadingSpace: top + 76 + pageIconSpace(d.page) + float32(len(sortedChildren(a.pages, d.page.ID)))*32,
				Label:        "笔记正文", Placeholder: notebookPlaceholder(d.doc.Text()), Font: notebookFont, Theme: &notebookTheme, OnLinkCommand: a.openLink,
			}).Grow(1).MinWidth(0).FocusRing(false).Bind(&d.editor)
			if editor.Focused() || a.editorDraft == nil {
				a.editorDraft = d
				a.editorHandle = d.editor
			}
			activeStart := d.gutter.update(d.doc, editor.Focused(), pointerX, pointerY, over)
			gutter.Children(func() {
				a.blockGutter(c, d, pointerY, over, activeStart)
				a.childPageGutter(c, d, top+76+pageIconSpace(d.page)-d.doc.ScrollOffset(), pointerY, over)
			})
			// Keep native task hit testing, selection and undo; only replace the
			// rounded marker paint, since MyGo has no task-corner theme option.
			a.blockReferenceHighlight(c, d, editor)
			a.urlPasteView(c, d, editor)
			a.linkHoverView(c, d, editor)
			a.attachmentControls(c, d)
			a.mathControls(c, d, editor)
			a.codeControls(c, d)
			a.timedTaskControls(c, d)
			a.leafControls(c, d)
			scroll := d.doc.ScrollOffset()
			ui.Column(c.Key("page-heading:" + d.page.ID)).Absolute().Left(gutterWidth).Right(0).Top(top - scroll).Gap(20).Children(func() {
				a.pageIconControl(c, d)
				title := ui.TextInputBase(c.Key("title:"+d.page.ID), &d.page.Title).Label("页面标题").Placeholder("新页面").PlaceholderColor(ui.Hex("#e1e1e0")).FontSize(40).Bold().Height(56).FocusRing(false).Bind(&a.titleHandle)
				title.OnTextKey(func(k ui.TextKey) bool { return a.undoTitleBoundary(d, k) })
				if a.focusTitleEndPage == d.page.ID {
					end := len([]rune(d.page.Title))
					title.Focus().SetTextSelection(end, end)
					a.focusTitleEndPage = ""
				}
				if a.focusTitlePage == d.page.ID {
					title.Focus()
					a.focusTitlePage = ""
				}
				if title.Changed() {
					a.titleBoundaryUndo = titleBoundaryUndo{}
					a.changed(d)
				}
				if title.Focused() {
					a.editorDraft = d
					a.editorHandle = d.editor
				}
				if title.Submitted() {
					a.enterBodyFromTitle(d)
				}
				if len(sortedChildren(a.pages, d.page.ID)) > 0 {
					ui.Column(c).Children(func() { a.childPages(c, d.page.ID) })
				}
			})
			if editor.Focused() || a.slash != nil && a.slash.Page == d.page.ID {
				a.syncSlash(d, editor)
				a.slashKeys(d, editor)
			}
			if a.slash != nil && a.slash.Page == d.page.ID {
				visible := false
				for _, box := range d.doc.VisibleParagraphs() {
					if box.Start == a.slash.ParagraphStart {
						anchor := ui.Box(c.Key("slash-anchor")).Absolute().Left(gutterWidth).Top(max(0, box.Top+box.Height)).Size(1, 1)
						a.slashMenu(c, d, anchor)
						visible = true
						break
					}
				}
				if !visible {
					a.dismissSlash()
				}
			}
			start, end := d.doc.Selection()
			if a.editorDraft == d && start != end && a.slash == nil && !a.insertOpen && !a.linkOpen && (editor.Focused() || a.formatHandle.FocusWithin(c)) {
				for _, box := range d.doc.VisibleParagraphs() {
					_, paraEnd := d.doc.Text().ParagraphRange(box.Start)
					if paraEnd < min(start, end) || box.Top < 0 {
						continue
					}
					anchor := ui.Box(c.Key("format-anchor")).Absolute().Left(gutterWidth).Top(box.Top).Size(1, 1)
					ui.Overlay(c, func() {
						ui.Row(c.Key("selection-format")).Label("文字格式").Bind(&a.formatHandle).AttachTo(anchor, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(8, 0, 8, 0).Padding(4).Gap(2).AlignItems(ui.Center).Background(paper).Border(1, line).Radius(6).Shadow(0, 3, 12, 0, ui.RGBA(0, 0, 0, .12)).Children(func() { a.toolbar(c, d) })
					})
					break
				}
			}
		})
	})
}

func (a *app) enterBodyFromTitle(d *draft) {
	a.titleBoundaryUndo = titleBoundaryUndo{}
	text := d.doc.Text()
	_, end := text.ParagraphRange(0)
	style := text.ParagraphStyleAt(0)
	style.ID = ""
	if end > 0 || style != (richtext.ParagraphStyle{}) {
		// Splitting at offset zero keeps the original block's ID and content
		// together. Only the inserted empty paragraph gets a fresh plain style.
		d.doc.Edit(func(e *richtext.Edit) {
			e.ReplaceString(0, 0, "\n", richtext.Attributes{})
			e.FormatParagraphs(0, 0, func(st *richtext.ParagraphStyle) {
				*st = richtext.ParagraphStyle{ID: newID()}
			})
			e.Select(0, 0)
		})
		a.changed(d)
	} else {
		d.doc.SetSelection(0, 0)
	}
	a.editorHandle.Focus()
}

func notebookPlaceholder(text richtext.Text) string {
	// MyGo paints its document placeholder at the editor origin, without
	// paragraph insets. Only show the generic hint for an empty plain block;
	// list markers, quotations and code blocks already provide their context.
	st := text.ParagraphStyleAt(0)
	if text.Len() != 0 || st.List != richtext.NoList || st.Indent != 0 || st.Quote || st.CodeBlock || st.Heading != 0 {
		return ""
	}
	return "输入文字，或输入 / 插入内容…"
}

func (a *app) blockGutter(c *ui.Context, d *draft, pointerY float32, over bool, activeStart int) {
	visible := false
	for _, box := range d.doc.VisibleParagraphs() {
		key := fmt.Sprintf("gutter:%s:%d", d.page.ID, box.Index)
		target := ui.Box(c.Key(key + ":drop")).Absolute().Top(box.Top).Width(gutterWidth).Height(max(28, box.Height))
		if drag, hover := ui.DragOver[paragraphDrag](target); hover && drag.Page == d.page.ID {
			target.Background(line)
		}
		if drag, ok := ui.Drop[paragraphDrag](target); ok && drag.Page == d.page.ID {
			moveParagraph(d.doc, drag.Index, box.Index)
		}
		blockID := d.doc.Text().ParagraphStyleAt(box.Start).ID
		menuHere := a.blockMenu && a.blockMenuPage == d.page.ID && a.blockID == blockID
		if menuHere {
			visible = true
		}
		// Keep the drag source mounted while the pointer travels to another
		// paragraph. Only its visible/focusable controls follow the hover.
		center := box.FirstLineCenter
		for _, at := range d.doc.VisibleAttachments() {
			if at.Start == box.Start && leafID(at.Attachment) != "" {
				center = at.Rect.Y + 26
				break
			}
		}
		handle := ui.Box(c.Key(key)).KeepFocus().Absolute().Left(12).Top(center-13).Size(24, 26).Padding(2).AlignItems(ui.Center).Justify(ui.Center).Radius(4).Drag(paragraphDrag{d.page.ID, box.Index})
		if menuHere || handle.Dragging() || box.Start == activeStart || (activeStart < 0 && over && pointerY >= box.Top && pointerY < box.Top+max(28, box.Height)) {
			handle.Label(fmt.Sprintf("移动段落 %d", box.Index+1)).Role(ui.RoleButton).Focusable().Tooltip("拖动调整顺序，点击打开段落菜单").Children(func() {
				ui.Icon(c, icons["grip"]).FontSize(20).TextColor(muted)
			})
			if handle.Hovered() {
				handle.Background(line)
			}
		}
		if drag, ok := ui.Drop[paragraphDrag](handle); ok && drag.Page == d.page.ID {
			moveParagraph(d.doc, drag.Index, box.Index)
		}
		if handle.Clicked() {
			a.editorDraft = d
			a.editorHandle = d.editor
			a.blockID, a.blockMenu = blockID, !menuHere
			a.blockMenuPage = d.page.ID
			d.doc.SetSelection(box.Start, box.Start)
			visible = true
		}
		if a.blockMenu && a.blockMenuPage == d.page.ID && a.blockID == blockID {
			a.blockMenuPanel(c, handle, d, box.Index)
		}
	}
	// Nested Leaf editors must not dismiss a menu owned by their page.
	if !visible && a.blockMenuPage == d.page.ID {
		a.blockMenu = false
	}
}

func paintTaskBoxes(p *ui.Painter, r ui.Rect, doc *richtext.Document) {
	p.Clip(r, 0, func() {
		text := doc.Text()
		indent := notebookFont.Size * notebookTheme.Indent
		for _, para := range doc.VisibleParagraphs() {
			st := text.ParagraphStyleAt(para.Start)
			if st.List != richtext.Tasks || st.CodeBlock {
				continue
			}
			font := notebookFont
			if st.Heading > 0 {
				heading := notebookTheme.Headings[st.Heading-1]
				if heading.Size == 0 {
					heading = richtext.DefaultTheme(ui.LightTheme()).Headings[st.Heading-1]
				}
				font.Size *= heading.Size
				font.Weight = heading.Weight
			}
			first := ui.ShapeText("M", font, 0).CaretAt(ui.TextCaretPosition{})
			size := font.Size * .95
			x := r.X + float32(st.Indent)*indent + (indent-size)/2 - indent*.1
			if st.Quote {
				x += indent * .75
			}
			y := r.Y + para.Top + first.Y + (first.H-size)/2
			p.Fill(ui.Rect{X: x - 1, Y: y - 1, W: size + 2, H: size + 2}, paper, 0)
			// Erase the native marker before drawing a slightly larger square
			// closer to the text, keeping its vertical center unchanged.
			x += 1
			y -= (font.Size - size) / 2
			size = font.Size
			box := ui.Rect{X: x, Y: y, W: size, H: size}
			if st.Checked {
				p.Fill(box, notebookTheme.Checkbox, 1.5)
				var check ui.Path
				check.MoveTo(x+size*.23, y+size*.51).LineTo(x+size*.43, y+size*.72).LineTo(x+size*.78, y+size*.28)
				p.StrokePath(&check, 1.6, paper)
			} else {
				p.Stroke(box, ink, 1.5, 1.35)
			}
		}
	})
}
