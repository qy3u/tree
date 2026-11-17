package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

const taskCheckInterval = 20 * time.Minute
const taskCheckGrace = 30 * time.Second
const taskCheckSetting = "timedTaskChecks"

type taskCheck struct {
	StartedAt int64  `json:"startedAt"`
	Due       int64  `json:"due"`
	Expires   int64  `json:"expires,omitempty"`
	Token     string `json:"token,omitempty"`
}
type runningTask struct {
	page, leaf, block, label string
	timer                    richtext.TaskTimer
}

func (r runningTask) key() string { return r.page + "/" + r.leaf + "/" + r.block }

func (a *app) loadTaskChecks() {
	_ = json.Unmarshal([]byte(a.store.Setting(taskCheckSetting)), &a.taskChecks)
	if a.taskChecks == nil {
		a.taskChecks = map[string]taskCheck{}
	}
	a.taskNotices = map[string]func(){}
	a.taskNotifyDismissed = map[string]string{}
	a.taskNotifyWatering = map[string]taskWateringAnim{}
}
func (a *app) saveTaskChecks() {
	raw, err := json.Marshal(a.taskChecks)
	if err == nil {
		err = a.store.SetSetting(taskCheckSetting, string(raw))
	}
	a.report(err)
}

// Read live drafts where present, including collapsed Leaves, without changing
// the selected page or loading an editor for every note in the library.
func (a *app) runningTasks() map[string]runningTask {
	out := map[string]runningTask{}
	for _, p := range a.pages {
		if p.Deleted != "" {
			continue
		}
		body := p.Body
		d := a.drafts[p.ID]
		if d != nil {
			body = d.page.Body
			body.Text = d.doc.Text()
		}
		add := func(leaf string, b Body) {
			for _, para := range b.Text.Paragraphs() {
				if para.Style.Timer.State != richtext.TimerRunning {
					continue
				}
				label := strings.TrimSpace(richtext.TextOf(para).PlainText())
				if label == "" {
					label = "未命名计时待办"
				}
				task := runningTask{p.ID, leaf, para.Style.ID, label, para.Style.Timer}
				out[task.key()] = task
			}
		}
		add("", body)
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
			if d != nil {
				if v := d.leafViews[id]; v != nil {
					content = v.draft.page.Body
					content.Text = v.draft.doc.Text()
				}
			}
			add(id, content)
		}
	}
	return out
}
func (a *app) taskDraft(r runningTask) *draft {
	d := a.draftForPage(r.page)
	if d != nil && r.leaf != "" {
		if v := a.leafView(d, r.leaf); v != nil {
			return v.draft
		}
		return nil
	}
	return d
}
func (a *app) closeTaskNotice(key string) {
	if close := a.taskNotices[key]; close != nil {
		close()
	}
	delete(a.taskNotices, key)
}

// Called on the window's update thread, never from a paint-dependent timer.
// Expired tasks stop at their deadline even after suspend or a delayed callback.
func (a *app) checkTimedTasks(now time.Time) {
	tasks := a.runningTasks()
	changed := false
	for key, check := range a.taskChecks {
		task, ok := tasks[key]
		if !ok || task.timer.StartedAtNS != check.StartedAt {
			a.closeTaskNotice(key)
			delete(a.taskChecks, key)
			delete(a.taskNotifyDismissed, key)
			delete(a.taskNotifyWatering, key)
			changed = true
		}
	}
	for key, task := range tasks {
		check, ok := a.taskChecks[key]
		if !ok || check.Due < check.StartedAt || check.Expires != 0 && check.Expires < check.Due {
			check = taskCheck{StartedAt: task.timer.StartedAtNS, Due: task.timer.StartedAtNS + int64(taskCheckInterval)}
			changed = true
		}
		if now.UnixNano() >= check.Due {
			if check.Expires == 0 {
				check.Expires = now.Add(taskCheckGrace).UnixNano()
				check.Token = newID()
				delete(a.taskNotifyWatering, key)
				changed = true
			}
			if now.UnixNano() >= check.Expires {
				if d := a.taskDraft(task); d != nil {
					a.timedTaskAction(d, task.block, "pause", time.Unix(0, check.Expires))
				}
				a.closeTaskNotice(key)
				delete(a.taskChecks, key)
				delete(a.taskNotifyDismissed, key)
				delete(a.taskNotifyWatering, key)
				changed = true
				continue
			}
			if _, shown := a.taskNotices[key]; !shown {
				close := func() {}
				if a.taskNotify != nil {
					close = a.taskNotify(key, check.Token, task.label, time.Unix(0, check.Expires))
				}
				a.taskNotices[key] = close
			}
		}
		a.taskChecks[key] = check
	}
	a.pruneExpiredWatering(now)
	if changed {
		a.saveTaskChecks()
	}
	a.syncTaskNotifyPanel()
}

