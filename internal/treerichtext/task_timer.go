package richtext

import "time"

// TaskTimer is saved with a paragraph so moving, saving and undoing a timed
// task keep its state and its identity together. ElapsedNS excludes pauses;
// StartedAtNS is a wall-clock timestamp, allowing a running timer to survive
// closing and reopening the document without writing on every tick.
type TaskTimer struct {
	State         TaskTimerState `json:"state,omitempty"`
	ElapsedNS     int64          `json:"elapsedNs,omitempty"`
	StartedAtNS   int64          `json:"startedAtNs,omitempty"`
	StartedZone   string         `json:"startedZone,omitempty"`
	StartedOffset int            `json:"startedOffset,omitempty"`
	HistoryID     string         `json:"historyId,omitempty"`
}

type TaskTimerState string

const (
	TimerNone       TaskTimerState = ""
	TimerIdle       TaskTimerState = "idle"
	TimerRunning    TaskTimerState = "running"
	TimerPaused     TaskTimerState = "paused"
	TimerCompleted  TaskTimerState = "completed"
	TimerTerminated TaskTimerState = "terminated"
)

func (t TaskTimer) Enabled() bool { return t.State != TimerNone }
func (t TaskTimer) Elapsed(now time.Time) time.Duration {
	elapsed := max(t.ElapsedNS, 0)
	if t.State == TimerRunning && t.StartedAtNS > 0 {
		elapsed += max(now.UnixNano()-t.StartedAtNS, 0)
	}
	return time.Duration(elapsed)
}

func (t TaskTimer) Start(now time.Time) TaskTimer {
	if t.State != TimerIdle && t.State != TimerPaused {
		return t
	}
	t.State, t.StartedAtNS = TimerRunning, now.UnixNano()
	t.StartedZone = now.Location().String()
	_, t.StartedOffset = now.Zone()
	return t
}

func (t TaskTimer) Stop(state TaskTimerState, now time.Time) TaskTimer {
	if !t.Enabled() {
		return t
	}
	t.ElapsedNS = int64(t.Elapsed(now))
	t.State, t.StartedAtNS = state, 0
	t.StartedZone, t.StartedOffset = "", 0
	return t
}

// FreshCopy creates an independent task; copied work must not double-count
// the original task's recorded time in later daily statistics.
func (t TaskTimer) FreshCopy() TaskTimer {
	if t.Enabled() {
		return TaskTimer{State: TimerIdle}
	}
	return TaskTimer{}
}

func (t TaskTimer) normalized() TaskTimer {
	switch t.State {
	case TimerNone:
		return TaskTimer{}
	case TimerIdle:
		return TaskTimer{State: TimerIdle}
	case TimerRunning:
		if t.StartedAtNS <= 0 {
			t.State = TimerPaused
		}
	case TimerPaused, TimerCompleted, TimerTerminated:
	default:
		return TaskTimer{}
	}
	t.ElapsedNS = max(0, t.ElapsedNS)
	if t.State != TimerRunning {
		t.StartedAtNS = 0
		t.StartedZone, t.StartedOffset = "", 0
	}
	return t
}
