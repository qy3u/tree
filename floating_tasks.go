package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

type activeTimedTask struct {
	page      string
	pageTitle string
	leaf      string
	leafTitle string
	block     string
	label     string
	timer     richtext.TaskTimer
}

func (t activeTimedTask) key() string { return t.page + "/" + t.leaf + "/" + t.block }

func truncateLabelRunes(s string, limit int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= limit || limit <= 1 {
		return string(r)
	}
	return string(r[:limit-1]) + "…"
}

func (t activeTimedTask) sourceLabel() string {
	pt := truncateLabelRunes(t.pageTitle, 10)
	if pt == "" {
		pt = "无标题"
	}
	if t.leaf == "" {
		return pt
	}
	lt := truncateLabelRunes(t.leafTitle, 8)
	if lt == "" {
		return pt + " · Leaf"
	}
	return pt + " · " + lt
}

func (a *app) activeTimedTasks() []activeTimedTask {
	var out []activeTimedTask
	for _, p := range a.pages {
		if p.Deleted != "" {
			continue
		}
		body := p.Body
		title := p.Title
		d := a.drafts[p.ID]
		if d != nil {
			body = d.page.Body
			body.Text = d.doc.Text()
			title = d.page.Title
		}
		collect := func(leafID, leafTitle string, b Body) {
			for _, para := range b.Text.Paragraphs() {
				if para.Style.Timer.State != richtext.TimerRunning && para.Style.Timer.State != richtext.TimerPaused {
					continue
				}
				label := strings.TrimSpace(richtext.TextOf(para).PlainText())
				if label == "" {
					label = "未命名计时待办"
				}
				out = append(out, activeTimedTask{
					page:      p.ID,
					pageTitle: title,
					leaf:      leafID,
					leafTitle: leafTitle,
					block:     para.Style.ID,
					label:     label,
					timer:     para.Style.Timer,
				})
			}
		}
		collect("", "", body)
		for _, run := range body.Text.Runs(0, body.Text.Len()) {
			id := leafID(run.Attributes.Attachment)
			if id == "" {
				continue
			}
			leaf, ok := body.Leaves[id]
			if !ok {
				continue
			}
			content := leaf.Content
			leafTitle := leaf.Title
			if d != nil {
				if v := d.leafViews[id]; v != nil {
					content = v.draft.page.Body
					content.Text = v.draft.doc.Text()
					leafTitle = v.draft.page.Title
				}
			}
			collect(id, leafTitle, content)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri := out[i].timer.State == richtext.TimerRunning
		rj := out[j].timer.State == richtext.TimerRunning
		if ri != rj {
			return ri
		}
		if ri && out[i].timer.StartedAtNS != out[j].timer.StartedAtNS {
			return out[i].timer.StartedAtNS > out[j].timer.StartedAtNS
		}
		return false
	})
	return out
}

func (a *app) runActiveTimedTaskAction(t activeTimedTask, action string, now time.Time) bool {
	d := a.taskDraft(runningTask{page: t.page, leaf: t.leaf, block: t.block})
	if d == nil {
		return false
	}
	return a.timedTaskAction(d, t.block, action, now)
}

func (a *app) jumpToTimedTask(t activeTimedTask) {
	a.floatingTasksOpen = false
	a.selectPage(t.page)
	if a.selected != t.page || a.errorOpen {
		return
	}
	d := a.current()
	if d == nil {
		return
	}
	if t.leaf == "" {
		pos, ok := blockPosition(d.doc.Text(), t.block)
		if !ok {
			return
		}
		d.doc.SetSelection(pos, pos)
		d.doc.CenterSelection()
		a.blockJump = blockJump{page: t.page, block: t.block}
		return
	}
	for pos, para := range d.doc.Text().Paragraphs() {
		matched := false
		for _, run := range para.Runs {
			if leafID(run.Attributes.Attachment) == t.leaf {
				matched = true
				break
			}
		}
		if matched {
			d.doc.SetSelection(pos, pos)
			d.doc.CenterSelection()
			break
		}
	}
	if v := a.leafView(d, t.leaf); v != nil {
		if v.collapsed {
			v.collapsed = false
			d.layoutRevision++
		}
		if bpos, ok := blockPosition(v.draft.doc.Text(), t.block); ok {
			v.draft.doc.SetSelection(bpos, bpos)
			v.focusBody = true
			v.reveal = true
			v.centerReveal = true
			a.blockJump = blockJump{page: d.page.ID + ":" + t.leaf, block: t.block}
		}
	}
}