func (a *app) startTaskWatering(key, label string, startFrac float32, now time.Time) {
	if a.taskNotifyWatering == nil {
		a.taskNotifyWatering = map[string]taskWateringAnim{}
	}
	ends := now.Add(taskNotifyWaterDuration)
	a.taskNotifyWatering[key] = taskWateringAnim{
		Label:     label,
		Started:   now.UnixNano(),
		Ends:      ends.UnixNano(),
		StartFrac: clamp01(startFrac),
	}
	a.scheduleTaskWaterCleanup()
}

func (a *app) pruneExpiredWatering(now time.Time) {
	if len(a.taskNotifyWatering) == 0 {
		return
	}
	nowNS := now.UnixNano()
	for key, anim := range a.taskNotifyWatering {
		if nowNS >= anim.Ends {
			delete(a.taskNotifyWatering, key)
		}
	}
}

func (a *app) scheduleTaskWaterCleanup() {
	if a.win == nil || a.win.IsDestroyed() || a.taskCheckStopped {
		return
	}
	if a.taskWaterTimer != nil {
		a.taskWaterTimer.Stop()
		a.taskWaterTimer = nil
	}
	var nextEnd int64
	for _, anim := range a.taskNotifyWatering {
		if nextEnd == 0 || anim.Ends < nextEnd {
			nextEnd = anim.Ends
		}
	}
	if nextEnd == 0 {
		return
	}
	delay := max(10*time.Millisecond, time.Until(time.Unix(0, nextEnd))+25*time.Millisecond)
	a.taskWaterTimer = time.AfterFunc(delay, func() {
		if a.win == nil || a.win.IsDestroyed() || a.taskCheckStopped {
			return
		}
		a.win.Update(func() {
			a.pruneExpiredWatering(time.Now())
			a.scheduleTaskWaterCleanup()
			a.syncTaskNotifyPanel()
		})
	})
}

func (a *app) continueTimedTask(key, token string, now time.Time) bool {
	check, ok := a.taskChecks[key]
	if !ok || token == "" || check.Token != token || check.Expires == 0 {
		return false
	}
	if now.UnixNano() >= check.Expires {
		a.checkTimedTasks(now)
		return false
	}
	task, ok := a.runningTasks()[key]
	if !ok || task.timer.StartedAtNS != check.StartedAt {
		a.closeTaskNotice(key)
		delete(a.taskChecks, key)
		delete(a.taskNotifyWatering, key)
		a.saveTaskChecks()
		return false
	}
	startFrac := clamp01(float32(check.Expires-now.UnixNano()) / float32(taskCheckGrace))
	// Confirmation only advances the next check. It never restarts the timer,
	// splits its history, changes the selection, or adds an editor undo step.
	check.Due = now.Add(taskCheckInterval).UnixNano()
	check.Expires = 0
	check.Token = ""
	a.taskChecks[key] = check
	delete(a.taskNotifyDismissed, key)
	a.closeTaskNotice(key)
	a.startTaskWatering(key, task.label, startFrac, now)
	a.saveTaskChecks()
	a.syncTaskNotifyPanel()
	return true
}

