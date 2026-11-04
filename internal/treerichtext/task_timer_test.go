package richtext

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestTimedTaskStrikeStyle(t *testing.T) {
	l := look{}
	l.Muted = ui.Hex("#999999")
	for _, tc := range []struct {
		name   string
		style  ParagraphStyle
		strike bool
	}{
		{"ordinary-done", ParagraphStyle{List: Tasks, Checked: true}, true},
		{"timed-done", ParagraphStyle{List: Tasks, Checked: true, Timer: TaskTimer{State: TimerCompleted}}, false},
		{"timed-terminated", ParagraphStyle{List: Tasks, Timer: TaskTimer{State: TimerTerminated}}, true},
		{"timed-paused", ParagraphStyle{List: Tasks, Timer: TaskTimer{State: TimerPaused}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			span := l.span("task", Attributes{}, tc.style)
			if span.Strikethrough != tc.strike {
				t.Fatalf("strike = %v", span.Strikethrough)
			}
			if tc.strike && span.Color != l.Muted {
				t.Fatal("struck task did not use muted color")
			}
		})
	}
}