func taskOverrunRatio(elapsed, est time.Duration) string {
	if est <= 0 || elapsed <= est {
		return ""
	}
	ratio := float64(elapsed) / float64(est)
	if ratio < 1.05 {
		return "1.0+×"
	}
	return fmt.Sprintf("%.1f×", ratio)
}

func taskSplitProgressBar(c *ui.Context, key string, timer richtext.TaskTimer, elapsed, est time.Duration, height float32, rounded bool) {
	radius := float32(0)
	if rounded {
		radius = 999
	}
	fillColor := ui.Hex("#2e87dc")
	if timer.State == richtext.TimerPaused {
		fillColor = muted
	}
	overColor := ui.Hex("#b8532a")
	track := ui.Row(c.Key("progress:"+key)).Label("悬浮计时进度："+key).WidthPercent(100).Height(height).Background(line).Radius(radius)
	track.Children(func() {
		if elapsed <= est && est > 0 {
			pct := max(0, min(100, float32(elapsed)/float32(est)*100))
			if pct > 0 {
				ui.Box(c.Key("expected:"+key)).Label("预期基准段："+key).WidthPercent(pct).Height(height).Background(fillColor).Radius(radius)
			}
			return
		}
		if elapsed > est {
			if est <= 0 {
				ui.Box(c.Key("overrun:"+key)).Label("超出延长段："+key).WidthPercent(100).Height(height).Background(overColor).Radius(radius)
				return
			}
			estPct := max(4, min(96, float32(est)/float32(elapsed)*100))
			ui.Box(c.Key("expected:"+key)).Label("预期基准段："+key).WidthPercent(estPct).Height(height).Background(fillColor).Radius(radius, 0, 0, radius).Shrink(0)
			ui.Box(c.Key("overrun:"+key)).Label("超出延长段："+key).Grow(1).MinWidth(0).Height(height).Background(overColor).Radius(0, radius, radius, 0)
		}
	})
}

func (a *app) topRightOverlays(c *ui.Context) {
	tasks := a.activeTimedTasks()
	if len(tasks) == 0 {
		a.floatingTasksOpen = false
	}
	noticeKeys, runningMap := a.pendingTaskCheckKeys()
	if len(tasks) == 0 && len(noticeKeys) == 0 {
		return
	}
	now := c.Now()
	var nextTick time.Duration
	for _, task := range tasks {
		if task.timer.State != richtext.TimerRunning {
			continue
		}
		rem := time.Second - task.timer.Elapsed(now)%time.Second
		if nextTick == 0 || rem < nextTick {
			nextTick = rem
		}
	}
	if nextTick > 0 {
		c.After(nextTick)
	}
	w, _ := c.Size()
	panelWidth := min(float32(340), max(float32(220), w-48))
	ui.Overlay(c, func() {
		ui.Column(c.Key("top-right-overlays")).Absolute().Right(24).Top(88).Width(panelWidth).Gap(8).Children(func() {
			if len(tasks) > 0 {
				a.floatingTimedTaskCard(c, tasks)
			}
			if len(noticeKeys) > 0 {
				a.renderTaskCheckNotices(c, noticeKeys, runningMap)
			}
		})
	})
}

func (a *app) floatingTaskActions(c *ui.Context, task activeTimedTask, includeTerminate bool) {
	actionBtn := func(label, icon, command string) {
		if quietButton(c.Key(command), "").Label("悬浮"+label+"计时："+task.key()).Tooltip(label).KeepFocus().
			Size(22, 22).Padding(3).Justify(ui.Center).AlignItems(ui.Center).TextColor(muted).Children(func() {
			ui.Icon(c, icons[icon]).FontSize(14)
		}).Clicked() {
			a.runActiveTimedTaskAction(task, command, c.Now())
		}
	}
	ui.Row(c.Key("actions:"+task.key())).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
		if task.timer.State == richtext.TimerRunning {
			actionBtn("暂停", "pause", "pause")
		} else {
			actionBtn("继续", "play", "start")
		}
		actionBtn("完成", "check", "complete")
		if includeTerminate {
			actionBtn("废弃", "close", "terminate")
		}
	})
}

