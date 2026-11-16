package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

type taskEstimatePrompt struct {
	draft      *draft
	blockID    string
	convert    bool
	blockIndex int
	input      string
	err        string
	open       bool
	focusInput bool
	rect       ui.Rect
	version    uint64
}

func timedTaskTrailingSpace(st richtext.ParagraphStyle) float32 {
	switch st.Timer.State {
	case richtext.TimerIdle:
		return 156
	case richtext.TimerRunning:
		return 208
	case richtext.TimerPaused:
		return 268
	case richtext.TimerCompleted, richtext.TimerTerminated:
		return 180
	default:
		return 0
	}
}

func taskDuration(elapsed time.Duration) string {
	seconds := int64(max(elapsed, 0) / time.Second)
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func effectiveTaskEstimate(timer richtext.TaskTimer, now time.Time) time.Duration {
	if timer.HasEstimate {
		return time.Duration(max(timer.EstimatedNS, 0))
	}
	return timer.Elapsed(now)
}

func backfillTimedTaskEstimates(b *Body, now time.Time) bool {
	changed := false
	missing := false
	for _, p := range b.Text.Paragraphs() {
		if p.Style.Timer.Enabled() && !p.Style.Timer.HasEstimate {
			missing = true
			break
		}
	}
	if missing {
		b.Text = b.Text.FormatParagraphs(0, b.Text.Len(), func(st *richtext.ParagraphStyle) {
			if st.Timer.Enabled() && !st.Timer.HasEstimate {
				st.Timer.EstimatedNS = int64(st.Timer.Elapsed(now))
				st.Timer.HasEstimate = true
			}
		})
		changed = true
	}
	for id, leaf := range b.Leaves {
		if backfillTimedTaskEstimates(&leaf.Content, now) {
			b.Leaves[id] = leaf
			changed = true
		}
	}
	return changed
}

func parseEstimateDuration(raw string) (time.Duration, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return 0, false
	}
	replacer := strings.NewReplacer(" ", "", "小时", "h", "分钟", "m", "分", "m", "秒", "s")
	s = replacer.Replace(s)
	var d time.Duration
	if mins, err := strconv.ParseFloat(s, 64); err == nil {
		d = time.Duration(mins * float64(time.Minute))
	} else {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return 0, false
		}
		d = parsed
	}
	if d < time.Second || d > 1000*time.Hour {
		return 0, false
	}
	return d.Round(time.Second), true
}

func formatEstimateInput(d time.Duration) string {
	d = d.Round(time.Second)
	if d <= 0 {
		return "25"
	}
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int64(d/time.Hour))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%d", int64(d/time.Minute))
	}
	return d.String()
}

func (a *app) openTaskEstimate(d *draft) {
	a.taskEstimate = &taskEstimatePrompt{
		draft:      d,
		input:      "25",
		open:       true,
		focusInput: true,
		rect:       d.doc.CaretRect(),
		version:    d.doc.Version(),
	}
	a.blockMenu = false
	a.linkOpen = false
	a.urlPaste = nil
}

func (a *app) openBlockTaskEstimate(d *draft, index int) {
	start, ok := paragraphStart(d.doc.Text(), index)
	if !ok {
		return
	}
	st := d.doc.Text().ParagraphStyleAt(start)
	rect := ui.Rect{W: 1, H: 24}
	for _, box := range d.doc.VisibleParagraphs() {
		if box.Start == start {
			rect = ui.Rect{X: 0, Y: box.Top, W: 1, H: max(box.Height, 24)}
			break
		}
	}
	a.taskEstimate = &taskEstimatePrompt{
		draft:      d,
		blockID:    st.ID,
		convert:    true,
		blockIndex: index,
		input:      "25",
		open:       true,
		focusInput: true,
		rect:       rect,
		version:    d.doc.Version(),
	}
}

