package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func timedTaskTrailingSpace(st richtext.ParagraphStyle) float32 {
	switch st.Timer.State {
	case richtext.TimerIdle:
		return 36
	case richtext.TimerRunning, richtext.TimerPaused:
		return 172
	case richtext.TimerCompleted, richtext.TimerTerminated:
		return 88
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

func (a *app) timedTaskControls(c *ui.Context, d *draft) {
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
			if timer.State != richtext.TimerIdle {
				ui.Row(c.Key(st.ID + ":elapsed")).Label("累计用时：" + st.ID).Absolute().Top(box.LastLineCenter - 12).Right(0).
					Width(80).Height(24).Justify(ui.End).AlignItems(ui.Center).Children(func() {
					ui.Text(c, taskDuration(timer.Elapsed(now))).FontSize(12).TextColor(muted).SingleLine()
				})
			}
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
}
