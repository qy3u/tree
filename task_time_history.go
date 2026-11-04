package main

import (
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo/treerichtext"
)

// TaskTimeSegment is one uninterrupted portion of work on one local date.
// UTC instants plus the original date/zone/offset keep historical day totals
// stable even if the computer's time zone changes later.
type TaskTimeSegment struct {
	Date             string    `json:"date"`
	StartedAt        time.Time `json:"startedAt"`
	EndedAt          time.Time `json:"endedAt"`
	DurationNS       int64     `json:"durationNs"`
	TimeZone         string    `json:"timeZone"`
	UTCOffsetSeconds int       `json:"utcOffsetSeconds"`
}

func splitTaskTime(timer richtext.TaskTimer, end time.Time) []TaskTimeSegment {
	if timer.State != richtext.TimerRunning || timer.StartedAtNS <= 0 {
		return nil
	}
	start := time.Unix(0, timer.StartedAtNS)
	if !end.After(start) {
		return nil
	}
	loc := time.FixedZone(timer.StartedZone, timer.StartedOffset)
	if timer.StartedZone != "Local" && timer.StartedZone != "" {
		if named, err := time.LoadLocation(timer.StartedZone); err == nil {
			loc = named
		}
	}
	var segments []TaskTimeSegment
	for cursor := start.In(loc); cursor.Before(end); {
		midnight := time.Date(cursor.Year(), cursor.Month(), cursor.Day()+1, 0, 0, 0, 0, loc)
		next := end.In(loc)
		if midnight.Before(next) {
			next = midnight
		}
		_, offset := cursor.Zone()
		segments = append(segments, TaskTimeSegment{
			Date: cursor.Format("2006-01-02"), StartedAt: cursor.UTC(), EndedAt: next.UTC(),
			DurationNS: int64(next.Sub(cursor)), TimeZone: timer.StartedZone, UTCOffsetSeconds: offset,
		})
		cursor = next
	}
	return segments
}

// Histories are immutable snapshots, like table versions: the timer in the
// current document points to the active snapshot. Undo/redo switches that
// pointer, so later statistics must read the active history, not sum every
// stored snapshot. Unreferenced versions are retained for undo.
func (b Body) taskTimeSegments(timer richtext.TaskTimer, now time.Time) []TaskTimeSegment {
	segments := append([]TaskTimeSegment(nil), b.TaskTimes[timer.HistoryID]...)
	return append(segments, splitTaskTime(timer, now)...)
}

// macOS names the system location "Local" in Go. Save its IANA name when
// available so a session spanning a daylight-saving change can be split at
// the correct local midnight even after the application restarts.
func taskLocalTime(now time.Time) time.Time {
	if now.Location().String() != "Local" {
		return now
	}
	if path, err := os.Readlink("/etc/localtime"); err == nil {
		if _, zone, found := strings.Cut(path, "zoneinfo/"); found {
			if loc, err := time.LoadLocation(zone); err == nil {
				return now.In(loc)
			}
		}
	}
	return now
}