func (a *app) editBlockTaskEstimate(d *draft, blockID string, current time.Duration) {
	a.taskEstimate = &taskEstimatePrompt{
		draft:      d,
		blockID:    blockID,
		input:      formatEstimateInput(current),
		open:       true,
		focusInput: true,
		version:    d.doc.Version(),
	}
	a.blockMenu = false
	a.linkOpen = false
	a.urlPaste = nil
}

func (a *app) cancelTaskEstimate() {
	s := a.taskEstimate
	a.taskEstimate = nil
	a.pendingSlash = nil
	if s != nil && s.draft != nil {
		a.focusBlockEditor(s.draft)
	}
}

func (a *app) applyTaskEstimate(dur time.Duration) {
	s := a.taskEstimate
	if s == nil || s.draft == nil {
		a.taskEstimate = nil
		a.pendingSlash = nil
		return
	}
	d := s.draft
	if d.doc.Version() != s.version {
		a.taskEstimate = nil
		a.pendingSlash = nil
		return
	}
	a.taskEstimate = nil
	switch {
	case s.convert:
		style := richtext.ParagraphStyle{
			List:  richtext.Tasks,
			Timer: richtext.TaskTimer{State: richtext.TimerIdle, EstimatedNS: int64(dur), HasEstimate: true},
		}
		convertBlock(d.doc, s.blockIndex, style)
		a.changed(d)
	case s.blockID != "":
		pos, ok := blockPosition(d.doc.Text(), s.blockID)
		if !ok {
			return
		}
		anchor, caret := d.doc.Selection()
		d.doc.Edit(func(e *richtext.Edit) {
			e.FormatParagraphs(pos, pos, func(p *richtext.ParagraphStyle) {
				p.Timer.EstimatedNS = int64(dur)
				p.Timer.HasEstimate = true
			})
			e.Select(anchor, caret)
		})
		a.changed(d)
		a.report(a.flush())
	default:
		start, end, remove := a.pendingCommandRange(d)
		d.doc.Edit(func(e *richtext.Edit) {
			if remove {
				e.ReplaceString(start, end, "", richtext.Attributes{})
				e.Select(start, start)
			}
			anchor, caret := e.Selection()
			e.FormatParagraphs(min(anchor, caret), max(anchor, caret), func(p *richtext.ParagraphStyle) {
				id := p.ID
				*p = richtext.ParagraphStyle{
					ID:    id,
					List:  richtext.Tasks,
					Timer: richtext.TaskTimer{State: richtext.TimerIdle, EstimatedNS: int64(dur), HasEstimate: true},
				}
			})
		})
		a.pendingSlash = nil
		a.changed(d)
	}
	a.focusBlockEditor(d)
}

