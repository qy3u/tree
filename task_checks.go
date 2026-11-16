package main

import (
	"context"
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
				changed = true
			}
			if now.UnixNano() >= check.Expires {
				if d := a.taskDraft(task); d != nil {
					a.timedTaskAction(d, task.block, "pause", time.Unix(0, check.Expires))
				}
				a.closeTaskNotice(key)
				delete(a.taskChecks, key)
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
	if changed {
		a.saveTaskChecks()
	}
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
		a.saveTaskChecks()
		return false
	}
	// Confirmation only advances the next check. It never restarts the timer,
	// splits its history, changes the selection, or adds an editor undo step.
	check.Due = now.Add(taskCheckInterval).UnixNano()
	check.Expires = 0
	check.Token = ""
	a.taskChecks[key] = check
	a.closeTaskNotice(key)
	a.saveTaskChecks()
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
	for key := range a.taskNotices {
		a.closeTaskNotice(key)
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

func (a *app) showTaskNotification(key, token, label string, expires time.Time) func() {
	if a.win.IsFocused() {
		return func() {}
	}
	if !mygo.NotificationsSupported() {
		a.revealWindow()
		return func() {}
	}
	n := mygo.NewNotification(mygo.NotificationOptions{ID: "tree-task-" + token, Title: "还在继续吗？", Subtitle: label, Body: "点击此通知表示「在继续」。30 秒内未确认将暂停计时。"})
	off := n.OnClick(func() { a.win.Update(func() { a.continueTimedTask(key, token, time.Now()) }) })
	ctx, cancel := context.WithDeadline(context.Background(), expires)
	// Authorization can await a system prompt; fall back to revealing the window if delivery fails.
	go func() {
		if err := n.ShowContext(ctx); err != nil && ctx.Err() == nil && a.win != nil && !a.win.IsDestroyed() {
			a.win.Update(func() {
				if check, ok := a.taskChecks[key]; ok && check.Token == token {
					a.revealWindow()
				}
			})
		}
	}()
	return func() { cancel(); off(); n.Close() }
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
	for _, key := range keys {
		check := a.taskChecks[key]
		task := tasks[key]
		if task.timer.StartedAtNS != check.StartedAt {
			continue
		}
		ui.Column(c.Key(key)).Label("计时继续确认").Padding(16).Gap(10).Background(paper).Border(1, line).Radius(10).Shadow(0, 4, 18, 0, ui.RGBA(0, 0, 0, .12)).Children(func() {
			ui.Text(c, "还在继续吗？").FontSize(16).Bold()
			ui.Text(c, task.label).FontSize(14).SingleLine().MinWidth(0)
			seconds := max(0, (check.Expires-c.Now().UnixNano()+int64(time.Second)-1)/int64(time.Second))
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, fmt.Sprintf("%d 秒后自动暂停", seconds)).FontSize(12).TextColor(muted).Grow(1)
				if ui.Button(c, "在继续").Label("继续计时：" + key).KeepFocus().Clicked() {
					a.continueTimedTask(key, check.Token, c.Now())
				}
			})
		})
	}
	c.After(time.Second)
}
