package main

import (
	"fmt"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
	"strings"
)

func (a *app) replaceAttachment(d *draft, start int, at *richtext.Attachment) {
	anchor, caret := d.doc.Selection()
	d.doc.Edit(func(e *richtext.Edit) {
		e.Replace(start, start+1, richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: at}}}}))
		e.Select(anchor, caret)
	})
	a.changed(d)
}
func (a *app) updateInlineTable(d *draft, start int, t Table) {
	id := newID()
	d.page.Body.Tables[id] = copyTable(t)
	a.replaceAttachment(d, start, tableAttachment(id, t))
}
func (a *app) nativePasteImage(d *draft, services ui.Services) bool {
	// The window clipboard service also normalizes macOS TIFF screenshots to PNG.
	clipboard := services.ReadClipboardData(transfer.PNG)
	if len(clipboard.Formats()) == 0 {
		return false
	}
	data, err := clipboard.Read(transfer.PNG)
	if err != nil {
		a.report(fmt.Errorf("无法读取剪贴板图片：%w", err))
		return true
	}
	if len(data) == 0 {
		return false
	}
	at, err := a.store.ImportImage(data, "粘贴的图片.png")
	if err != nil {
		a.report(err)
		return true
	}
	// The paste belongs to this exact editor, regardless of the last focused Leaf.
	a.editorDraft = d
	a.editorHandle = d.editor
	a.urlPaste = nil
	a.insertAttachment(at)
	a.dismissSlash()
	return true
}

func (a *app) attachmentControls(c *ui.Context, d *draft) {
	for _, box := range d.doc.VisibleAttachments() {
		at := box.Attachment
		r := box.Rect
		key := fmt.Sprintf("attachment:%s:%d", d.page.ID, box.Start)
		if strings.HasPrefix(at.URL, "table:") {
			table, ok := d.page.Body.Tables[strings.TrimPrefix(at.URL, "table:")]
			if !ok || len(table.Cells) == 0 {
				continue
			}
			rh := (r.H - 28) / float32(len(table.Cells))
			ui.Column(c.Key(key)).Absolute().Left(gutterWidth + r.X).Top(r.Y).Width(r.W).Height(r.H).Background(paper).Children(func() {
				for i, row := range table.Cells {
					ui.Row(c.Key(fmt.Sprint(i))).Height(rh).Children(func() {
						for j, cell := range row {
							value := cell
							input := ui.TextInputBase(c.Key(fmt.Sprint(j)), &value).Label(fmt.Sprintf("表格 %d 第 %d 行第 %d 列", box.Start, i+1, j+1)).Grow(1).MinWidth(0).Height(rh).Padding(5, 8).FontSize(13).Border(1, line)
							if i == 0 {
								input.Background(sidebarColor)
							}
							if input.Changed() {
								t := copyTable(table)
								t.Cells[i][j] = value
								a.updateInlineTable(d, box.Start, t)
							}
							if input.Shortcut(ui.Cmd, ui.KeyZ) {
								d.doc.Undo()
								a.changed(d)
							}
							if input.Shortcut(ui.Cmd|ui.Shift, ui.KeyZ) {
								d.doc.Redo()
								a.changed(d)
							}
						}
					})
				}
				ui.Row(c).Height(28).Gap(3).Children(func() {
					change := func(label string, disabled bool, fn func(*Table)) {
						if quietButton(c, label).FontSize(11).Padding(3, 6).Disabled(disabled).Clicked() {
							t := copyTable(table)
							fn(&t)
							a.updateInlineTable(d, box.Start, t)
						}
					}
					change("＋ 行", len(table.Cells) >= 30, func(t *Table) { t.Cells = append(t.Cells, make([]string, len(t.Cells[0]))) })
					change("＋ 列", len(table.Cells[0]) >= 6, func(t *Table) {
						for i := range t.Cells {
							t.Cells[i] = append(t.Cells[i], "")
						}
					})
					change("− 行", len(table.Cells) <= 1, func(t *Table) { t.Cells = t.Cells[:len(t.Cells)-1] })
					change("− 列", len(table.Cells[0]) <= 1, func(t *Table) {
						for i := range t.Cells {
							t.Cells[i] = t.Cells[i][:len(t.Cells[i])-1]
						}
					})
				})
			})
		} else if strings.HasPrefix(at.URL, "asset:") {
			target := ui.Box(c.Key(key)).Absolute().Left(gutterWidth+r.X).Top(r.Y).Size(r.W, r.H)
			target.ContextMenu(func(m *ui.Menu) {
				for _, width := range []float32{240, 400, 560} {
					if m.Item(fmt.Sprintf("图片宽度 %.0f", width)).Chosen() {
						copy := *at
						copy.Height = at.Height * width / at.Width
						copy.Width = width
						a.replaceAttachment(d, box.Start, &copy)
					}
				}
				if m.Item("删除图片").Chosen() {
					d.doc.SetSelection(box.Start, box.Start+1)
					d.doc.InsertText("")
					a.changed(d)
				}
			})
			if target.Clicked() {
				d.doc.SetSelection(box.Start, box.Start+1)
				a.editorHandle.Focus()
			}
			start, end := d.doc.Selection()
			if min(start, end) == box.Start && max(start, end) == box.Start+1 {
				target.Border(2, ui.Hex("#2e87dc"))
			}
		}
	}
}