func (a *app) startTaskChecks() {
	a.taskCheckStopped = false
	var tick func()
	tick = func() {
		if a.taskCheckStopped {
			return
		}
		now := time.Now()
		a.checkTimedTasks(now)
		delay := time.Second
		for _, check := range a.taskChecks {
			at := check.Due
			if check.Expires != 0 {
				at = check.Expires
			}
			delay = min(delay, max(time.Millisecond, time.Until(time.Unix(0, at))))
		}
		a.taskCheckTimer = time.AfterFunc(delay, func() { a.win.Update(tick) })
	}
	tick()
}
func (a *app) stopTaskChecks() {
	a.taskCheckStopped = true
	if a.taskCheckTimer != nil {
		a.taskCheckTimer.Stop()
	}
	if a.taskWaterTimer != nil {
		a.taskWaterTimer.Stop()
		a.taskWaterTimer = nil
	}
	a.taskNotifyWatering = map[string]taskWateringAnim{}
	for key := range a.taskNotices {
		a.closeTaskNotice(key)
	}
	if a.taskNotifyPanel != nil && !a.taskNotifyPanel.IsDestroyed() {
		a.taskNotifyPanel.Destroy()
		a.taskNotifyPanel = nil
	}
}

func (a *app) revealWindow() {
	if a.win == nil || a.win.IsDestroyed() {
		return
	}
	mygo.App.Show()
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	mygo.App.Focus()
	a.win.Focus()
}

const (
	taskNotifyPanelWidth = 340
	taskNotifyCardHeight = 104
)

func taskNoticePanelBounds(disp mygo.Display, itemCount int) mygo.Rectangle {
	if itemCount < 1 {
		itemCount = 1
	}
	area := disp.WorkArea
	if area.Width <= 0 || area.Height <= 0 {
		area = disp.Bounds
	}
	if area.Width <= 0 || area.Height <= 0 {
		return mygo.Rectangle{X: 16, Y: 14, Width: taskNotifyPanelWidth, Height: taskNotifyCardHeight}
	}
	w := min(taskNotifyPanelWidth, max(240, area.Width-32))
	desiredH := itemCount*taskNotifyCardHeight + max(0, itemCount-1)
	h := min(desiredH, max(taskNotifyCardHeight, area.Height-32))
	x := area.X + area.Width - w - 16
	y := area.Y + 14
	return mygo.Rectangle{X: x, Y: y, Width: w, Height: h}
}

func shouldShowExternalTaskNoticeState(focused, visible, minimized bool, winDisplayID, cursorDisplayID int64) bool {
	if !focused || !visible || minimized {
		return true
	}
	if winDisplayID != 0 && cursorDisplayID != 0 && winDisplayID != cursorDisplayID {
		return true
	}
	return false
}

func (a *app) shouldShowExternalTaskNotice() bool {
	if a.win == nil || a.win.IsDestroyed() {
		return false
	}
	focused := a.win.IsFocused()
	visible := a.win.IsVisible()
	minimized := a.win.IsMinimized()
	var winDisplayID, cursorDisplayID int64
	if focused && visible && !minimized {
		winDisplayID = mygo.Screen.DisplayMatching(a.win.Bounds()).ID
		cursorDisplayID = mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint()).ID
	}
	return shouldShowExternalTaskNoticeState(focused, visible, minimized, winDisplayID, cursorDisplayID)
}

func (a *app) externalTaskNoticeKeysAt(now time.Time) ([]string, map[string]runningTask) {
	a.pruneExpiredWatering(now)
	keys, tasks := a.pendingTaskCheckKeys()
	seen := map[string]bool{}
	visible := make([]string, 0, len(keys)+len(a.taskNotifyWatering))
	for _, key := range keys {
		check := a.taskChecks[key]
		if a.taskNotifyDismissed[key] == check.Token && check.Token != "" {
			continue
		}
		seen[key] = true
		visible = append(visible, key)
	}
	if len(a.taskNotifyWatering) > 0 {
		if tasks == nil {
			tasks = a.runningTasks()
		}
		for key := range a.taskNotifyWatering {
			if seen[key] {
				continue
			}
			if _, ok := tasks[key]; !ok {
				delete(a.taskNotifyWatering, key)
				continue
			}
			seen[key] = true
			visible = append(visible, key)
		}
	}
	if len(visible) == 0 {
		return nil, nil
	}
	sort.Strings(visible)
	return visible, tasks
}

func (a *app) externalTaskNoticeKeys() ([]string, map[string]runningTask) {
	return a.externalTaskNoticeKeysAt(time.Now())
}

