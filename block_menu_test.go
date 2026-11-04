package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

var blockMenuLabels = []string{"在上方插入", "在下方插入", "复制块", "复制块链接", "转为文本", "转为一级标题", "转为二级标题", "转为三级标题", "转为项目列表", "转为编号列表", "转为待办列表", "转为引用", "转为代码块", "删除块", "段落上移", "段落下移"}

func menuParagraphs(text richtext.Text) []richtext.Paragraph {
	var result []richtext.Paragraph
	for _, p := range text.Paragraphs() {
		result = append(result, p)
	}
	return result
}

// Open with the mouse while the caret is in a different block. Moving down
// the popup must not redirect an operation to the block behind the menu.
func openBlockMenu(t *testing.T, a *app, tt *ui.Tester, index int) {
	t.Helper()
	doc := a.current().doc
	doc.SetSelection(doc.Text().Len(), doc.Text().Len())
	a.editorHandle.Focus()
	tt.Frame()
	ed, _ := tt.Find("笔记正文")
	found := false
	for _, box := range doc.VisibleParagraphs() {
		if box.Index == index {
			tt.Move(ed.X+50, ed.Y+box.FirstLineCenter)
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("block %d not visible", index)
	}
	tt.Frame()
	if err := tt.Click(fmt.Sprintf("移动段落 %d", index+1)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.blockMenu {
		t.Fatal("menu failed to open")
	}
}

func clickBlockMenuItem(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing menu item %s", label)
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

func TestBlockMenuAllActions(t *testing.T) {
	sources := []struct {
		name  string
		style richtext.ParagraphStyle
		text  string
	}{
		{"text", richtext.ParagraphStyle{}, "正文🌲 内容"},
		{"heading", richtext.ParagraphStyle{Heading: 2}, "2026-11-03"},
		{"todo", richtext.ParagraphStyle{List: richtext.Tasks, Checked: true}, "已完成任务"},
		{"quote", richtext.ParagraphStyle{Quote: true}, "引用 内容"},
		{"nested-todo", richtext.ParagraphStyle{List: richtext.Tasks, Checked: true, Indent: 2}, "嵌套待办"},
		{"code", richtext.ParagraphStyle{CodeBlock: true, CodeLanguage: "go"}, "func main() {\u2028\tprintln(1)\u2028}"},
		{"empty-heading", richtext.ParagraphStyle{Heading: 3}, ""},
	}
	for _, source := range sources {
		for _, label := range blockMenuLabels {
			for _, index := range []int{0, 1, 2} {
				t.Run(fmt.Sprintf("%s/%s/%d", source.name, label, index), func(t *testing.T) {
					a, tt := commandTestApp(t)
					d := a.current()
					input := []richtext.Paragraph{
						{Style: richtext.ParagraphStyle{ID: "first", Heading: 1}, Runs: []richtext.Run{{Text: "前面的长标题🌲"}}},
						{Style: richtext.ParagraphStyle{ID: "middle", Quote: true}, Runs: []richtext.Run{{Text: "中间的段落测试", Attributes: richtext.Attributes{Italic: true}}}},
						{Style: richtext.ParagraphStyle{ID: "last", List: richtext.Tasks, Checked: true}, Runs: []richtext.Run{{Text: "末段中文内容", Attributes: richtext.Attributes{Link: "https://example.com"}}}},
					}
					input[index] = richtext.Paragraph{Style: source.style, Runs: []richtext.Run{{Text: source.text, Attributes: richtext.Attributes{Weight: 700}}}}
					input[index].Style.ID = "target"
					d.doc.Insert(richtext.TextOf(input...))
					tt.Frame()
					before := d.doc.Text()
					expected := menuParagraphs(before)
					openBlockMenu(t, a, tt, index)
					clickBlockMenuItem(t, tt, label)
					boundary := (label == "段落上移" && index == 0) || (label == "段落下移" && index == 2)
					if boundary {
						if !d.doc.Text().Equal(before) {
							t.Fatal("boundary move changed content")
						}
						tt.Key(0, ui.KeyEscape)
						return
					}
					if a.blockMenu {
						t.Fatal("menu did not close")
					}
					if !tt.Focused("笔记正文") {
						t.Fatal("editor focus not restored")
					}
					selectedIndex := index
					got := menuParagraphs(d.doc.Text())
					switch label {
					case "复制块链接":
						if tt.Clipboard() != referenceURL(d.page.ID, expected[index].Style.ID) {
							t.Fatalf("wrong block link: %s", tt.Clipboard())
						}
					case "在上方插入", "在下方插入", "复制块":
						if label != "在上方插入" {
							selectedIndex++
						}
						added := richtext.Paragraph{}
						if label == "复制块" {
							added = expected[index]
						}
						added.Style.ID = got[selectedIndex].Style.ID
						if added.Style.ID == "" {
							t.Fatal("new block has no ID")
						}
						for _, p := range expected {
							if p.Style.ID == added.Style.ID {
								t.Fatal("new block reuses existing ID")
							}
						}
						expected = append(expected, richtext.Paragraph{})
						copy(expected[selectedIndex+1:], expected[selectedIndex:])
						expected[selectedIndex] = added
					case "删除块":
						expected = append(expected[:index], expected[index+1:]...)
						selectedIndex = min(index, len(expected)-1)
					case "段落上移", "段落下移":
						delta := 1
						if label == "段落上移" {
							delta = -1
						}
						selectedIndex = index + delta
						expected[index], expected[selectedIndex] = expected[selectedIndex], expected[index]
					default:
						found := false
						for _, cmd := range blockCommands {
							if label == "转为"+cmd.Label {
								old := expected[index].Style
								expected[index].Style = cmd.Style
								expected[index].Style.ID = old.ID
								expected[index].Style.Indent = old.Indent
								expected[index].Style.Align = old.Align
								if cmd.ID == "todo" && old.List == richtext.Tasks {
									expected[index].Style.Checked = old.Checked
								}
								if cmd.ID == "code" && old.CodeBlock {
									expected[index].Style.CodeLanguage = old.CodeLanguage
								}
								found = true
								break
							}
						}
						if !found {
							t.Fatal("unaccounted menu action")
						}
					}
					want := richtext.TextOf(expected...)
					if d.doc.Text().String() != want.String() || !reflect.DeepEqual(got, menuParagraphs(want)) {
						t.Fatalf("wrong block/content/style: got %s; want %s", d.doc.Text().Markdown(), want.Markdown())
					}
					pos := 0
					for i := 0; i < selectedIndex; i++ {
						pos += paragraphLength(expected[i]) + 1
					}
					anchor, caret := d.doc.Selection()
					if anchor != pos || caret != pos {
						t.Fatalf("caret (%d,%d), expected block start %d", anchor, caret, pos)
					}
					after := d.doc.Text()
					if !after.Equal(before) {
						tt.Key(ui.Cmd, ui.KeyZ)
						if !d.doc.Text().Equal(before) {
							t.Fatal("one undo did not restore exact content, styles and IDs")
						}
						tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
						if !d.doc.Text().Equal(after) {
							t.Fatal("one redo did not restore result")
						}
					}
					if err := a.flush(); err != nil {
						t.Fatal(err)
					}
					reopened, err := newApp(a.store)
					if err != nil {
						t.Fatal(err)
					}
					reopened.selectPage(d.page.ID)
					if !reflect.DeepEqual(menuParagraphs(reopened.current().doc.Text()), menuParagraphs(after)) {
						t.Fatal("result changed after save/reopen")
					}
				})
			}
		}
	}
}

func TestBlockMenuSmallWindowAndDismissal(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.SetSize(850, 540)
	tt.Type("上面的内容\n中间的内容\n下面的内容")
	before := a.current().doc.Text()
	openBlockMenu(t, a, tt, 1)
	panel, ok := tt.Find("段落操作菜单")
	if !ok || panel.Y < 0 || panel.Y+panel.H > 540 {
		t.Fatalf("menu outside window: %+v", panel)
	}
	list, _ := tt.Find("段落操作列表")
	tt.Scroll(list.X+list.W/2, list.Y+list.H/2, 0, 600)
	tt.Frame()
	clickBlockMenuItem(t, tt, "段落下移")
	if got := a.current().doc.Text().String(); got != "上面的内容\n下面的内容\n中间的内容" {
		t.Fatalf("scrolled action failed: %q", got)
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if !a.current().doc.Text().Equal(before) {
		t.Fatal("small-window undo failed")
	}
	openBlockMenu(t, a, tt, 0)
	tt.Key(0, ui.KeyEscape)
	if a.blockMenu {
		t.Fatal("Escape did not dismiss")
	}
	openBlockMenu(t, a, tt, 0)
	tt.ClickAt(820, 500)
	if a.blockMenu {
		t.Fatal("outside click did not dismiss")
	}
	if !a.current().doc.Text().Equal(before) {
		t.Fatal("dismissal changed text")
	}
}

func TestBlockMenuSingleEmptyBlock(t *testing.T) {
	for _, label := range []string{"删除块", "在上方插入", "在下方插入", "复制块"} {
		t.Run(label, func(t *testing.T) {
			a, tt := commandTestApp(t)
			// A task marker makes the otherwise empty block visible and interactive.
			tt.Type("/todo")
			tt.Key(0, ui.KeyEnter)
			before := a.current().doc.Text()
			openBlockMenu(t, a, tt, 0)
			clickBlockMenuItem(t, tt, label)
			doc := a.current().doc
			if label == "删除块" {
				if doc.Text().ParagraphCount() != 1 || doc.Text().String() != "" || doc.Text().ParagraphStyleAt(0).List != richtext.NoList {
					t.Fatal("delete did not leave editable plain block")
				}
			} else if doc.Text().ParagraphCount() != 2 {
				t.Fatal("empty block insertion/copy failed")
			}
			after := doc.Text()
			tt.Key(ui.Cmd, ui.KeyZ)
			if !doc.Text().Equal(before) {
				t.Fatal("undo failed")
			}
			tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
			if !doc.Text().Equal(after) {
				t.Fatal("redo failed")
			}
			tt.Type("继续输入")
			if !strings.Contains(doc.Text().String(), "继续输入") {
				t.Fatal("cannot type after operation")
			}
		})
	}
}

func TestBlockMenuKeepsTargetWhenSelectionOrOrderChanges(t *testing.T) {
	for _, label := range []string{"转为一级标题", "段落下移", "复制块链接", "删除块"} {
		t.Run(label, func(t *testing.T) {
			a, tt := commandTestApp(t)
			tt.Type("第一块很长的中文\n目标🌲\n最后一块")
			doc := a.current().doc
			targetID := menuParagraphs(doc.Text())[1].Style.ID
			openBlockMenu(t, a, tt, 1)
			// Editor changes while a popup is open must not retarget its actions.
			doc.SetSelection(0, 0)
			doc.InsertText("新插入的块\n")
			doc.SetSelection(0, 0)
			tt.Frame()
			clickBlockMenuItem(t, tt, label)
			paras := menuParagraphs(doc.Text())
			switch label {
			case "转为一级标题":
				if paras[2].Style.ID != targetID || paras[2].Style.Heading != 1 || paras[0].Style.Heading != 0 {
					t.Fatal("conversion followed caret/old index")
				}
			case "段落下移":
				if paras[3].Style.ID != targetID {
					t.Fatal("move followed caret/old index")
				}
			case "复制块链接":
				if tt.Clipboard() != referenceURL(a.selected, targetID) {
					t.Fatal("copied a different block link")
				}
			case "删除块":
				if _, found := blockPosition(doc.Text(), targetID); found || len(paras) != 3 || paras[2].Runs[0].Text != "最后一块" {
					t.Fatal("deleted the wrong block")
				}
			}
		})
	}
}

func TestBlockMenuInsertedParagraphAcceptsPlainText(t *testing.T) {
	for _, label := range []string{"在上方插入", "在下方插入"} {
		t.Run(label, func(t *testing.T) {
			a, tt := commandTestApp(t)
			tt.Type("前面的长段落\n/code")
			tt.Key(0, ui.KeyEnter)
			tt.Type("func test() {}")
			openBlockMenu(t, a, tt, 1)
			clickBlockMenuItem(t, tt, label)
			tt.Type("新的正文🌲")
			tt.Key(0, ui.KeyEnter)
			tt.Type("下一段")
			ps := menuParagraphs(a.current().doc.Text())
			if len(ps) != 4 {
				t.Fatalf("new text went into code block: %q", a.current().doc.Text().String())
			}
			idx := 1
			if label == "在下方插入" {
				idx = 2
			}
			if ps[idx].Runs[0].Text != "新的正文🌲" || ps[idx].Style.CodeBlock || ps[idx+1].Runs[0].Text != "下一段" {
				t.Fatal("insert did not focus new plain block")
			}
			codeIndex := 3
			if label == "在下方插入" {
				codeIndex = 1
			}
			if !ps[codeIndex].Style.CodeBlock || ps[codeIndex].Runs[0].Text != "func test() {}" {
				t.Fatal("original code changed")
			}
		})
	}
}

func TestBlockMenuAttachmentsAndReferences(t *testing.T) {
	for _, kind := range []string{"image", "table", "divider", "reference"} {
		for _, label := range []string{"在上方插入", "在下方插入", "复制块", "删除块", "段落上移", "段落下移", "复制块链接"} {
			t.Run(kind+"/"+label, func(t *testing.T) {
				a, tt := commandTestApp(t)
				d := a.current()
				target := richtext.Paragraph{Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement)}}}
				switch kind {
				case "image":
					target.Runs[0].Attributes.Attachment = &richtext.Attachment{URL: "asset:fixture.png", Name: "图片", Width: 120, Height: 40}
				case "table":
					table := Table{Cells: [][]string{{"列甲", "列乙"}, {"数据", "值"}}}
					d.page.Body.Tables["fixture"] = table
					target.Runs[0].Attributes.Attachment = tableAttachment("fixture", table)
				case "divider":
					target.Runs[0].Attributes.Attachment = dividerAttachment()
				case "reference":
					text, ok := a.pasteReference(referenceURL(d.page.ID, ""))
					if !ok {
						t.Fatal("reference fixture")
					}
					target = menuParagraphs(text)[0]
				}
				d.doc.Insert(richtext.TextOf(
					richtext.Paragraph{Runs: []richtext.Run{{Text: "前文中文"}}}, target,
					richtext.Paragraph{Runs: []richtext.Run{{Text: "后文中文"}}},
				))
				tt.Frame()
				before := d.doc.Text()
				original := menuParagraphs(before)[1]
				openBlockMenu(t, a, tt, 1)
				clickBlockMenuItem(t, tt, label)
				ps := menuParagraphs(d.doc.Text())
				if label == "删除块" {
					if d.doc.Text().String() != "前文中文\n后文中文" {
						t.Fatal("attachment deletion damaged surrounding text")
					}
				} else {
					found := 0
					for _, p := range ps {
						if reflect.DeepEqual(p.Runs, original.Runs) {
							found++
						}
					}
					want := 1
					if label == "复制块" {
						want = 2
					}
					if found != want {
						t.Fatal("attachment/reference content lost")
					}
				}
				after := d.doc.Text()
				if label != "复制块链接" {
					tt.Key(ui.Cmd, ui.KeyZ)
					if !d.doc.Text().Equal(before) {
						t.Fatal("attachment undo failed")
					}
					tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
					if !d.doc.Text().Equal(after) {
						t.Fatal("attachment redo failed")
					}
				}
				if kind == "table" && label == "复制块" {
					duplicateStart, _ := paragraphStart(d.doc.Text(), 2)
					changed := copyTable(d.page.Body.Tables["fixture"])
					changed.Cells[1][0] = "仅修改副本"
					a.updateInlineTable(d, duplicateStart, changed)
					if d.page.Body.Tables["fixture"].Cells[1][0] != "数据" {
						t.Fatal("copy edited original table")
					}
					at := menuParagraphs(d.doc.Text())[2].Runs[0].Attributes.Attachment
					if at.URL == "table:fixture" {
						t.Fatal("table copy did not detach on edit")
					}
					d.doc.Undo()
					tt.Frame()
					if !d.doc.Text().Equal(after) {
						t.Fatal("table edit undo lost duplicate")
					}
				}
				if err := a.flush(); err != nil {
					t.Fatal(err)
				}
				reopened, err := newApp(a.store)
				if err != nil {
					t.Fatal(err)
				}
				reopened.selectPage(d.page.ID)
				// Attachments are hydrated to new drawing objects; compare serialized
				// content rather than pointer identity after reopening the database.
				saved, _ := json.Marshal(after)
				restored, _ := json.Marshal(reopened.current().doc.Text())
				if string(saved) != string(restored) {
					t.Fatal("attachment/reference changed after save")
				}
			})
		}
	}
}