func (a *app) timedTaskAction(d *draft, id, action string, now time.Time) bool {
	start, found := blockPosition(d.doc.Text(), id)
	if !found {
		return false
	}
	st := d.doc.Text().ParagraphStyleAt(start)
	timer := st.Timer
	if !timer.Enabled() {
		return false
	}
	key := runningTask{page: d.page.ID, block: id}.key()
	if d.parent != nil {
		key = runningTask{page: d.parent.page.ID, leaf: d.leafID, block: id}.key()
	}
	if check, ok := a.taskChecks[key]; ok && check.StartedAt == timer.StartedAtNS && check.Expires != 0 && now.UnixNano() > check.Expires {
		now = time.Unix(0, check.Expires)
	}
	switch action {
	case "start":
		timer = timer.Start(taskLocalTime(now))
	case "pause":
		if timer.State != richtext.TimerRunning {
			return false
		}
		timer = timer.Stop(richtext.TimerPaused, now)
	case "complete":
		if timer.State == richtext.TimerCompleted {
			return false
		}
		timer = timer.Stop(richtext.TimerCompleted, now)
	case "terminate":
		if timer.State == richtext.TimerCompleted || timer.State == richtext.TimerTerminated {
			return false
		}
		timer = timer.Stop(richtext.TimerTerminated, now)
	case "reopen":
		if timer.State != richtext.TimerCompleted && timer.State != richtext.TimerTerminated {
			return false
		}
		timer = timer.Stop(richtext.TimerPaused, now)
	default:
		return false
	}
	if timer == st.Timer {
		return false
	}
	if st.Timer.State == richtext.TimerRunning && timer.State != richtext.TimerRunning {
		segments := splitTaskTime(st.Timer, now)
		if len(segments) > 0 {
			if d.page.Body.TaskTimes == nil {
				d.page.Body.TaskTimes = map[string][]TaskTimeSegment{}
			}
			previous := d.page.Body.TaskTimes[st.Timer.HistoryID]
			history := append(append([]TaskTimeSegment(nil), previous...), segments...)
			timer.HistoryID = newID()
			d.page.Body.TaskTimes[timer.HistoryID] = history
		}
	}
	d.doc.Edit(func(e *richtext.Edit) {
		e.FormatParagraphs(start, start, func(p *richtext.ParagraphStyle) {
			p.Timer = timer
			p.Checked = timer.State == richtext.TimerCompleted
		})
	})
	a.changed(d)
	// Save transitions immediately; repainting the counter does not edit the
	// document, add undo steps, or trigger a write every second.
	a.report(a.flush())
	if _, ok := a.taskChecks[key]; ok {
		a.closeTaskNotice(key)
		delete(a.taskChecks, key)
		a.saveTaskChecks()
	}
	return true
}

func (a *app) toggleTimedTask(d *draft, start int, now time.Time) bool {
	st := d.doc.Text().ParagraphStyleAt(start)
	if !st.Timer.Enabled() {
		return false
	}
	action := "complete"
	if st.Timer.State == richtext.TimerCompleted || st.Timer.State == richtext.TimerTerminated {
		action = "reopen"
	}
	a.timedTaskAction(d, st.ID, action, now)
	return true
}

func taskDeltaDisplay(timer richtext.TaskTimer, est, elapsed time.Duration) (text, tooltip string, fg ui.Color) {
	estSec := int64(max(est, 0) / time.Second)
	elapsedSec := int64(max(elapsed, 0) / time.Second)
	diffSec := estSec - elapsedSec
	if diffSec < 0 {
		over := taskDuration(time.Duration(-diffSec) * time.Second)
		return "+" + over, "超出 " + over, ui.Hex("#b8532a")
	}
	rem := taskDuration(time.Duration(diffSec) * time.Second)
	if timer.State == richtext.TimerCompleted && diffSec > 0 {
		return rem, "提前 " + rem, ui.Hex("#2f7450")
	}
	return rem, "剩余 " + rem, muted
}

