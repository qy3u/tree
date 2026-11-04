package main

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

type blockCommand struct {
	ID, Label, Aliases string
	Style              richtext.ParagraphStyle
}

var blockCommands = []blockCommand{
	{ID: "text", Label: "文本", Aliases: "paragraph plain 正文 段落"},
	{ID: "h1", Label: "一级标题", Aliases: "heading1 heading 1 title 标题1", Style: richtext.ParagraphStyle{Heading: 1}},
	{ID: "h2", Label: "二级标题", Aliases: "heading2 heading 2 subtitle 标题2", Style: richtext.ParagraphStyle{Heading: 2}},
	{ID: "h3", Label: "三级标题", Aliases: "heading3 heading 3 标题3", Style: richtext.ParagraphStyle{Heading: 3}},
	{ID: "page", Label: "子页面", Aliases: "subpage child 页面 子页"},
	{ID: "bullet", Label: "项目列表", Aliases: "bulleted unordered list 无序列表", Style: richtext.ParagraphStyle{List: richtext.Bullets}},
	{ID: "number", Label: "编号列表", Aliases: "numbered ordered list 有序列表", Style: richtext.ParagraphStyle{List: richtext.Numbers}},
	{ID: "ttodo", Label: "计时待办", Aliases: "timed timer task 计时 计时器", Style: richtext.ParagraphStyle{List: richtext.Tasks, Timer: richtext.TaskTimer{State: richtext.TimerIdle}}},
	{ID: "todo", Label: "待办列表", Aliases: "to-do task checkbox checklist 待办 任务 复选框", Style: richtext.ParagraphStyle{List: richtext.Tasks}},
	{ID: "quote", Label: "引用", Aliases: "blockquote 引用块", Style: richtext.ParagraphStyle{Quote: true}},
	{ID: "divider", Label: "分隔线", Aliases: "hr separator 分割线"},
	{ID: "image", Label: "图片…", Aliases: "photo picture 图片 图像"},
	{ID: "paste-image", Label: "粘贴图片", Aliases: "clipboard 粘贴图片"},
	{ID: "code", Label: "代码块", Aliases: "codeblock 代码", Style: richtext.ParagraphStyle{CodeBlock: true}},
	{ID: "table", Label: "插入 / 编辑表格…", Aliases: "grid 表格"},
}

func matchingCommands(query string) []blockCommand {
	query = strings.ToLower(strings.TrimSpace(query))
	var exact, other []blockCommand
	for _, cmd := range blockCommands {
		if cmd.ID == query {
			exact = append(exact, cmd)
		} else if strings.Contains(strings.ToLower(cmd.ID+" "+cmd.Label+" "+cmd.Aliases), query) {
			other = append(other, cmd)
		}
	}
	return append(exact, other...)
}

type slashCommand struct {
	Page, Query          string
	Start, End, Selected int
	ParagraphStart       int
	Reveal               bool
}

func (s *slashCommand) key() string { return fmt.Sprintf("%s:%d", s.Page, s.Start) }

// A slash starts a command at a word boundary. URLs, paths and code keep
// their normal editing behavior. Offsets are runes, matching richtext.
func findSlash(d *draft) *slashCommand {
	anchor, caret := d.doc.Selection()
	if anchor != caret {
		return nil
	}
	text := d.doc.Text()
	var start int
	for pos, p := range text.Paragraphs() {
		if pos > caret {
			break
		}
		start = pos
		if pos == caret || pos+paragraphLength(p) >= caret {
			if p.Style.CodeBlock {
				return nil
			}
			break
		}
	}
	prefix := []rune(text.Slice(start, caret).PlainText())
	i := len(prefix) - 1
	for ; i >= 0 && isCommandRune(prefix[i]); i-- {
	}
	if i < 0 || prefix[i] != '/' || i > 0 && !unicode.IsSpace(prefix[i-1]) || len(prefix)-i > 65 {
		return nil
	}
	if caret < text.Len() {
		next := []rune(text.Slice(caret, caret+1).PlainText())
		if len(next) > 0 && (isCommandRune(next[0]) || next[0] == '/') {
			return nil
		}
	}
	for _, run := range text.Runs(start+i, caret) {
		if run.Attributes.Code {
			return nil
		}
	}
	return &slashCommand{Page: d.page.ID, Start: start + i, End: caret, ParagraphStart: start, Query: string(prefix[i+1:]), Reveal: true}
}
func isCommandRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_'
}
func paragraphLength(p richtext.Paragraph) int {
	n := 0
	for _, r := range p.Runs {
		n += len([]rune(r.Text))
	}
	return n
}

