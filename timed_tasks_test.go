package main

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func timedTaskFixture(t *testing.T) (*app, *ui.Tester, string) {
	t.Helper()
	a, tt := commandTestApp(t)
	tt.Type("/ttodo")
	if a.slash == nil || len(matchingCommands(a.slash.Query)) != 1 {
		t.Fatal("/ttodo does not identify timed todo")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Type("阅读计时任务🌲")
	st := a.current().doc.Text().ParagraphStyleAt(0)
	if st.List != richtext.Tasks || st.Timer.State != richtext.TimerIdle || st.Checked {
		t.Fatalf("wrong initial task: %+v", st)
	}
	return a, tt, st.ID
}
func clickTask(t *testing.T, tt *ui.Tester, id, label string) {
	t.Helper()
	if err := tt.Click(label + "计时待办：" + id); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}
func taskStyle(t *testing.T, a *app, id string) richtext.ParagraphStyle {
	t.Helper()
	doc := a.current().doc
	start, ok := blockPosition(doc.Text(), id)
	if !ok {
		t.Fatal("task ID lost")
	}
	return doc.Text().ParagraphStyleAt(start)
}

func TestTimedTaskStartPauseResumeComplete(t *testing.T) {
	a, tt, id := timedTaskFixture(t)
	clickTask(t, tt, id, "开始")
	started := taskStyle(t, a, id)
	if started.Timer.State != richtext.TimerRunning {
		t.Fatal("start failed")
	}
	version := a.current().doc.Version()
	advanceTaskClock(tt, 12*time.Second)
	if !tt.HasText("00:12") {
		t.Fatalf("counter did not tick: %v", tt.Texts())
	}
	if a.current().doc.Version() != version {
		t.Fatal("each tick edits the document")
	}
	clickTask(t, tt, id, "暂停")
	paused := taskStyle(t, a, id)
	if paused.Timer.State != richtext.TimerPaused || paused.Timer.Elapsed(tt.Now()) != 12*time.Second {
		t.Fatalf("pause: %+v", paused.Timer)
	}
	advanceTaskClock(tt, 20*time.Minute)
	if !tt.HasText("00:12") {
		t.Fatal("pause time was counted")
	}
	clickTask(t, tt, id, "继续")
	advanceTaskClock(tt, 48*time.Second)
	if !tt.HasText("01:00") {
		t.Fatal("resume did not accumulate both sessions")
	}
	clickTask(t, tt, id, "完成")
	done := taskStyle(t, a, id)
	if !done.Checked || done.Timer.State != richtext.TimerCompleted || done.Timer.Elapsed(tt.Now()) != time.Minute {
		t.Fatalf("complete: %+v", done)
	}
	for _, label := range []string{"开始", "暂停", "继续", "完成", "废弃"} {
		if _, found := tt.Find(label + "计时待办：" + id); found {
			t.Fatal("finished task still has active controls")
		}
	}
	advanceTaskClock(tt, time.Hour)
	if !tt.HasText("01:00") || taskStyle(t, a, id).Timer != done.Timer {
		t.Fatal("completed timer kept running")
	}
	if a.current().doc.Text().String() != "阅读计时任务🌲" {
		t.Fatal("timer controls changed task text")
	}
	// Undo completes one action, without walking through counter ticks.
	tt.Key(ui.Cmd, ui.KeyZ)
	if st := taskStyle(t, a, id); st.Checked || st.Timer.State != richtext.TimerRunning {
		t.Fatal("completion undo failed")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	if taskStyle(t, a, id).Timer != done.Timer {
		t.Fatal("completion redo failed")
	}
}

func TestTimedTaskTerminateAndCheckbox(t *testing.T) {
	for _, paused := range []bool{false, true} {
		t.Run(map[bool]string{false: "running", true: "paused"}[paused], func(t *testing.T) {
			a, tt, id := timedTaskFixture(t)
			clickTask(t, tt, id, "开始")
			advanceTaskClock(tt, 27*time.Second)
			if paused {
				clickTask(t, tt, id, "暂停")
				advanceTaskClock(tt, 3*time.Minute)
			}
			clickTask(t, tt, id, "废弃")
			st := taskStyle(t, a, id)
			if st.Checked || st.Timer.State != richtext.TimerTerminated || st.Timer.Elapsed(tt.Now()) != 27*time.Second {
				t.Fatalf("termination: %+v", st)
			}
			advanceTaskClock(tt, time.Minute)
			if !tt.HasText("00:27") {
				t.Fatal("terminated timer kept running")
			}
			// A checkbox click reopens a finished/terminated task in paused state.
			ed, _ := tt.Find("笔记正文")
			box := a.current().doc.VisibleParagraphs()[0]
			tt.ClickAt(ed.X+12, ed.Y+box.FirstLineCenter)
			if taskStyle(t, a, id).Timer.State != richtext.TimerPaused {
				t.Fatal("checkbox did not reopen terminated task")
			}
			clickTask(t, tt, id, "继续")
			advanceTaskClock(tt, 3*time.Second)
			ed, _ = tt.Find("笔记正文")
			box = a.current().doc.VisibleParagraphs()[0]
			tt.ClickAt(ed.X+12, ed.Y+box.FirstLineCenter)
			st = taskStyle(t, a, id)
			if !st.Checked || st.Timer.State != richtext.TimerCompleted || st.Timer.Elapsed(tt.Now()) != 30*time.Second {
				t.Fatal("checkbox did not stop and complete running timer")
			}
		})
	}
	a, tt, id := timedTaskFixture(t)
	ed, _ := tt.Find("笔记正文")
	box := a.current().doc.VisibleParagraphs()[0]
	tt.ClickAt(ed.X+12, ed.Y+box.FirstLineCenter)
	if st := taskStyle(t, a, id); !st.Checked || st.Timer.Elapsed(tt.Now()) != 0 {
		t.Fatal("idle completion failed")
	}
}

func TestTimedTaskPageSwitchPersistenceAndBackup(t *testing.T) {
	a, tt, id := timedTaskFixture(t)
	page := a.selected
	clickTask(t, tt, id, "开始")
	started := taskStyle(t, a, id).Timer
	a.addPage("")
	tt.Frame()
	tt.Type("另一页")
	advanceTaskClock(tt, 63*time.Second)
	a.selectPage(page)
	tt.Frame()
	if !tt.HasText("01:03") {
		t.Fatal("background timer stopped")
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	reopened.selectPage(page)
	st := taskStyle(t, reopened, id)
	if st.Timer != started || st.Timer.Elapsed(tt.Now()) != 63*time.Second {
		t.Fatal("running timer lost on restart")
	}
	clickTask(t, tt, id, "暂停")
	snapshot := taskStyle(t, a, id)
	path, err := a.store.Backup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dest := testStore(t)
	_, err = dest.RestoreBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := dest.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range pages {
		for _, para := range p.Body.Text.Paragraphs() {
			if para.Style.Timer == snapshot.Timer {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("backup lost timed task")
	}
	reopened, err = newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	reopened.selectPage(page)
	if taskStyle(t, reopened, id).Timer != snapshot.Timer {
		t.Fatal("paused state lost")
	}
}

func TestTimedTaskEnterAndBlockOperations(t *testing.T) {
	for _, where := range []string{"start", "middle", "end"} {
		t.Run(where, func(t *testing.T) {
			a, tt, id := timedTaskFixture(t)
			clickTask(t, tt, id, "开始")
			advanceTaskClock(tt, 5*time.Second)
			before := a.current().doc.Text()
			doc := a.current().doc
			pos := doc.Text().Len()
			if where == "start" {
				pos = 0
			}
			if where == "middle" {
				pos = 2
			}
			doc.SetSelection(pos, pos)
			a.editorHandle.Focus()
			tt.Frame()
			tt.Key(0, ui.KeyEnter)
			ps := menuParagraphs(doc.Text())
			if len(ps) != 2 {
				t.Fatal("Enter did not create block")
			}
			old, new := 0, 1
			if where == "start" {
				old, new = 1, 0
			}
			if ps[old].Style.ID != id || ps[old].Style.Timer.State != richtext.TimerRunning || ps[new].Style.Timer.State != richtext.TimerIdle || ps[new].Style.Checked {
				t.Fatal("new block inherited active timer, or original timer was lost")
			}
			tt.Key(ui.Cmd, ui.KeyZ)
			if !doc.Text().Equal(before) {
				t.Fatal("Enter undo lost timer")
			}
			openBlockMenu(t, a, tt, 0)
			clickBlockMenuItem(t, tt, "复制块")
			ps = menuParagraphs(doc.Text())
			if ps[0].Style.ID != id || ps[0].Style.Timer.State != richtext.TimerRunning || ps[1].Style.ID == id || ps[1].Style.Timer.State != richtext.TimerIdle {
				t.Fatal("duplicate did not create a fresh independent task")
			}
			copyID := ps[1].Style.ID
			openBlockMenu(t, a, tt, 1)
			clickBlockMenuItem(t, tt, "段落上移")
			if menuParagraphs(doc.Text())[1].Style.ID != id || taskStyle(t, a, id).Timer.State != richtext.TimerRunning {
				t.Fatal("move lost timer")
			}
			openBlockMenu(t, a, tt, 1)
			clickBlockMenuItem(t, tt, "删除块")
			if _, ok := blockPosition(doc.Text(), id); ok {
				t.Fatal("delete left running block")
			}
			tt.Key(ui.Cmd, ui.KeyZ)
			if taskStyle(t, a, id).Timer.State != richtext.TimerRunning {
				t.Fatal("delete undo did not restore timer")
			}
			if taskStyle(t, a, copyID).Timer.State != richtext.TimerIdle {
				t.Fatal("operations changed copied timer")
			}
		})
	}
}

func TestTimedTaskLayoutAndNormalTodo(t *testing.T) {
	for _, width := range []int{850, 1120} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a, tt, id := timedTaskFixture(t)
			tt.SetSize(width, 800)
			tt.Type(strings.Repeat("长文字不会盖住右侧计时按钮", 8))
			clickTask(t, tt, id, "开始")
			advanceTaskClock(tt, time.Hour+2*time.Minute+3*time.Second)
			if !tt.HasText("1:02:03") {
				t.Fatal("hour display failed")
			}
			controls, ok := tt.Find("计时待办操作：" + id)
			if !ok {
				t.Fatal("controls absent")
			}
			doc := a.current().doc
			doc.SetSelection(doc.Text().Len(), doc.Text().Len())
			a.editorHandle.Focus()
			tt.Frame()
			caret, ok := tt.TextCaret()
			if !ok || caret.X > controls.X {
				t.Fatalf("text overlaps controls: caret %+v, controls %+v", caret, controls)
			}
			clickTask(t, tt, id, "完成")
			timeBox, _ := tt.Find("累计用时：" + id)
			ed, _ := tt.Find("笔记正文")
			if timeBox.X+timeBox.W < ed.X+ed.W-2 {
				t.Fatal("completed duration is not at far right")
			}
			// Switching to an ordinary task removes timing controls and state.
			openBlockMenu(t, a, tt, 0)
			clickBlockMenuItem(t, tt, "转为待办列表")
			if taskStyle(t, a, id).Timer.Enabled() {
				t.Fatal("conversion to ordinary todo kept timer")
			}
			if _, found := tt.Find("计时待办操作：" + id); found {
				t.Fatal("ordinary todo has timing controls")
			}
		})
	}
}

func TestTimedTaskIconsFollowTextAndDurationStaysRight(t *testing.T) {
	for _, width := range []int{850, 1120} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a, tt, id := timedTaskFixture(t)
			tt.SetSize(width, 800)
			check := func(labels ...string) {
				t.Helper()
				doc := a.current().doc
				doc.SetSelection(doc.Text().Len(), doc.Text().Len())
				a.editorHandle.Focus()
				tt.Move(10, 10)
				tt.Frame()
				caret, ok := tt.TextCaret()
				if !ok {
					t.Fatal("missing text caret")
				}
				controls, ok := tt.Find("计时待办操作：" + id)
				if !ok || math.Abs(float64(controls.X-caret.X-8)) > 1 || math.Abs(float64(controls.Y+controls.H/2-caret.Y-caret.H/2)) > 1 {
					t.Fatalf("controls must follow text end: caret %+v, controls %+v", caret, controls)
				}
				for i, label := range labels {
					button, ok := tt.Find(label + "计时待办：" + id)
					if !ok || math.Abs(float64(button.W-24)) > 0.1 || math.Abs(float64(button.X-controls.X-float32(i*26))) > 1 {
						t.Fatalf("icons not compact: %s %+v", label, button)
					}
					if slices.Contains(tt.Texts(), label) {
						t.Fatalf("action shows visible text: %s", label)
					}
				}
				if timer := taskStyle(t, a, id).Timer; timer.State != richtext.TimerIdle {
					duration, _ := tt.Find("累计用时：" + id)
					ed, _ := tt.Find("笔记正文")
					if math.Abs(float64(duration.X+duration.W-ed.X-ed.W)) > 1 || controls.X+controls.W+8 > duration.X {
						t.Fatalf("duration must stay right without overlap: controls %+v, duration %+v", controls, duration)
					}
				}
			}
			check("开始")
			clickTask(t, tt, id, "开始")
			check("暂停", "完成", "废弃")
			tt.Type("，继续输入🌲")
			check("暂停", "完成", "废弃")
			tt.Type(strings.Repeat("这是一段会自动换行的中文任务", 5))
			check("暂停", "完成", "废弃")
			tt.Key(ui.Shift, ui.KeyEnter)
			tt.Type("手动换行后的内容")
			check("暂停", "完成", "废弃")
			clickTask(t, tt, id, "暂停")
			check("继续", "完成", "废弃")
		})
	}
}

