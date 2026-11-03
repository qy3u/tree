package main

import (
	"fmt"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

var notebookFont = ui.Font{Size: 16, LineHeight: 1.75, Antialiased: true}
var notebookTheme = richtext.Theme{
	Text: ink, Muted: ui.Hex("#969590"), Caret: ink,
	Link: ui.Hex("#337ea9"), Checkbox: ui.Hex("#2e87dc"),
	ParagraphSpacing: .25, Indent: 1.9,
	Headings: [6]richtext.HeadingStyle{
		{Size: 1.875, Weight: 700, Top: .9, Bottom: .25},
		{Size: 1.5, Weight: 700, Top: 1, Bottom: .25},
		{Size: 1.25, Weight: 700, Top: 1, Bottom: .25},
	},
}

func (a *app) documentView(c *ui.Context, d *draft) {
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
				CodeHeaderHeight: 32,
				OnPasteText:      a.pasteReference,
				OnPasteRichText:  pasteReferenceStyles,
				TagStyle:         referenceSpanStyle,
				DrawAttachment:   a.drawPageReferenceIcon,
				OnLink: func(link string) {
					if !a.openReference(link) && ui.SafeToOpen(link) {
						c.OpenURL(link)
					}
				},
				LeadingSpace: top + 76 + pageIconSpace(d.page) + float32(len(sortedChildren(a.pages, d.page.ID)))*32,
				Label:        "笔记正文", Placeholder: notebookPlaceholder(d.doc.Text()), Font: notebookFont, Theme: &notebookTheme, OnLinkCommand: a.openLink,
			}).Grow(1).MinWidth(0).FocusRing(false).Bind(&a.editorHandle)
			activeStart := d.gutter.update(d.doc, editor.Focused(), pointerX, pointerY, over)
			gutter.Children(func() {
				a.blockGutter(c, d, pointerY, over, activeStart)
				a.childPageGutter(c, d, top+76+pageIconSpace(d.page)-d.doc.ScrollOffset(), pointerY, over)
			})
			// Keep native task hit testing, selection and undo; only replace the
			// rounded marker paint, since MyGo has no task-corner theme option.
			a.blockReferenceHighlight(c, d, editor)
			a.attachmentControls(c, d)
			a.codeControls(c, d)
			scroll := d.doc.ScrollOffset()
			ui.Column(c.Key("page-heading:" + d.page.ID)).Absolute().Left(gutterWidth).Right(0).Top(top - scroll).Gap(20).Children(func() {
				a.pageIconControl(c, d)
				title := ui.TextInputBase(c.Key("title:"+d.page.ID), &d.page.Title).Label("页面标题").Placeholder("新页面").PlaceholderColor(ui.Hex("#e1e1e0")).FontSize(40).Bold().Height(56).FocusRing(false).Bind(&a.titleHandle)
				if a.focusTitlePage == d.page.ID {
					title.Focus()
					a.focusTitlePage = ""
				}
				if title.Changed() {
					a.changed(d)
				}
				if title.Submitted() {
					d.doc.SetSelection(0, 0)
					a.editorHandle.Focus()
				}
				if len(sortedChildren(a.pages, d.page.ID)) > 0 {
					ui.Column(c).Children(func() { a.childPages(c, d.page.ID) })
				}
			})
			a.syncSlash(d, editor)
			a.slashKeys(d, editor)
			if a.slash != nil {
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
			if start != end && a.slash == nil && !a.insertOpen && !a.linkOpen && (editor.Focused() || a.formatHandle.FocusWithin(c)) {
				for _, box := range d.doc.VisibleParagraphs() {
					_, paraEnd := d.doc.Text().ParagraphRange(box.Index)
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
		menuHere := a.blockMenu && a.blockIndex == box.Index
		if menuHere {
			visible = true
		}
		// Keep the drag source mounted while the pointer travels to another
		// paragraph. Only its visible/focusable controls follow the hover.
		handle := ui.Box(c.Key(key)).KeepFocus().Absolute().Left(12).Top(box.FirstLineCenter-13).Size(24, 26).Padding(4).AlignItems(ui.Center).Justify(ui.Center).Radius(4).Drag(paragraphDrag{d.page.ID, box.Index})
		if menuHere || handle.Dragging() || box.Start == activeStart || (activeStart < 0 && over && pointerY >= box.Top && pointerY < box.Top+max(28, box.Height)) {
			handle.Label(fmt.Sprintf("移动段落 %d", box.Index+1)).Role(ui.RoleButton).Focusable().Tooltip("拖动调整顺序，点击打开段落菜单").Children(func() {
				ui.Icon(c, icons["grip"]).FontSize(16).TextColor(muted)
			})
			if handle.Hovered() {
				handle.Background(line)
			}
		}
		if drag, ok := ui.Drop[paragraphDrag](handle); ok && drag.Page == d.page.ID {
			moveParagraph(d.doc, drag.Index, box.Index)
		}
		if handle.Clicked() {
			a.blockIndex, a.blockMenu = box.Index, !menuHere
			d.doc.SetSelection(box.Start, box.Start)
			visible = true
		}
		if a.blockMenu && a.blockIndex == box.Index {
			ui.Popover(c, handle, &a.blockMenu, func() {
				a.blockActions(c, d, box.Index)
				if quietButton(c, "段落上移").Clicked() {
					a.moveCurrent(-1)
					a.blockMenu = false
					a.editorHandle.Focus()
				}
				if quietButton(c, "段落下移").Clicked() {
					a.moveCurrent(1)
					a.blockMenu = false
					a.editorHandle.Focus()
				}
			})
		}
	}
	if !visible {
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
			box := ui.Rect{X: x, Y: y, W: size, H: size}
			if st.Checked {
				p.Fill(box, notebookTheme.Checkbox, 1)
				var check ui.Path
				check.MoveTo(x+size*.23, y+size*.51).LineTo(x+size*.43, y+size*.72).LineTo(x+size*.78, y+size*.28)
				p.StrokePath(&check, 1.6, paper)
			} else {
				p.Stroke(box, ink, 1, 1.35)
			}
		}
	})
}