func (a *app) dismissSlash() {
	if a.slash != nil {
		a.slashDismissed = a.slash.key()
	}
	a.slash = nil
}
func (a *app) syncSlash(d *draft, editor ui.Element) {
	if a.tableOpen || a.errorOpen || a.linkOpen || a.searchOpen || a.insertOpen {
		return
	}
	s := findSlash(d)
	if s == nil {
		a.slash = nil
		a.slashDismissed = ""
		return
	}
	if !editor.Focused() {
		a.dismissSlash()
		return
	}
	if a.slash != nil && a.slash.key() == s.key() {
		if a.slash.Query == s.Query {
			s.Selected = a.slash.Selected
			s.Reveal = a.slash.Reveal
		}
		a.slash = s
	} else if editor.Changed() && s.key() != a.slashDismissed {
		a.slash = s
	} else {
		a.dismissSlash()
	}
}

// Keep the native editor focused while claiming only command-navigation
// keys. MyGo routes registered element shortcuts ahead of editor input.
func (a *app) slashKeys(d *draft, editor ui.Element) {
	s := a.slash
	if s == nil {
		return
	}
	items := matchingCommands(s.Query)
	if editor.Shortcut(0, ui.KeyEscape) {
		a.dismissSlash()
		return
	}
	down, up := editor.Shortcut(0, ui.KeyDown), editor.Shortcut(0, ui.KeyUp)
	if len(items) > 0 && (down || up) {
		delta := 1
		if up {
			delta = -1
		}
		s.Selected = (s.Selected + delta + len(items)) % len(items)
		s.Reveal = true
	}
	enter, tab := editor.Shortcut(0, ui.KeyEnter), editor.Shortcut(0, ui.KeyTab)
	if len(items) > 0 && (enter || tab) {
		a.runBlockCommand(d, items[min(s.Selected, len(items)-1)], true)
	}
}

func (a *app) slashMenu(c *ui.Context, d *draft, anchor ui.Element) {
	s := a.slash
	if s == nil {
		return
	}
	items := matchingCommands(s.Query)
	open := true
	panel := ui.PopoverBase(c.Key("slash-menu"), anchor, &open, func(panel ui.Element) {
		panel.Width(320).Padding(0).Gap(0).Radius(10).Background(paper).Border(1, line).Shadow(0, 5, 18, 0, ui.RGBA(0, 0, 0, 0.08)).Margin(6, 0)
		a.commandList(c, items, s.Selected, s.Reveal, func(cmd blockCommand) { a.runBlockCommand(d, cmd, true) })
		ui.Box(c).Height(1).Background(line)
		if quietButton(c, "").Label("关闭命令菜单").KeepFocus().Height(40).Padding(0, 12).Radius(0).Children(func() {
			ui.Text(c, "关闭菜单").FontSize(13).Grow(1)
			ui.Text(c, "esc").FontSize(12).TextColor(muted)
		}).Clicked() {
			a.dismissSlash()
		}
	})
	s.Reveal = false
	if panel.Dismissed() {
		a.dismissSlash()
	}
}