func TestTimedTaskCommandClickAndCopyPaste(t *testing.T) {
	a, tt := commandTestApp(t)
	tt.Type("/ttodo")
	if err := tt.Click("命令：计时待办"); err != nil {
		t.Fatal(err)
	}
	tt.Type("复制计时块")
	d := a.current()
	id := d.doc.Text().ParagraphStyleAt(0).ID
	clickTask(t, tt, id, "开始")
	d.doc.SetSelection(0, d.doc.Text().Len())
	a.editorHandle.Focus()
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyC)
	sourceTimer := d.doc.Text().ParagraphStyleAt(0).Timer
	a.addPage("")
	tt.Frame()
	if err := tt.Click("笔记正文"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyV)
	pasted := a.current().doc.Text().ParagraphStyleAt(0)
	if pasted.ID == id || pasted.Timer.State != richtext.TimerIdle {
		t.Fatal("pasted a shared/running timer")
	}
	if d.doc.Text().ParagraphStyleAt(0).Timer != sourceTimer {
		t.Fatal("copy stopped source timer")
	}
	raw, err := json.Marshal(a.current().doc.Text())
	if err != nil {
		t.Fatal(err)
	}
	var restored richtext.Text
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ParagraphStyleAt(0).Timer != pasted.Timer {
		t.Fatal("timer JSON roundtrip failed")
	}
}

func advanceTaskClock(tt *ui.Tester, elapsed time.Duration) {
	tt.Advance(elapsed)
	tt.Frame()
}
