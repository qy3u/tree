package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type codeLanguageMenu struct {
	Open  bool
	Page  string
	Start int
	Query string
}
type codeLanguage struct{ Name, ID string }

func codeLanguages(query string) []codeLanguage {
	query = strings.ToLower(strings.TrimSpace(query))
	var result []codeLanguage
	if query == "" || strings.Contains("纯文本 plain text plaintext", query) {
		result = append(result, codeLanguage{"纯文本", ""})
	}
	alias := editor.LanguageNamed(query)
	for _, lang := range editor.Languages() {
		if lang.Tokenizer == nil {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(lang.Name), query) || lang == alias {
			result = append(result, codeLanguage{lang.Name, strings.ToLower(lang.Name)})
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].ID == "" {
			return true
		}
		if result[j].ID == "" {
			return false
		}
		return result[i].Name < result[j].Name
	})
	return result
}
func codeLanguageName(id string) string {
	if id == "" {
		return "纯文本"
	}
	if lang := editor.LanguageNamed(id); lang != nil {
		return lang.Name
	}
	return id
}
func setCodeLanguage(d *draft, box richtext.CodeBlockBox, language string) {
	anchor, caret := d.doc.Selection()
	d.doc.Edit(func(e *richtext.Edit) {
		e.FormatParagraphs(box.Start, box.End, func(st *richtext.ParagraphStyle) {
			if st.CodeBlock {
				st.CodeLanguage = language
			}
		})
		e.Select(anchor, caret)
	})
}
func codeBlockText(d *draft, box richtext.CodeBlockBox) string {
	return strings.ReplaceAll(d.doc.Text().Slice(box.Start, box.End).PlainText(), "\u2028", "\n")
}
func (a *app) codeControls(c *ui.Context, d *draft) {
	for _, box := range d.doc.VisibleCodeBlocks() {
		if box.Rect.Y < 0 {
			continue
		}
		key := fmt.Sprintf("code:%s:%d", d.page.ID, box.Start)
		ui.Row(c.Key(key)).Absolute().Top(box.Rect.Y+4).Right(8).Height(26).Radius(6).Background(paper).Border(1, line).AlignItems(ui.Center).Children(func() {
			language := quietButton(c, "").Label(fmt.Sprintf("代码语言 %d", box.Start)).KeepFocus().Height(26).Padding(0, 8).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, codeLanguageName(box.Language)).FontSize(12).TextColor(muted)
				ui.Icon(c, icons["chevron-down"]).FontSize(13).TextColor(muted)
			})
			if language.Clicked() {
				open := !(a.codeMenu.Open && a.codeMenu.Page == d.page.ID && a.codeMenu.Start == box.Start)
				a.codeMenu = codeLanguageMenu{Open: open, Page: d.page.ID, Start: box.Start}
			}
			ui.Box(c).Width(1).Height(16).Background(line)
			copy := quietButton(c, "").Label(fmt.Sprintf("复制代码 %d", box.Start)).Tooltip("复制代码").KeepFocus().Size(28, 26).Padding(6).Children(func() { ui.Icon(c, icons["copy"]).FontSize(15).TextColor(muted) })
			if copy.Clicked() {
				c.WriteClipboard(codeBlockText(d, box))
			}
			if a.codeMenu.Page == d.page.ID && a.codeMenu.Start == box.Start {
				ui.PopoverBase(c, language, &a.codeMenu.Open, func(panel ui.Element) {
					panel.Width(250).Padding(8).Gap(6).Radius(10).Background(paper).Border(1, line).Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, .1))
					search := ui.TextInput(c, &a.codeMenu.Query).Label("搜索代码语言").Placeholder("搜索语言…").Height(34).AutoFocus()
					options := codeLanguages(a.codeMenu.Query)
					choose := func(id string) {
						setCodeLanguage(d, box, id)
						a.changed(d)
						a.codeMenu.Open = false
						a.editorHandle.Focus()
					}
					if search.Submitted() && len(options) > 0 {
						choose(options[0].ID)
					}
					ui.Scroll(c).Height(float32(min(280, max(32, len(options)*32)))).Children(func() {
						for _, option := range options {
							row := quietButton(c.Key(option.Name), "").Label("代码语言："+option.Name).Height(32).Padding(0, 10).AlignItems(ui.Center).Children(func() {
								ui.Text(c, option.Name).FontSize(13).Grow(1)
								if strings.EqualFold(codeLanguageName(box.Language), option.Name) {
									ui.Text(c, "✓").FontSize(13).TextColor(muted)
								}
							})
							if row.Clicked() {
								choose(option.ID)
							}
						}
						if len(options) == 0 {
							ui.Text(c, "没有匹配的语言").TextColor(muted).FontSize(12).Padding(8)
						}
					})
				})
			}
		})
	}
}