func (a *app) inWindowTaskNotices(now time.Time) ([]string, map[string]runningTask) {
	a.pruneExpiredWatering(now)
	keys, tasks := a.pendingTaskCheckKeys()
	if len(a.taskNotifyWatering) == 0 {
		return keys, tasks
	}
	if tasks == nil {
		tasks = a.runningTasks()
	}
	seen := make(map[string]bool, len(keys))
	out := make([]string, 0, len(keys)+len(a.taskNotifyWatering))
	for _, key := range keys {
		seen[key] = true
		out = append(out, key)
	}
	for key := range a.taskNotifyWatering {
		if seen[key] {
			continue
		}
		if _, ok := tasks[key]; !ok {
			delete(a.taskNotifyWatering, key)
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Strings(out)
	return out, tasks
}

func (a *app) dismissTaskNotifyKey(key string) {
	delete(a.taskNotifyWatering, key)
	if check, ok := a.taskChecks[key]; ok && check.Token != "" {
		if a.taskNotifyDismissed == nil {
			a.taskNotifyDismissed = map[string]string{}
		}
		a.taskNotifyDismissed[key] = check.Token
	}
	a.syncTaskNotifyPanel()
}

func (a *app) dismissAllTaskNotifyPanels() {
	keys, _ := a.externalTaskNoticeKeys()
	for _, key := range keys {
		delete(a.taskNotifyWatering, key)
		if check, ok := a.taskChecks[key]; ok && check.Token != "" {
			if a.taskNotifyDismissed == nil {
				a.taskNotifyDismissed = map[string]string{}
			}
			a.taskNotifyDismissed[key] = check.Token
		}
	}
	a.syncTaskNotifyPanel()
}

func (a *app) ensureTaskNotifyPanel() *mygo.Window {
	if a.taskNotifyPanel != nil && !a.taskNotifyPanel.IsDestroyed() {
		return a.taskNotifyPanel
	}
	p := mygo.NewWindow(mygo.WindowOptions{
		Title:         "计时确认",
		Panel:         true,
		Hidden:        true,
		Frameless:     true,
		Transparent:   true,
		CornerRadius:  12,
		AlwaysOnTop:   true,
		DisableResize: true,
		DisableMove:   true,
		SkipTaskbar:   true,
		Width:         taskNotifyPanelWidth,
		Height:        taskNotifyCardHeight,
		Content:       ui.View(a.taskNotifyView),
	})
	p.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		a.dismissAllTaskNotifyPanels()
	})
	a.taskNotifyPanel = p
	return p
}

func (a *app) syncTaskNotifyPanel() {
	if a.win == nil || a.win.IsDestroyed() {
		return
	}
	keys, _ := a.externalTaskNoticeKeys()
	if len(keys) == 0 || !a.shouldShowExternalTaskNotice() {
		if a.taskNotifyPanel != nil && !a.taskNotifyPanel.IsDestroyed() && a.taskNotifyPanel.IsVisible() {
			a.taskNotifyPanel.Hide()
		}
		return
	}
	p := a.ensureTaskNotifyPanel()
	wasVisible := p.IsVisible()
	var disp mygo.Display
	if !wasVisible {
		disp = mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	} else {
		disp = mygo.Screen.DisplayMatching(p.Bounds())
	}
	bounds := taskNoticePanelBounds(disp, len(keys))
	if !wasVisible || bounds != p.Bounds() {
		p.SetBounds(bounds)
	}
	p.Update(func() {})
	if !wasVisible {
		p.ShowInactive()
	}
}

func (a *app) showTaskNotification(key, token, label string, expires time.Time) func() {
	a.syncTaskNotifyPanel()
	return func() {
		delete(a.taskNotifyDismissed, key)
		a.syncTaskNotifyPanel()
	}
}