func floatingStatusDotColor(timer richtext.TaskTimer, elapsed, est time.Duration) ui.Color {
	if elapsed > est {
		return ui.Hex("#b8532a")
	}
	if timer.State == richtext.TimerRunning {
		return ui.Hex("#2f7450")
	}
	return muted
}

func (a *app) floatingTimedTaskCard(c *ui.Context, tasks []activeTimedTask) {
	now := c.Now()
	primary := tasks[0]
	pElapsed := primary.timer.Elapsed(now)
	pEst := effectiveTaskEstimate(primary.timer, now)

	ui.Column(c.Key("floating-tasks-capsule")).Label("悬浮计时待办胶囊").
		Background(paper).Border(1, line).Radius(10).Shadow(0, 3, 12, 0, ui.RGBA(0, 0, 0, .08)).
		Padding(6, 10, 6, 10).Gap(5).Clip().
		Children(func() {
			ui.Row(c.Key("capsule-row")).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(7, 7).Radius(99).Background(floatingStatusDotColor(primary.timer, pElapsed, pEst)).Shrink(0)
				titleBtn := quietButton(c.Key("capsule-jump"), "").Label("定位计时待办："+primary.key()).
					Tooltip(primary.sourceLabel()+" · 点击跳转").KeepFocus().
					Grow(1).MinWidth(0).Height(22).Padding(0, 4).AlignItems(ui.Center).
					Children(func() {
						ui.Text(c, primary.label).FontSize(12.5).FontWeight(500).TextColor(ink).SingleLine().MinWidth(0)
					})
				if titleBtn.Clicked() {
					a.jumpToTimedTask(primary)
				}
				ui.Row(c.Key("capsule-time")).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
					ui.Text(c, taskDuration(pElapsed)).FontSize(11.5).FontWeight(500).TextColor(ink).SingleLine()
					ui.Text(c, "/").FontSize(11).TextColor(ui.Hex("#c9c7c2")).SingleLine()
					ui.Text(c, taskDuration(pEst)).FontSize(11.5).TextColor(muted).SingleLine()
				})
				a.floatingTaskActions(c, primary, false)

				toggleLabel := "展开计时待办列表"
				toggleIcon := "chevron-down"
				if a.floatingTasksOpen {
					toggleLabel = "收起计时待办列表"
					toggleIcon = "chevron-up"
				}
				toggleBtn := ui.Button(c.Key("floating-tasks-toggle"), "").Label(toggleLabel).
					Tooltip(toggleLabel).KeepFocus().
					Height(22).Padding(0, 5).Gap(2).Radius(5).Background(sidebarColor).Border(1, line).
					AlignItems(ui.Center).Justify(ui.Center).Shrink(0).
					Children(func() {
						if len(tasks) > 1 && !a.floatingTasksOpen {
							ui.Text(c, fmt.Sprintf("+%d", len(tasks)-1)).FontSize(11).FontWeight(500).TextColor(muted).SingleLine()
						}
						ui.Icon(c, icons[toggleIcon]).FontSize(13).TextColor(muted)
					})
				if toggleBtn.Clicked() {
					a.floatingTasksOpen = !a.floatingTasksOpen
				}
			})
			taskSplitProgressBar(c, "capsule:"+primary.key(), primary.timer, pElapsed, pEst, 3, true)
		})

	if !a.floatingTasksOpen {
		return
	}

	_, h := c.Size()
	scrollH := min(float32(len(tasks)*86), max(float32(120), h-240))
	panel := ui.Column(c.Key("floating-tasks-list")).Label("悬浮计时待办列表").
		Background(paper).Border(1, line).Radius(10).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, .12)).Clip()
	panel.Children(func() {
		ui.Scroll(c.Key("floating-tasks-scroll")).Height(scrollH).Children(func() {
			for i, task := range tasks {
				if i > 0 {
					ui.Box(c).Height(1).Background(line)
				}
				tElapsed := task.timer.Elapsed(now)
				tEst := effectiveTaskEstimate(task.timer, now)
				tOverrun := tElapsed > tEst
				ui.Column(c.Key("row:"+task.key())).Padding(9, 12).Gap(6).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(7, 7).Radius(99).Background(floatingStatusDotColor(task.timer, tElapsed, tEst)).Shrink(0)
						rowTitle := quietButton(c.Key("list-jump:"+task.key()), "").Label("列表定位计时待办："+task.key()).
							Tooltip("点击跳转到该待办").KeepFocus().
							Grow(1).MinWidth(0).Height(22).Padding(0, 2).AlignItems(ui.Center).
							Children(func() {
								ui.Text(c, task.label).FontSize(12.5).FontWeight(500).TextColor(ink).SingleLine().MinWidth(0)
							})
						if rowTitle.Clicked() {
							a.jumpToTimedTask(task)
						}
						srcBtn := ui.Button(c.Key("list-source:"+task.key()), "").Label("跳转来源："+task.key()).
							Tooltip("跳转到 "+task.sourceLabel()).KeepFocus().
							Height(20).Padding(0, 5).Radius(4).Background(sidebarColor).Border(0, line).
							AlignItems(ui.Center).Shrink(0).
							Children(func() {
								ui.Text(c, task.sourceLabel()+" ↗").FontSize(10.5).TextColor(muted).SingleLine()
							})
						if srcBtn.Clicked() {
							a.jumpToTimedTask(task)
						}
						a.floatingTaskActions(c, task, true)
					})
					taskSplitProgressBar(c, "list:"+task.key(), task.timer, tElapsed, tEst, 4, true)
					ui.Row(c).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Children(func() {
						estSwatchColor := ui.Hex("#2e87dc")
						if task.timer.State == richtext.TimerPaused {
							estSwatchColor = muted
						}
						if tOverrun {
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								if tEst > 0 {
									ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
										ui.Box(c).Size(8, 3).Radius(2).Background(estSwatchColor)
										ui.Text(c, "预期 "+taskDuration(tEst)).FontSize(10.5).TextColor(muted).SingleLine()
									})
								}
								ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
									ui.Box(c).Size(8, 3).Radius(2).Background(ui.Hex("#b8532a"))
									ui.Text(c, "超 +"+taskDuration(tElapsed-tEst)).FontSize(10.5).TextColor(muted).SingleLine()
								})
							})
							summary := taskDuration(tElapsed)
							if tEst > 0 {
								summary = fmt.Sprintf("%s (%s)", summary, taskOverrunRatio(tElapsed, tEst))
							}
							ui.Text(c, summary).FontSize(11).FontWeight(500).TextColor(ui.Hex("#b8532a")).SingleLine()
							return
						}
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								ui.Box(c).Size(8, 3).Radius(2).Background(estSwatchColor)
								ui.Text(c, "预期 "+taskDuration(tEst)).FontSize(10.5).TextColor(muted).SingleLine()
							})
							if task.timer.State == richtext.TimerPaused {
								ui.Text(c, "剩余 "+taskDuration(tEst-tElapsed)).FontSize(10.5).TextColor(muted).SingleLine()
							}
						})
						if task.timer.State == richtext.TimerPaused {
							ui.Text(c, taskDuration(tElapsed)+" · 已暂停").FontSize(11).TextColor(muted).SingleLine()
						} else if tEst > 0 {
							pct := int(tElapsed * 100 / tEst)
							ui.Text(c, fmt.Sprintf("%s (%d%%)", taskDuration(tElapsed), pct)).FontSize(11).TextColor(ui.Hex("#6f6e69")).SingleLine()
						} else {
							ui.Text(c, taskDuration(tElapsed)).FontSize(11).TextColor(ui.Hex("#6f6e69")).SingleLine()
						}
					})
				})
			}
		})
	})
	if panel.OverlayShortcut(0, ui.KeyEscape) {
		a.floatingTasksOpen = false
	}
}
