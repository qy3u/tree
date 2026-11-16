package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/treerichtext"
	"github.com/egoist/mygo/ui"
)

func createFloatingTask(t *testing.T, a *app, tt *ui.Tester, title string, est time.Duration) string {
	t.Helper()
	tt.Type("/ttodo")
	tt.Key(0, ui.KeyEnter)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type(formatEstimateInput(est))
	tt.Key(0, ui.KeyEnter)
	tt.Type(title)
	d := a.current()
	p := menuParagraphs(d.doc.Text())
	return p[len(p)-1].Style.ID
}

func TestFloatingTimedTasksEmptyAndMultiSourceAggregation(t *testing.T) {
	a, tt := commandTestApp(t)
	if _, found := tt.Find("悬浮计时待办胶囊"); found {
		t.Fatal("capsule shown with no running/paused tasks")
	}

	page1 := a.selected
	idIdle := createFloatingTask(t, a, tt, "未开始任务", 25*time.Minute)
	if _, found := tt.Find("悬浮计时待办胶囊"); found {
		t.Fatal("capsule shown for idle task")
	}
	clickTask(t, tt, idIdle, "开始")
	advanceTaskClock(tt, 10*time.Second)
	clickTask(t, tt, idIdle, "暂停")
	page1TaskKey := runningTask{page: page1, block: idIdle}.key()

	// Exit todo list to a plain paragraph, then create a Leaf on page1 with a running task and collapse it.
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnter)
	a.createLeaf(a.current())
	tt.Frame()
	var lv *leafView
	for _, view := range a.current().leafViews {
		lv = view
		break
	}
	if lv == nil {
		t.Fatal("leaf not created")
	}
	lv.focusBody = true
	tt.Frame()
	tt.Type("/ttodo")
	tt.Key(0, ui.KeyEnter)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("20")
	tt.Key(0, ui.KeyEnter)
	tt.Type("Leaf 运行任务")
	leafBlockID := lv.draft.doc.Text().ParagraphStyleAt(0).ID
	a.timedTaskAction(lv.draft, leafBlockID, "start", tt.Now())
	lv.collapsed = true
	a.current().layoutRevision++
	tt.Frame()
	leafTaskKey := runningTask{page: page1, leaf: lv.draft.leafID, block: leafBlockID}.key()

	// Switch to a second page, create a newer running task, then switch to Library.
	a.addPage("")
	tt.Frame()
	if err := tt.Click("笔记正文"); err != nil {
		t.Fatal(err)
	}
	page2 := a.selected
	idNew := createFloatingTask(t, a, tt, "第二页进行中", 15*time.Minute)
	advanceTaskClock(tt, 2*time.Second)
	clickTask(t, tt, idNew, "开始")
	page2TaskKey := runningTask{page: page2, block: idNew}.key()

	a.selectPage(libraryRoute)
	tt.Frame()

	// Primary capsule should show most recently started running task and +2 badge.
	if _, found := tt.Find("悬浮计时待办胶囊"); !found {
		t.Fatal("capsule missing in Library view")
	}
	if _, found := tt.Find("定位计时待办：" + page2TaskKey); !found {
		t.Fatalf("expected newest running task in capsule, texts=%v", tt.Texts())
	}
	if !tt.HasText("+2") {
		t.Fatalf("expected +2 count badge on capsule, tasks=%+v texts=%v", a.activeTimedTasks(), tt.Texts())
	}

	// Expand list and verify all 3 running/paused tasks appear in order.
	if err := tt.Click("展开计时待办列表"); err != nil {
		t.Fatal(err)
	}
	if _, found := tt.Find("悬浮计时待办列表"); !found {
		t.Fatal("expanded list did not open")
	}
	for _, key := range []string{page2TaskKey, leafTaskKey, page1TaskKey} {
		if _, found := tt.Find("列表定位计时待办：" + key); !found {
			t.Fatalf("missing task in expanded list: %s", key)
		}
	}
}