func (a *app) commandList(c *ui.Context, items []blockCommand, selected int, reveal bool, run func(blockCommand)) {
	if len(items) == 0 {
		ui.Text(c, "没有匹配的内容块").FontSize(13).TextColor(muted).Padding(12, 6)
		return
	}
	groups := 0
	lastGroup := ""
	for _, cmd := range items {
		group := commandGroup(cmd.ID)
		if group != lastGroup {
			groups++
			lastGroup = group
		}
	}
	ui.Scroll(c).Height(min(float32(344), float32(len(items)*34+groups*34+12))).Padding(6, 6).Children(func() {
		lastGroup := ""
		for i, cmd := range items {
			group := commandGroup(cmd.ID)
			if group != lastGroup {
				if lastGroup != "" {
					ui.Box(c).Height(1).Background(line).Margin(6, 6)
				}
				ui.Text(c, group).FontSize(12).FontWeight(500).TextColor(ui.Hex("#85847f")).Height(32).Padding(8, 6).Shrink(0)
				lastGroup = group
			}
			icon, hint, label := commandAppearance(cmd)
			button := quietButton(c.Key("command:"+cmd.ID), "").Label("命令："+cmd.Label).KeepFocus().Height(34).Shrink(0).Padding(0, 8).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icons[icon]).FontSize(19).TextColor(ink).Shrink(0)
				ui.Text(c, label).FontSize(14).FontWeight(400).SingleLine().Grow(1).MinWidth(0)
				if hint != "" {
					ui.Text(c, hint).FontSize(13).TextColor(muted).SingleLine()
				}
			})
			if i == selected {
				button.Background(line)
				if reveal {
					button.ScrollIntoView()
				}
			}
			if button.Clicked() {
				run(cmd)
			}
		}
	})
}

func (a *app) pendingCommandRange(d *draft) (int, int, bool) {
	s := a.pendingSlash
	if s == nil || s.Page != d.page.ID || s.End > d.doc.Text().Len() || d.doc.Text().Slice(s.Start, s.End).PlainText() != "/"+s.Query {
		return 0, 0, false
	}
	return s.Start, s.End, true
}

func (a *app) runBlockCommand(d *draft, cmd blockCommand, fromSlash bool) {
	if fromSlash {
		if a.slash == nil {
			return
		}
		copy := *a.slash
		a.pendingSlash = &copy
		a.dismissSlash()
	} else {
		a.pendingSlash = nil
	}
	a.insertOpen = false
	switch cmd.ID {
	case "page":
		a.slashPage(d)
		return
	case "divider":
		a.insertAttachment(dividerAttachment())
	case "image":
		a.importImage()
		a.pendingSlash = nil
	case "paste-image":
		a.pasteImage()
		a.pendingSlash = nil
	case "table":
		a.openTable()
		return
	default:
		start, end, remove := a.pendingCommandRange(d)
		d.doc.Edit(func(e *richtext.Edit) {
			if remove {
				e.ReplaceString(start, end, "", richtext.Attributes{})
				e.Select(start, start)
			}
			anchor, caret := e.Selection()
			e.FormatParagraphs(min(anchor, caret), max(anchor, caret), func(p *richtext.ParagraphStyle) { id := p.ID; *p = cmd.Style; p.ID = id })
		})
		a.pendingSlash = nil
	}
	a.changed(d)
	a.editorHandle.Focus()
}

// Resolve against the current document even when text and Enter arrive before
// another frame can display the menu (fast typing or an accessibility input).
func (a *app) nativeSlashKey(d *draft, key ui.Key, mods ui.Modifiers) bool {
	if mods != 0 || (key != ui.KeyEnter && key != ui.KeyTab) {
		return false
	}
	s := findSlash(d)
	if s == nil || s.key() == a.slashDismissed {
		return false
	}
	items := matchingCommands(s.Query)
	if len(items) == 0 {
		return false
	}
	index := 0
	if a.slash != nil && a.slash.Start == s.Start && a.slash.Query == s.Query {
		index = min(a.slash.Selected, len(items)-1)
	}
	a.slash = s
	a.runBlockCommand(d, items[index], true)
	return true
}