func (a *app) timedTaskControls(c *ui.Context, d *draft, editor ui.Element) {
	var editAnchor ui.Element
	hasEditAnchor := false
	// Clip controls at the same viewport as the native text when scrolling.
	ui.Box(c.Key("timed-task-controls")).Absolute().Left(gutterWidth).Right(0).Top(0).Bottom(0).Clip().PassThrough().Children(func() {
		now := c.Now()
		for _, box := range d.doc.VisibleParagraphs() {
			st := d.doc.Text().ParagraphStyleAt(box.Start)
			timer := st.Timer
			if !timer.Enabled() {
				continue
			}
			if timer.State == richtext.TimerRunning {
				c.After(time.Second - timer.Elapsed(now)%time.Second)
			}
			est := effectiveTaskEstimate(timer, now)
			elapsed := timer.Elapsed(now)
			showDelta := timer.State == richtext.TimerPaused || timer.State == richtext.TimerCompleted || timer.State == richtext.TimerTerminated
			ui.Row(c.Key(st.ID+":elapsed")).Label("累计用时："+st.ID).Absolute().Top(box.LastLineCenter-12).Right(0).
				Height(24).Gap(3).AlignItems(ui.Center).Children(func() {
				elapsedColor := muted
				if timer.State == richtext.TimerRunning || timer.State == richtext.TimerCompleted {
					elapsedColor = ui.Hex("#6f6e69")
				}
				ui.Text(c, taskDuration(elapsed)).FontSize(12).TextColor(elapsedColor).SingleLine()
				ui.Text(c, "/").FontSize(11).TextColor(ui.Hex("#c9c7c2")).SingleLine()
				estText := taskDuration(est)
				editable := timer.State == richtext.TimerIdle || timer.State == richtext.TimerRunning || timer.State == richtext.TimerPaused
				if editable {
					estBtn := quietButton(c.Key("estimate:"+st.ID), "").Label("调整预计时间："+st.ID).
						Tooltip("预计时间 · 点击调整").KeepFocus().
						Height(22).Padding(0, 3).Radius(5).AlignItems(ui.Center).
						Children(func() { ui.Text(c, estText).FontSize(12).TextColor(muted).SingleLine() })
					if estBtn.Clicked() {
						a.editBlockTaskEstimate(d, st.ID, est)
					}
					if a.taskEstimate != nil && a.taskEstimate.open && a.taskEstimate.draft == d && !a.taskEstimate.convert && a.taskEstimate.blockID == st.ID {
						editAnchor = estBtn
						hasEditAnchor = true
					}
				} else {
					ui.Row(c.Key("estimate:"+st.ID)).Tooltip("预计时间").Height(22).Padding(0, 3).AlignItems(ui.Center).Children(func() {
						ui.Text(c, estText).FontSize(12).TextColor(muted).SingleLine()
					})
				}
				if showDelta {
					deltaText, deltaTip, deltaFG := taskDeltaDisplay(timer, est, elapsed)
					ui.Text(c, "/").FontSize(11).TextColor(ui.Hex("#c9c7c2")).SingleLine()
					ui.Row(c.Key("delta:"+st.ID)).Tooltip(deltaTip).Height(22).AlignItems(ui.Center).Children(func() {
						ui.Text(c, deltaText).FontSize(12).TextColor(deltaFG).SingleLine()
					})
				}
			})
			if timer.State == richtext.TimerCompleted || timer.State == richtext.TimerTerminated {
				continue
			}
			width := float32(76)
			if timer.State == richtext.TimerIdle {
				width = 24
			}
			ui.Row(c.Key(st.ID)).Label("计时待办操作：" + st.ID).Absolute().Top(box.LastLineCenter - 12).Left(box.TextEndX + 8).
				Height(24).Width(width).Gap(2).AlignItems(ui.Center).Children(func() {
				action := func(label, icon, command string) {
					if quietButton(c.Key(command), "").Label(label+"计时待办："+st.ID).Tooltip(label).KeepFocus().
						Size(24, 24).Padding(4).TextColor(muted).Justify(ui.Center).Children(func() {
						ui.Icon(c, icons[icon]).FontSize(16)
					}).Clicked() {
						a.timedTaskAction(d, st.ID, command, c.Now())
					}
				}
				switch timer.State {
				case richtext.TimerIdle:
					action("开始", "play", "start")
				case richtext.TimerRunning:
					action("暂停", "pause", "pause")
					action("完成", "check", "complete")
					action("废弃", "close", "terminate")
				case richtext.TimerPaused:
					action("继续", "play", "start")
					action("完成", "check", "complete")
					action("废弃", "close", "terminate")
				}
			})
		}
	})
	a.taskEstimateView(c, d, editor, editAnchor, hasEditAnchor)
}