func (a *app) renderTaskEcoNoticeCard(c *ui.Context, key string, task runningTask, check taskCheck, anim *taskWateringAnim, now time.Time, external bool) {
	watering := anim != nil
	var remainFrac float32
	var seconds int64
	if !watering && check.Expires != 0 {
		seconds = max(0, (check.Expires-now.UnixNano()+int64(time.Second)-1)/int64(time.Second))
		remainFrac = clamp01(float32(check.Expires-now.UnixNano()) / float32(taskCheckGrace))
	}
	var animCopy *taskWateringAnim
	if watering {
		copyVal := *anim
		animCopy = &copyVal
	}

	ui.Row(c.Key("card:"+key)).Padding(11, 13).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Box(c.Key("eco:"+key)).Size(62, 62).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			paintEcoWateringBadge(p, r, remainFrac, animCopy, p.Now())
		})

		ui.Column(c.Key("body:"+key)).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Row(c).Height(20).Gap(6).AlignItems(ui.Center).Children(func() {
				if watering {
					ui.Text(c, "已浇水 · 继续生长中").FontSize(13).Bold().TextColor(ui.Hex("#256f47")).Grow(1).SingleLine()
				} else {
					ui.Text(c, "还在继续吗？").FontSize(13.5).Bold().TextColor(ink).Grow(1).SingleLine()
				}
				if external {
					if quietButton(c.Key("dismiss:"+key), "").Label("关闭提醒：" + key).Tooltip("暂时收起提醒").
						Size(20, 20).Padding(3).Justify(ui.Center).AlignItems(ui.Center).TextColor(muted).
						Children(func() { ui.Icon(c, icons["close"]).FontSize(12) }).Clicked() {
						a.dismissTaskNotifyKey(key)
						if a.win != nil && !a.win.IsDestroyed() {
							a.win.Update(func() {})
						}
					}
				}
			})

			if external {
				titleBtn := quietButton(c.Key("ext-jump:"+key), "").Label("定位提醒待办：" + key).Tooltip("在 Tree 中查看").
					Height(20).Padding(0).AlignItems(ui.Center).
					Children(func() {
						ui.Text(c, task.label).FontSize(12.5).FontWeight(500).TextColor(ui.Hex("#343b37")).SingleLine().MinWidth(0)
					})
				if titleBtn.Clicked() {
					a.revealWindow()
					a.jumpToTimedTask(activeTimedTask{page: task.page, leaf: task.leaf, block: task.block, label: task.label, timer: task.timer})
					a.syncTaskNotifyPanel()
					if a.win != nil && !a.win.IsDestroyed() {
						a.win.Update(func() {})
					}
				}
			} else {
				ui.Box(c).Height(20).Justify(ui.Center).Children(func() {
					ui.Text(c, task.label).FontSize(12.5).FontWeight(500).TextColor(ui.Hex("#343b37")).SingleLine().MinWidth(0)
				})
			}

			ui.Row(c).Height(26).Margin(2, 0, 0, 0).Gap(6).AlignItems(ui.Center).Children(func() {
				if watering {
					ui.Text(c, "20 分钟后再次提醒").FontSize(11.5).TextColor(ui.Hex("#2f855a")).Grow(1).SingleLine()
					ui.Box(c.Key("watered-pill:"+key)).Label("已浇水确认：" + key).
						Height(26).Padding(0, 10).Radius(6).Background(ui.Hex("#e9f6ee")).Border(1, ui.Hex("#bde2ca")).
						Justify(ui.Center).AlignItems(ui.Center).
						Children(func() {
							ui.Text(c, "已确认 ✓").FontSize(12).FontWeight(500).TextColor(ui.Hex("#256f47")).SingleLine()
						})
					return
				}
				ui.Text(c, fmt.Sprintf("%d 秒后自动暂停", seconds)).FontSize(11.5).TextColor(muted).Grow(1).SingleLine()
				pauseLabel := "页面内暂停计时：" + key
				pauseKey := "in-pause:" + key
				contLabel := "继续计时：" + key
				contKey := "in-continue:" + key
				if external {
					pauseLabel = "通知暂停计时：" + key
					pauseKey = "ext-pause:" + key
					contLabel = "通知继续计时：" + key
					contKey = "ext-continue:" + key
				}
				pauseBtn := ui.Button(c.Key(pauseKey), "暂停").Label(pauseLabel).
					FontSize(12).Height(26).Padding(0, 10).Radius(6).Background(sidebarColor).TextColor(ink).Border(1, line)
				if !external {
					pauseBtn = pauseBtn.KeepFocus()
				}
				if pauseBtn.Clicked() {
					a.runActiveTimedTaskAction(activeTimedTask{page: task.page, leaf: task.leaf, block: task.block, label: task.label, timer: task.timer}, "pause", now)
					a.syncTaskNotifyPanel()
					if a.win != nil && !a.win.IsDestroyed() {
						a.win.Update(func() {})
					}
				}
				contBtn := ui.Button(c.Key(contKey), "在继续").Label(contLabel).
					FontSize(12).FontWeight(500).Height(26).Padding(0, 12).Radius(6).Background(ui.Hex("#2f855a")).TextColor(paper).Border(0, line)
				if !external {
					contBtn = contBtn.KeepFocus()
				}
				if contBtn.Clicked() {
					a.continueTimedTask(key, check.Token, now)
					if a.win != nil && !a.win.IsDestroyed() {
						a.win.Update(func() {})
					}
				}
			})
		})
	})
}

