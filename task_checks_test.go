package main

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func checkFixture(t *testing.T) (*app, *draft, string, time.Time) {
	t.Helper()
	a, _, id := timedTaskFixture(t)
	d := a.current()
	start := time.Date(2026, 11, 6, 23, 50, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	a.timedTaskAction(d, id, "start", start)
	return a, d, id, start
}
func timerOf(d *draft, id string) richtext.TaskTimer {
	pos, _ := blockPosition(d.doc.Text(), id)
	return d.doc.Text().ParagraphStyleAt(pos).Timer
}
func checkKey(d *draft, id string) string {
	if d.parent != nil {
		return (runningTask{page: d.parent.page.ID, leaf: d.leafID, block: id}).key()
	}
	return (runningTask{page: d.page.ID, block: id}).key()
}

func TestTaskCheckContinueAndNextInterval(t *testing.T) {
	a, d, id, start := checkFixture(t)
	key := checkKey(d, id)
	shown, closed := 0, 0
	a.taskNotify = func(_, _, _ string, _ time.Time) func() { shown++; return func() { closed++ } }
	a.checkTimedTasks(start.Add(taskCheckInterval - time.Nanosecond))
	if shown != 0 {
		t.Fatal("notified early")
	}
	due := start.Add(taskCheckInterval)
	a.checkTimedTasks(due)
	check := a.taskChecks[key]
	if shown != 1 || check.Expires != due.Add(taskCheckGrace).UnixNano() {
		t.Fatal("missing thirty-second confirmation")
	}
	before, version := timerOf(d, id), d.doc.Version()
	a.checkTimedTasks(due.Add(time.Second))
	if shown != 1 {
		t.Fatal("notification repeated every tick")
	}
	if !a.continueTimedTask(key, check.Token, due.Add(4*time.Second)) {
		t.Fatal("confirmation rejected")
	}
	if timerOf(d, id) != before || d.doc.Version() != version || closed != 1 {
		t.Fatal("confirmation edited timer or did not close notice")
	}
	if a.continueTimedTask(key, check.Token, due.Add(4*time.Second)) {
		t.Fatal("double click accepted")
	}
	next := due.Add(4*time.Second + taskCheckInterval)
	a.checkTimedTasks(next.Add(-time.Nanosecond))
	if shown != 1 {
		t.Fatal("second reminder early")
	}
	a.checkTimedTasks(next)
	if shown != 2 {
		t.Fatal("second reminder absent")
	}
}

func TestTaskCheckTimeoutDailyHistoryAndLateClick(t *testing.T) {
	a, _, id := timedTaskFixture(t)
	d := a.current()
	midnight := time.Date(2026, 11, 7, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	firstDay := taskCheckInterval / 2
	secondDay := taskCheckInterval - firstDay + taskCheckGrace
	start := midnight.Add(-firstDay)
	a.timedTaskAction(d, id, "start", start)
	key := checkKey(d, id)
	due := start.Add(taskCheckInterval)
	a.checkTimedTasks(due)
	token := a.taskChecks[key].Token
	a.checkTimedTasks(due.Add(taskCheckGrace - time.Nanosecond))
	if timerOf(d, id).State != richtext.TimerRunning {
		t.Fatal("paused before deadline")
	}
	if a.continueTimedTask(key, token, due.Add(taskCheckGrace)) {
		t.Fatal("deadline click revived task")
	}
	timer := timerOf(d, id)
	if timer.State != richtext.TimerPaused || timer.ElapsedNS != int64(taskCheckInterval+taskCheckGrace) {
		t.Fatalf("incorrect timeout: %+v", timer)
	}
	history := d.page.Body.TaskTimes[timer.HistoryID]
	if len(history) != 2 || history[0].DurationNS != int64(firstDay) || history[1].DurationNS != int64(secondDay) {
		t.Fatalf("daily history incorrect: %+v", history)
	}
	a.checkTimedTasks(due.Add(time.Hour))
	if len(d.page.Body.TaskTimes[timer.HistoryID]) != 2 {
		t.Fatal("timeout recorded twice")
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	restored := reopened.draftForPage(d.page.ID)
	if timerOf(restored, id) != timer {
		t.Fatal("pause not saved")
	}
}

func TestTaskCheckDelayedWakeRestartAndConfirmationPersistence(t *testing.T) {
	a, d, id, start := checkFixture(t)
	key := checkKey(d, id)
	due := start.Add(taskCheckInterval)
	a.checkTimedTasks(due)
	a.continueTimedTask(key, a.taskChecks[key].Token, due.Add(2*time.Second))
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	reopened.checkTimedTasks(due.Add(taskCheckInterval / 2))
	if timerOf(reopened.draftForPage(d.page.ID), id).State != richtext.TimerRunning {
		t.Fatal("restart forgot confirmation")
	}
	late := start.Add(8 * time.Hour)
	reopened.checkTimedTasks(late)
	if timerOf(reopened.draftForPage(d.page.ID), id).State != richtext.TimerRunning || reopened.taskChecks[key].Expires != late.Add(taskCheckGrace).UnixNano() {
		t.Fatal("delayed check did not show thirty-second confirmation")
	}
	reopened.checkTimedTasks(late.Add(taskCheckGrace))
	timer := timerOf(reopened.draftForPage(d.page.ID), id)
	if timer.State != richtext.TimerPaused || timer.ElapsedNS != int64(8*time.Hour+taskCheckGrace) {
		t.Fatalf("delayed timeout incorrect: %+v", timer)
	}
}

func TestTaskCheckManualActionsInvalidateNotice(t *testing.T) {
	for _, action := range []string{"pause", "complete", "terminate"} {
		t.Run(action, func(t *testing.T) {
			a, d, id, start := checkFixture(t)
			key := checkKey(d, id)
			due := start.Add(taskCheckInterval)
			a.checkTimedTasks(due)
			token := a.taskChecks[key].Token
			a.timedTaskAction(d, id, action, due.Add(time.Second))
			if _, ok := a.taskChecks[key]; ok {
				t.Fatal("notice not canceled immediately")
			}
			if a.continueTimedTask(key, token, due.Add(2*time.Second)) {
				t.Fatal("stale notice accepted")
			}
			before := timerOf(d, id)
			a.checkTimedTasks(due.Add(10 * time.Second))
			if timerOf(d, id) != before {
				t.Fatal("timeout changed manually stopped task")
			}
		})
	}
}

func TestTaskCheckDeletedTaskAndResumedSession(t *testing.T) {
	a, d, id, start := checkFixture(t)
	key := checkKey(d, id)
	due := start.Add(taskCheckInterval)
	a.checkTimedTasks(due)
	token := a.taskChecks[key].Token
	a.timedTaskAction(d, id, "pause", due.Add(time.Second))
	a.timedTaskAction(d, id, "start", due.Add(2*time.Second))
	a.checkTimedTasks(due.Add(2 * time.Second))
	if a.continueTimedTask(key, token, due.Add(3*time.Second)) {
		t.Fatal("old notice confirmed new session")
	}
	if a.taskChecks[key].Due != due.Add(2*time.Second+taskCheckInterval).UnixNano() {
		t.Fatal("resume interval wrong")
	}
	editBlock(d.doc, 0, "delete")
	a.changed(d)
	a.checkTimedTasks(due.Add(time.Hour))
	if len(a.taskChecks) != 0 || d.doc.Text().Len() != 0 {
		t.Fatal("deleted timer resurrected")
	}
}

func TestTaskCheckInactivePageAndCollapsedLeaf(t *testing.T) {
	a, _, root, v := createTestLeaf(t)
	child := v.draft
	child.doc.InsertText("Leaf 计时")
	child.doc.Edit(func(e *richtext.Edit) {
		e.FormatParagraphs(0, 0, func(p *richtext.ParagraphStyle) {
			p.List = richtext.Tasks
			p.Timer = richtext.TaskTimer{State: richtext.TimerIdle}
		})
	})
	id := child.doc.Text().ParagraphStyleAt(0).ID
	start := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	a.timedTaskAction(child, id, "start", start)
	v.collapsed = true
	a.addPage("")
	selected := a.selected
	a.checkTimedTasks(start.Add(taskCheckInterval))
	if len(a.taskChecks) != 1 || a.selected != selected {
		t.Fatal("background Leaf not monitored or changed selected page")
	}
	a.checkTimedTasks(start.Add(taskCheckInterval + taskCheckGrace))
	if timerOf(child, id).State != richtext.TimerPaused {
		t.Fatal("collapsed Leaf still running")
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	owner := reopened.draftForPage(root.page.ID)
	loaded := reopened.leafView(owner, child.leafID).draft
	if timerOf(loaded, id).State != richtext.TimerPaused {
		t.Fatal("Leaf timeout not persisted")
	}
}

func TestTaskCheckNotificationButton(t *testing.T) {
	a, d, id, _ := checkFixture(t)
	// Use the test view's wall clock so a click falls inside the grace period.
	now := time.Now()
	d.doc.Edit(func(e *richtext.Edit) {
		e.FormatParagraphs(0, 0, func(p *richtext.ParagraphStyle) { p.Timer.StartedAtNS = now.Add(-taskCheckInterval).UnixNano() })
	})
	a.checkTimedTasks(now)
	key := checkKey(d, id)
	tt := ui.NewTester(a.view, 1120, 800)
	if _, ok := tt.Find("计时继续确认"); !ok {
		t.Fatal("confirmation popup absent")
	}
	if path := os.Getenv("TREE_TASK_CHECK_SCREENSHOT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err = png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click("继续计时：" + key); err != nil {
		t.Fatal(err)
	}
	if a.taskChecks[key].Expires != 0 || timerOf(d, id).State != richtext.TimerRunning {
		t.Fatal("continue button did not confirm")
	}
	if _, ok := tt.Find("计时继续确认"); ok {
		t.Fatal("confirmation popup remained")
	}
}

func TestTaskCheckMultipleIndependentTimers(t *testing.T) {
	a, d, id, start := checkFixture(t)
	a.addPage("")
	other := a.current()
	other.doc.Insert(richtext.TextOf(richtext.Paragraph{Style: richtext.ParagraphStyle{ID: "second-task", List: richtext.Tasks, Timer: richtext.TaskTimer{State: richtext.TimerIdle}}, Runs: []richtext.Run{{Text: "另一个任务"}}}))
	secondID := other.doc.Text().ParagraphStyleAt(0).ID
	if !a.timedTaskAction(other, secondID, "start", start) {
		t.Fatal("second timer did not start")
	}
	due := start.Add(taskCheckInterval)
	a.checkTimedTasks(due)
	if len(a.taskChecks) != 2 {
		t.Fatal("concurrent timer checks missing")
	}
	key := checkKey(d, id)
	if !a.continueTimedTask(key, a.taskChecks[key].Token, due.Add(time.Second)) {
		t.Fatal("first timer confirmation failed")
	}
	a.checkTimedTasks(due.Add(taskCheckGrace))
	if timerOf(d, id).State != richtext.TimerRunning || timerOf(other, secondID).State != richtext.TimerPaused {
		t.Fatal("confirming one timer affected another")
	}
}

func TestTaskCheckPendingRestartKeepsDeadline(t *testing.T) {
	a, d, id, start := checkFixture(t)
	due := start.Add(taskCheckInterval)
	a.checkTimedTasks(due)
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	reopened.checkTimedTasks(due.Add(4 * time.Second))
	key := checkKey(d, id)
	if reopened.taskChecks[key].Expires != due.Add(taskCheckGrace).UnixNano() {
		t.Fatal("restart extended response window")
	}
	reopened.checkTimedTasks(due.Add(30 * time.Minute))
	if got := timerOf(reopened.draftForPage(d.page.ID), id); got.State != richtext.TimerPaused || got.ElapsedNS != int64(taskCheckInterval+taskCheckGrace) {
		t.Fatal("pending restart lost timeout")
	}
}

func TestTaskCheckBackupKeepsConfirmedTime(t *testing.T) {
	a, d, id, start := checkFixture(t)
	due := start.Add(taskCheckInterval)
	key := checkKey(d, id)
	a.checkTimedTasks(due)
	a.continueTimedTask(key, a.taskChecks[key].Token, due.Add(time.Second))
	path, err := a.store.Backup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := testStore(t)
	if _, err = store.RestoreBackup(path); err != nil {
		t.Fatal(err)
	}
	restored, err := newApp(store)
	if err != nil {
		t.Fatal(err)
	}
	restored.checkTimedTasks(due.Add(taskCheckInterval / 2))
	if len(restored.runningTasks()) != 1 {
		t.Fatal("backup lost confirmation and truncated already confirmed time")
	}
	restored.checkTimedTasks(start.Add(time.Hour))
	restored.checkTimedTasks(start.Add(time.Hour + taskCheckGrace))
	for _, p := range restored.pages {
		for _, para := range p.Body.Text.Paragraphs() {
			if para.Style.ID == id {
				if para.Style.Timer.ElapsedNS != int64(time.Hour+taskCheckGrace) {
					t.Fatal("restored deadline wrong")
				}
				return
			}
		}
	}
	t.Fatal("restored task missing")
}

func TestTaskCheckLeafBackupRemapsPendingNotice(t *testing.T) {
	a, _, _, v := createTestLeaf(t)
	d := v.draft
	d.doc.Edit(func(e *richtext.Edit) {
		e.FormatParagraphs(0, 0, func(p *richtext.ParagraphStyle) {
			p.List = richtext.Tasks
			p.Timer = richtext.TaskTimer{State: richtext.TimerIdle}
		})
	})
	id := d.doc.Text().ParagraphStyleAt(0).ID
	start := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	due := start.Add(taskCheckInterval)
	a.timedTaskAction(d, id, "start", start)
	a.checkTimedTasks(due)
	original := a.taskChecks[checkKey(d, id)]
	path, err := a.store.Backup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.store.RestoreBackup(path); err != nil {
		t.Fatal(err)
	}
	restored, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	restored.checkTimedTasks(due.Add(time.Second))
	if len(restored.taskChecks) != 2 {
		t.Fatal("Leaf check did not follow imported identities")
	}
	seen := map[string]bool{}
	for _, check := range restored.taskChecks {
		if seen[check.Token] || check.Expires != original.Expires {
			t.Fatal("restored notice reused token or extended deadline")
		}
		seen[check.Token] = true
	}
}