func (a *app) taskEstimateView(c *ui.Context, d *draft, editor, editAnchor ui.Element, hasEditAnchor bool) {
	s := a.taskEstimate
	if s == nil || !s.open || s.draft != d {
		return
	}
	if d.doc.Version() != s.version {
		a.taskEstimate = nil
		a.pendingSlash = nil
		return
	}
	anchor := editAnchor
	if !hasEditAnchor {
		if !s.convert && s.blockID != "" {
			a.taskEstimate = nil
			return
		}
		r := s.rect
		anchor = ui.Box(c.Key("task-estimate-anchor:"+d.page.ID)).AttachTo(editor, ui.AnchorTopStart, ui.AnchorTopStart).
			Left(r.X).Top(r.Y).Size(max(r.W, 1), max(r.H, 24)).PassThrough()
	}
	presets := [][]struct {
		label string
		dur   time.Duration
	}{
		{{"15 分钟", 15 * time.Minute}, {"25 分钟", 25 * time.Minute}, {"30 分钟", 30 * time.Minute}},
		{{"45 分钟", 45 * time.Minute}, {"1 小时", time.Hour}, {"2 小时", 2 * time.Hour}},
	}
	currentDur, hasCurrent := parseEstimateDuration(s.input)
	width, _ := c.Size()
	panel := ui.PopoverBase(c.Key("task-estimate-popover"), anchor, &s.open, func(panel ui.Element) {
		panel.Label("设置预计完成时间").Width(min(float32(308), width-32)).Padding(14).Gap(10).
			Background(paper).Border(1, line).Radius(12).Margin(6, 0, 0, 0).Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, .1))
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons["clock"]).FontSize(15).TextColor(muted)
			ui.Text(c, "预计完成时间").FontSize(13).FontWeight(600).TextColor(ink)
		})
		ui.Column(c).Gap(6).Children(func() {
			for rowIdx, row := range presets {
				ui.Row(c.Key(fmt.Sprintf("preset-row:%d", rowIdx))).Gap(6).Children(func() {
					for _, p := range row {
						btn := ui.Button(c.Key(p.label), p.label).Label("预计时长："+p.label).
							Grow(1).MinWidth(0).Height(28).Radius(6).FontSize(12).Justify(ui.Center).
							Background(sidebarColor).Border(1, line).TextColor(ink)
						if hasCurrent && currentDur == p.dur {
							btn.Background(ui.Hex("#f2f7fd")).Border(1, ui.Hex("#2e87dc")).TextColor(ui.Hex("#2e87dc"))
						}
						if btn.Clicked() {
							a.applyTaskEstimate(p.dur)
						}
					}
				})
			}
		})
		input := ui.TextInput(c, &s.input).Label("预计时长输入").Placeholder("25、1h30m 或 45s").
			Height(32).FontSize(14).Background(sidebarColor).Border(1, ui.Hex("#e5e4e2")).Radius(7).AutoFocus()
		if s.focusInput {
			input.Focus().SetTextSelection(0, len([]rune(s.input)))
			s.focusInput = false
		}
		if input.Changed() {
			s.err = ""
		}
		if s.err != "" {
			ui.Text(c, s.err).FontSize(11).TextColor(ui.Hex("#b8532a"))
		} else {
			ui.Text(c, "输入分钟数（如 25）或 1h30m · Enter 确认").FontSize(11).TextColor(muted)
		}
		confirm := func() {
			dur, ok := parseEstimateDuration(s.input)
			if !ok {
				s.err = "请输入大于 0 的时长，如 25 或 1h30m"
				return
			}
			a.applyTaskEstimate(dur)
		}
		if input.Submitted() {
			confirm()
		}
		ui.Row(c).Gap(6).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			if quietButton(c, "取消").Label("取消预计时间").Padding(5, 10).Clicked() {
				a.cancelTaskEstimate()
			}
			if ui.PrimaryButton(c, "确定").Label("确认预计时间").Padding(5, 12).Clicked() {
				confirm()
			}
		})
		if panel.OverlayShortcut(0, ui.KeyEscape) {
			a.cancelTaskEstimate()
		}
	})
	if panel.Dismissed() && a.taskEstimate == s {
		a.cancelTaskEstimate()
	}
}