func (a *app) taskNotifyView(c *ui.Context) {
	now := c.Now()
	keys, tasks := a.externalTaskNoticeKeysAt(now)
	if len(keys) == 0 {
		ui.Box(c)
		return
	}
	c.After(time.Second)
	anyWatering := false
	for _, key := range keys {
		check := a.taskChecks[key]
		if check.Expires == 0 {
			if _, ok := a.taskNotifyWatering[key]; ok {
				anyWatering = true
				break
			}
		}
	}
	borderColor := line
	if anyWatering {
		borderColor = ui.Hex("#b7dec3")
	}
	ui.Column(c.Key("external-task-notices")).Label("悬浮屏计时确认通知").Fill().Background(paper).Border(1, borderColor).Radius(12).Clip().Children(func() {
		ui.Scroll(c.Key("external-task-notices-scroll")).Grow(1).MinHeight(0).Children(func() {
			first := true
			for _, key := range keys {
				check := a.taskChecks[key]
				task := tasks[key]
				var animPtr *taskWateringAnim
				if check.Expires == 0 {
					if anim, ok := a.taskNotifyWatering[key]; ok {
						animPtr = &anim
					} else {
						continue
					}
				} else if task.timer.StartedAtNS != check.StartedAt {
					continue
				}
				if !first {
					ui.Box(c).Height(1).Background(line)
				}
				first = false
				a.renderTaskEcoNoticeCard(c, key, task, check, animPtr, now, true)
			}
		})
	})
}

func (a *app) pendingTaskCheckKeys() ([]string, map[string]runningTask) {
	pending := false
	for _, check := range a.taskChecks {
		if check.Expires != 0 {
			pending = true
			break
		}
	}
	if !pending {
		return nil, nil
	}
	tasks := a.runningTasks()
	keys := []string{}
	for key, check := range a.taskChecks {
		if _, ok := tasks[key]; ok && check.Expires != 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil, nil
	}
	return keys, tasks
}

func (a *app) renderTaskCheckNotices(c *ui.Context, keys []string, tasks map[string]runningTask) {
	now := c.Now()
	for _, key := range keys {
		check := a.taskChecks[key]
		task := tasks[key]
		var animPtr *taskWateringAnim
		if check.Expires == 0 {
			if anim, ok := a.taskNotifyWatering[key]; ok {
				animPtr = &anim
			} else {
				continue
			}
		} else if task.timer.StartedAtNS != check.StartedAt {
			continue
		}
		cardLabel := "计时继续确认"
		borderColor := line
		shadowColor := ui.RGBA(0, 0, 0, .12)
		if animPtr != nil {
			cardLabel = "计时浇水确认：" + key
			borderColor = ui.Hex("#b7dec3")
			shadowColor = ui.RGBA(47, 133, 90, .16)
		}
		ui.Box(c.Key(key)).Label(cardLabel).Background(paper).Border(1, borderColor).Radius(12).Shadow(0, 4, 18, 0, shadowColor).Children(func() {
			a.renderTaskEcoNoticeCard(c, key, task, check, animPtr, now, false)
		})
	}
	c.After(time.Second)
}