func TestFloatingTimedTasksDesign1OverrunProgressAndRatio(t *testing.T) {
	a, tt := commandTestApp(t)
	page := a.selected
	id := createFloatingTask(t, a, tt, "定量超时对照", 15*time.Minute)
	key := runningTask{page: page, block: id}.key()
	clickTask(t, tt, id, "开始")

	// Normal progress (50%): expected segment exists, no overrun segment, no ratio chip, track inset inside capsule.
	advanceTaskClock(tt, 7*time.Minute+30*time.Second)
	capsule, ok := tt.Find("悬浮计时待办胶囊")
	if !ok {
		t.Fatal("capsule missing")
	}
	track, ok := tt.Find("悬浮计时进度：capsule:" + key)
	if !ok {
		t.Fatal("capsule progress track missing")
	}
	if track.X <= capsule.X || track.X+track.W >= capsule.X+capsule.W || track.Y+track.H >= capsule.Y+capsule.H {
		t.Fatalf("expected progress track inset inside capsule, got track=%+v capsule=%+v", track, capsule)
	}
	exp, ok := tt.Find("预期基准段：capsule:" + key)
	if !ok || exp.W < track.W*0.45 || exp.W > track.W*0.55 {
		t.Fatalf("expected 50%% bar width, got exp=%+v track=%+v", exp, track)
	}
	if _, found := tt.Find("超出延长段：capsule:" + key); found {
		t.Fatal("unexpected overrun segment before estimate")
	}

	// Slight overrun: 18m / 15m = 1.2x (expected ~83%, overrun ~17%), flush contact without ratio badge in capsule.
	advanceTaskClock(tt, 10*time.Minute+30*time.Second)
	if tt.HasText("1.2×") {
		t.Fatalf("unexpected ratio chip in capsule: %v", tt.Texts())
	}
	exp1, ok := tt.Find("预期基准段：capsule:" + key)
	over1, ok2 := tt.Find("超出延长段：capsule:" + key)
	if !ok || !ok2 || exp1.W <= over1.W || over1.X-exp1.X-exp1.W > 0.5 || exp1.X+exp1.W-over1.X > 0.5 {
		t.Fatalf("slight overrun segments mismatch: exp=%+v over=%+v", exp1, over1)
	}

	// Heavy overrun: 45m / 15m = 3.0x (expected ~33%, overrun ~67%).
	advanceTaskClock(tt, 27*time.Minute)
	if tt.HasText("3.0×") {
		t.Fatalf("unexpected ratio chip in capsule: %v", tt.Texts())
	}
	exp2, ok := tt.Find("预期基准段：capsule:" + key)
	over2, ok2 := tt.Find("超出延长段：capsule:" + key)
	if !ok || !ok2 || over2.W <= exp2.W || over2.X-exp2.X-exp2.W > 0.5 || exp2.X+exp2.W-over2.X > 0.5 {
		t.Fatalf("heavy overrun segments mismatch: exp=%+v over=%+v", exp2, over2)
	}

	// Expanded list shows Design 1 legend breakdown.
	if err := tt.Click("展开计时待办列表"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("预期 15:00") || !tt.HasText("超 +30:00") || !tt.HasText("45:00 (3.0×)") {
		t.Fatalf("missing expanded overrun legend: %v", tt.Texts())
	}
}

func TestFloatingTimedTasksActionsAcrossPagesAndLeaves(t *testing.T) {
	a, tt := commandTestApp(t)
	page1 := a.selected
	id1 := createFloatingTask(t, a, tt, "第一页任务", 25*time.Minute)
	clickTask(t, tt, id1, "开始")
	advanceTaskClock(tt, 15*time.Second)
	key1 := runningTask{page: page1, block: id1}.key()

	// Switch away to page 2, control page 1 task directly from capsule and expanded list.
	a.addPage("")
	tt.Frame()
	if err := tt.Click("悬浮暂停计时：" + key1); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	d1 := a.draftForPage(page1)
	pos1, _ := blockPosition(d1.doc.Text(), id1)
	if st := d1.doc.Text().ParagraphStyleAt(pos1); st.Timer.State != richtext.TimerPaused {
		t.Fatalf("pause from capsule failed: %+v", st.Timer)
	}

	// Expand and resume, then complete.
	if err := tt.Click("展开计时待办列表"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("悬浮继续计时：" + key1); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if st := d1.doc.Text().ParagraphStyleAt(pos1); st.Timer.State != richtext.TimerRunning {
		t.Fatalf("resume from expanded list failed: %+v", st.Timer)
	}
	if err := tt.Click("悬浮完成计时：" + key1); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if st := d1.doc.Text().ParagraphStyleAt(pos1); st.Timer.State != richtext.TimerCompleted || !st.Checked {
		t.Fatalf("complete from expanded list failed: %+v", st)
	}
	if _, found := tt.Find("悬浮计时待办胶囊"); found {
		t.Fatal("capsule still shown after completing last active task")
	}

	// Verify persistence after reopening store.
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	reopened.selectPage(page1)
	if st := reopened.current().doc.Text().ParagraphStyleAt(pos1); st.Timer.State != richtext.TimerCompleted {
		t.Fatalf("completion did not persist: %+v", st.Timer)
	}
}

func TestFloatingTimedTasksJumpAndNoticeCoexistence(t *testing.T) {
	a, tt := commandTestApp(t)
	page1 := a.selected
	a.current().page.Title = "工作日志"
	id1 := createFloatingTask(t, a, tt, "跳转目标任务", 25*time.Minute)
	key1 := runningTask{page: page1, block: id1}.key()
	clickTask(t, tt, id1, "开始")

	// Trigger pending task check notice and ensure it stacks below capsule without overlap.
	a.taskChecks[key1] = taskCheck{
		StartedAt: taskStyle(t, a, id1).Timer.StartedAtNS,
		Due:       tt.Now().Add(-time.Second).UnixNano(),
		Expires:   tt.Now().Add(20 * time.Second).UnixNano(),
		Token:     "tok-1",
	}
	tt.Frame()
	capsule, ok1 := tt.Find("悬浮计时待办胶囊")
	notice, ok2 := tt.Find("计时继续确认")
	if !ok1 || !ok2 {
		t.Fatalf("expected both capsule and check notice: ok1=%v ok2=%v", ok1, ok2)
	}
	if capsule.Y+capsule.H > notice.Y {
		t.Fatalf("capsule overlaps task check notice: capsule=%+v notice=%+v", capsule, notice)
	}

	// Switch to page 2 and create a Leaf with a paused task on page 1, then jump to both.
	a.addPage("")
	tt.Frame()
	if err := tt.Click("定位计时待办：" + key1); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.selected != page1 || a.blockJump.block != id1 {
		t.Fatalf("jump to page task failed: selected=%s jump=%+v", a.selected, a.blockJump)
	}

	// Now test jumping into a collapsed Leaf from another page.
	a.createLeaf(a.current())
	tt.Frame()
	var lv *leafView
	for _, view := range a.current().leafViews {
		lv = view
		break
	}
	lv.focusBody = true
	tt.Frame()
	tt.Type("/ttodo")
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnter)
	tt.Type("Leaf 跳转任务")
	leafBlockID := lv.draft.doc.Text().ParagraphStyleAt(0).ID
	a.timedTaskAction(lv.draft, leafBlockID, "start", tt.Now())
	lv.collapsed = true
	a.current().layoutRevision++
	a.addPage("")
	tt.Frame()

	leafKey := runningTask{page: page1, leaf: lv.draft.leafID, block: leafBlockID}.key()
	if err := tt.Click("展开计时待办列表"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("跳转来源：" + leafKey); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.selected != page1 || lv.collapsed || a.blockJump.block != leafBlockID {
		t.Fatalf("jump to leaf task failed: selected=%s collapsed=%v jump=%+v", a.selected, lv.collapsed, a.blockJump)
	}
}

func TestFloatingTimedTasksJumpCentersBlock(t *testing.T) {
	a, tt := commandTestApp(t)
	page1 := a.selected
	d := a.current()

	// Build a tall document with a timed task near the top, one in the middle, and one near the bottom.
	var paras []richtext.Paragraph
	for i := range 90 {
		p := richtext.Paragraph{
			Style: richtext.ParagraphStyle{ID: newID()},
			Runs:  []richtext.Run{{Text: fmt.Sprintf("第 %02d 段参考内容，用于拉长页面高度以便测试居中跳转效果", i)}},
		}
		if i == 1 || i == 45 || i == 88 {
			p.Style.List = richtext.Tasks
			p.Style.Timer = richtext.TaskTimer{
				State:       richtext.TimerRunning,
				StartedAtNS: tt.Now().UnixNano(),
				HasEstimate: true,
				EstimatedNS: int64(25 * time.Minute),
			}
		}
		paras = append(paras, p)
	}
	d.doc.SetText(richtext.TextOf(paras...))
	tt.Frame()

	allParas := menuParagraphs(d.doc.Text())
	topID := allParas[1].Style.ID
	midID := allParas[45].Style.ID
	botID := allParas[88].Style.ID

	assertVisibleCenter := func(id string, wantCentered bool) {
		t.Helper()
		pos, ok := blockPosition(d.doc.Text(), id)
		if !ok {
			t.Fatalf("missing block %s", id)
		}
		var targetBox richtext.ParagraphBox
		found := false
		for _, box := range d.doc.VisibleParagraphs() {
			if box.Start == pos {
				targetBox = box
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("jumped block %s not visible in viewport (scroll=%v)", id, d.doc.ScrollOffset())
		}
		if wantCentered {
			vp := d.doc.ViewportHeight()
			center := targetBox.Top + targetBox.Height/2
			if math.Abs(float64(center-vp/2)) > 24 {
				t.Fatalf("expected block %s centered near %.1f, got center=%.1f (box=%+v scroll=%.1f)", id, vp/2, center, targetBox, d.doc.ScrollOffset())
			}
		}
	}

	// Jump to middle block from another page: should land near vertical center.
	a.addPage("")
	tt.Frame()
	a.jumpToTimedTask(activeTimedTask{page: page1, block: midID})
	tt.Frame()
	assertVisibleCenter(midID, true)

	// Jump to bottom block: not enough blank space below, should still clamp cleanly and show block.
	a.jumpToTimedTask(activeTimedTask{page: page1, block: botID})
	tt.Frame()
	assertVisibleCenter(botID, false)

	// Jump to top block: not enough content above, should clamp cleanly to top.
	a.jumpToTimedTask(activeTimedTask{page: page1, block: topID})
	tt.Frame()
	if d.doc.ScrollOffset() != 0 {
		t.Fatalf("expected top jump clamped to 0, got %v", d.doc.ScrollOffset())
	}
	assertVisibleCenter(topID, false)

	// Add a tall Leaf near the bottom of a long page and jump to a task in the middle of the Leaf.
	d.doc.SetSelection(d.doc.Text().Len(), d.doc.Text().Len())
	a.createLeaf(d)
	tt.Frame()
	var lv *leafView
	for _, view := range d.leafViews {
		lv = view
		break
	}
	var leafParas []richtext.Paragraph
	for i := range 50 {
		p := richtext.Paragraph{
			Style: richtext.ParagraphStyle{ID: newID()},
			Runs:  []richtext.Run{{Text: fmt.Sprintf("Leaf 内部第 %02d 段内容，用来验证跨 Leaf 跳转居中", i)}},
		}
		if i == 25 {
			p.Style.List = richtext.Tasks
			p.Style.Timer = richtext.TaskTimer{
				State:       richtext.TimerRunning,
				StartedAtNS: tt.Now().UnixNano(),
				HasEstimate: true,
				EstimatedNS: int64(15 * time.Minute),
			}
		}
		leafParas = append(leafParas, p)
	}
	// Append trailing paragraphs to the outer page so there is room below the Leaf too.
	for i := range 30 {
		d.doc.SetSelection(d.doc.Text().Len(), d.doc.Text().Len())
		d.doc.InsertText(fmt.Sprintf("\nLeaf 后方补充段落 %02d", i))
	}
	lv.draft.doc.SetText(richtext.TextOf(leafParas...))
	lv.collapsed = true
	d.layoutRevision++
	a.addPage("")
	tt.Frame()

	leafMidID := menuParagraphs(lv.draft.doc.Text())[25].Style.ID
	a.jumpToTimedTask(activeTimedTask{page: page1, leaf: lv.draft.leafID, block: leafMidID})
	for range 4 {
		tt.Frame()
	}
	leafPos, _ := blockPosition(lv.draft.doc.Text(), leafMidID)
	var leafBox richtext.ParagraphBox
	for _, box := range lv.draft.doc.VisibleParagraphs() {
		if box.Start == leafPos {
			leafBox = box
			break
		}
	}
	var leafRect ui.Rect
	for _, att := range d.doc.VisibleAttachments() {
		if leafID(att.Attachment) == lv.draft.leafID {
			leafRect = att.Rect
			break
		}
	}
	if leafRect.W == 0 {
		t.Fatal("target Leaf not visible after jump")
	}
	vp := d.doc.ViewportHeight()
	leafCenter := leafRect.Y + 52 + leafBox.Top + leafBox.Height/2
	if math.Abs(float64(leafCenter-vp/2)) > 24 {
		t.Fatalf("expected Leaf block centered near %.1f, got %.1f (leafRect=%+v leafBox=%+v scroll=%.1f)", vp/2, leafCenter, leafRect, leafBox, d.doc.ScrollOffset())
	}
}

