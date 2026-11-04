package main

import (
	"github.com/egoist/mygo/treerichtext"
	"reflect"
	"testing"
	"time"
)

func TestTimedTaskDailyHistoryExcludesPauses(t *testing.T) {
	a, tt, id := timedTaskFixture(t)
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 3, 23, 50, 0, 0, loc)
	d := a.current()
	a.timedTaskAction(d, id, "start", start)
	a.timedTaskAction(d, id, "pause", start.Add(20*time.Minute))
	paused := taskStyle(t, a, id)
	segments := d.page.Body.taskTimeSegments(paused.Timer, start.Add(20*time.Minute))
	if len(segments) != 2 || segments[0].Date != "2026-11-03" || segments[1].Date != "2026-11-04" || segments[0].DurationNS != int64(10*time.Minute) || segments[1].DurationNS != int64(10*time.Minute) {
		t.Fatalf("midnight split: %+v", segments)
	}
	resume := time.Date(2026, 11, 4, 9, 0, 0, 0, loc)
	a.timedTaskAction(d, id, "start", resume)
	// An active session can already be attributed to dates before it ends.
	ongoing := d.page.Body.taskTimeSegments(taskStyle(t, a, id).Timer, resume.Add(5*time.Minute))
	if len(ongoing) != 3 || ongoing[2].DurationNS != int64(5*time.Minute) {
		t.Fatal("active interval cannot be reconstructed")
	}
	a.timedTaskAction(d, id, "complete", resume.Add(30*time.Minute))
	finished := taskStyle(t, a, id)
	segments = d.page.Body.taskTimeSegments(finished.Timer, resume.Add(time.Hour))
	if len(segments) != 3 {
		t.Fatalf("wrong segments: %+v", segments)
	}
	days := map[string]time.Duration{}
	for _, segment := range segments {
		if segment.DurationNS <= 0 || segment.EndedAt.Sub(segment.StartedAt) != time.Duration(segment.DurationNS) || segment.TimeZone != "Asia/Shanghai" || segment.UTCOffsetSeconds != 8*3600 {
			t.Fatalf("invalid interval: %+v", segment)
		}
		days[segment.Date] += time.Duration(segment.DurationNS)
	}
	if days["2026-11-03"] != 10*time.Minute || days["2026-11-04"] != 40*time.Minute || finished.Timer.Elapsed(resume.Add(time.Hour)) != 50*time.Minute {
		t.Fatalf("pause was included or elapsed disagrees: %v", days)
	}
	d.doc.Undo()
	tt.Frame()
	if taskStyle(t, a, id).Timer.HistoryID != paused.Timer.HistoryID {
		t.Fatal("undo did not restore previous history snapshot")
	}
	if len(d.page.Body.TaskTimes[paused.Timer.HistoryID]) != 2 {
		t.Fatal("append mutated old history")
	}
	d.doc.Redo()
	tt.Frame()
	if !reflect.DeepEqual(d.page.Body.taskTimeSegments(taskStyle(t, a, id).Timer, resume.Add(time.Hour)), segments) {
		t.Fatal("redo duplicated or lost intervals")
	}
	if err := a.flush(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newApp(a.store)
	if err != nil {
		t.Fatal(err)
	}
	reopened.selectPage(d.page.ID)
	recovered := reopened.current().page.Body.taskTimeSegments(taskStyle(t, reopened, id).Timer, resume.Add(time.Hour))
	if !reflect.DeepEqual(recovered, segments) {
		t.Fatal("SQLite reopen lost daily records")
	}
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
			if para.Style.Timer.HistoryID == finished.Timer.HistoryID {
				found = reflect.DeepEqual(p.Body.taskTimeSegments(para.Style.Timer, resume), segments)
			}
		}
	}
	if !found {
		t.Fatal("backup restore lost daily records")
	}
}

func TestTimedTaskDailyHistoryBoundariesAndZones(t *testing.T) {
	for _, tc := range []struct {
		name, zone string
		start      time.Time
		duration   time.Duration
		days       []time.Duration
	}{
		{name: "fractional-midnight", zone: "Asia/Shanghai", start: time.Date(2026, 11, 3, 23, 59, 59, 500000000, time.UTC), duration: time.Second, days: []time.Duration{500 * time.Millisecond, 500 * time.Millisecond}},
		{name: "exact-midnight", zone: "Asia/Shanghai", start: time.Date(2026, 11, 3, 23, 50, 0, 0, time.UTC), duration: 10 * time.Minute, days: []time.Duration{10 * time.Minute}},
		{name: "multiple-days", zone: "Asia/Shanghai", start: time.Date(2026, 11, 3, 23, 0, 0, 0, time.UTC), duration: 26 * time.Hour, days: []time.Duration{time.Hour, 24 * time.Hour, time.Hour}},
		{name: "spring-DST", zone: "America/New_York", start: time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC), duration: 24 * time.Hour, days: []time.Duration{23 * time.Hour, time.Hour}},
		{name: "fall-DST", zone: "America/New_York", start: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), duration: 26 * time.Hour, days: []time.Duration{25 * time.Hour, time.Hour}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			s := tc.start
			start := time.Date(s.Year(), s.Month(), s.Day(), s.Hour(), s.Minute(), s.Second(), s.Nanosecond(), loc)
			timer := (richtext.TaskTimer{State: richtext.TimerIdle}).Start(start)
			// Stop from a different current zone; the original local dates stay fixed.
			end := start.Add(tc.duration).In(time.FixedZone("changed", -7*3600))
			segments := splitTaskTime(timer, end)
			if len(segments) != len(tc.days) {
				t.Fatalf("got %d segments, want %d", len(segments), len(tc.days))
			}
			sum := time.Duration(0)
			for i, seg := range segments {
				if time.Duration(seg.DurationNS) != tc.days[i] || seg.Date != seg.StartedAt.In(loc).Format("2006-01-02") {
					t.Fatalf("wrong day %d: %+v", i, seg)
				}
				if i > 0 && !segments[i-1].EndedAt.Equal(seg.StartedAt) {
					t.Fatal("gap/overlap at midnight")
				}
				sum += time.Duration(seg.DurationNS)
			}
			if sum != tc.duration || sum != timer.Elapsed(end) {
				t.Fatal("daily totals disagree with elapsed")
			}
			if len(splitTaskTime(timer, start.Add(-time.Second))) != 0 || timer.Elapsed(start.Add(-time.Second)) != 0 {
				t.Fatal("clock reversal created negative work")
			}
		})
	}
}

func TestTimedTaskTerminatedHistoryAndFreshCopies(t *testing.T) {
	a, tt, id := timedTaskFixture(t)
	clickTask(t, tt, id, "开始")
	advanceTaskClock(tt, 13*time.Second)
	clickTask(t, tt, id, "废弃")
	d := a.current()
	st := taskStyle(t, a, id)
	segments := d.page.Body.taskTimeSegments(st.Timer, tt.Now())
	if len(segments) != 1 || segments[0].DurationNS != int64(13*time.Second) {
		t.Fatal("termination lost invested time")
	}
	openBlockMenu(t, a, tt, 0)
	clickBlockMenuItem(t, tt, "复制块")
	copyTimer := menuParagraphs(d.doc.Text())[1].Style.Timer
	if copyTimer.State != richtext.TimerIdle || copyTimer.HistoryID != "" || len(d.page.Body.taskTimeSegments(copyTimer, tt.Now())) != 0 {
		t.Fatal("copy would double count historical time")
	}
}
