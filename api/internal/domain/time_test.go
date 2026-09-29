package domain

import (
	"testing"
	"time"
)

func TestParseOffsetDateTime(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"UTC designator", "2026-09-25T10:00:00Z", "2026-09-25T10:00:00Z"},
		{"positive offset", "2026-09-25T10:00:00+07:00", "2026-09-25T03:00:00Z"},
		{"negative offset", "2026-09-25T10:00:00-04:30", "2026-09-25T14:30:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOffsetDateTime(tt.raw)
			if err != nil {
				t.Fatalf("ParseOffsetDateTime() error = %v", err)
			}
			if got.Location() != time.UTC || got.Format(time.RFC3339) != tt.want {
				t.Fatalf("ParseOffsetDateTime() = %s (%v), want %s in UTC", got.Format(time.RFC3339), got.Location(), tt.want)
			}
		})
	}
}

func TestParseOffsetDateTimeRejectsMissingOrMalformedOffset(t *testing.T) {
	for _, raw := range []string{
		"2026-09-25T10:00:00",
		"2026-09-25 10:00:00",
		"2026-09-25T10:00:00+7:00",
		"2026-09-25T10:00:00+07",
		"2026-09-25T10:00:00+24:00",
		"2026-09-25T10:00:00+00:60",
		"2026-09-25T10:00:00-01:60",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseOffsetDateTime(raw); err == nil {
				t.Fatalf("ParseOffsetDateTime(%q) succeeded, want error", raw)
			}
		})
	}
}

func TestParseOffsetDateTimeAcceptsValidQuarterHourOffset(t *testing.T) {
	got, err := ParseOffsetDateTime("2026-09-25T10:00:00+05:45")
	if err != nil {
		t.Fatalf("ParseOffsetDateTime() error = %v", err)
	}
	if want := "2026-09-25T04:15:00Z"; got.Format(time.RFC3339) != want {
		t.Fatalf("ParseOffsetDateTime() = %s, want %s", got.Format(time.RFC3339), want)
	}
}

func TestDuration(t *testing.T) {
	for _, minutes := range []int{0, -1} {
		if _, err := NewDuration(minutes); err == nil {
			t.Fatalf("NewDuration(%d) succeeded, want error", minutes)
		}
	}

	duration, err := NewDuration(90)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	if got := duration.End(start); !got.Equal(start.Add(90 * time.Minute)) {
		t.Fatalf("Duration.End() = %v, want %v", got, start.Add(90*time.Minute))
	}
	if duration.Minutes() != 90 {
		t.Fatalf("Duration.Minutes() = %d, want 90", duration.Minutes())
	}
}

func TestDurationRejectsTimeDurationOverflow(t *testing.T) {
	maxMinutes := int((1<<63 - 1) / int64(time.Minute))
	if _, err := NewDuration(maxMinutes + 1); err == nil {
		t.Fatalf("NewDuration(%d) succeeded, want overflow error", maxMinutes+1)
	}

	duration, err := NewDuration(maxMinutes)
	if err != nil {
		t.Fatalf("NewDuration(%d) error = %v", maxMinutes, err)
	}
	start := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	if end := duration.End(start); !end.After(start) {
		t.Fatalf("Duration.End() = %v, want after %v", end, start)
	}
}

func TestIntervalRejectsEmptyOrReversedRange(t *testing.T) {
	start := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	for _, end := range []time.Time{start, start.Add(-time.Minute)} {
		if _, err := NewInterval(start, end); err == nil {
			t.Fatalf("NewInterval(%v, %v) succeeded, want error", start, end)
		}
	}
}

func TestIntervalUsesHalfOpenOverlapSemantics(t *testing.T) {
	first := mustInterval(t, "2026-09-25T10:00:00Z", "2026-09-25T11:00:00Z")
	touching := mustInterval(t, "2026-09-25T11:00:00Z", "2026-09-25T12:00:00Z")
	overlapping := mustInterval(t, "2026-09-25T10:59:59Z", "2026-09-25T12:00:00Z")

	if first.Overlaps(touching) || touching.Overlaps(first) {
		t.Fatal("boundary-touching half-open intervals overlap")
	}
	if !first.Overlaps(overlapping) || !overlapping.Overlaps(first) {
		t.Fatal("positive intersection was not detected")
	}
}

func TestGridAlignment(t *testing.T) {
	for _, raw := range []string{"2026-09-25T10:00:00Z", "2026-09-25T10:30:00+07:00"} {
		instant, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			t.Fatal(err)
		}
		if !IsGridAligned(instant, 30) {
			t.Fatalf("IsGridAligned(%s, 30) = false, want true", raw)
		}
	}
	for _, instant := range []time.Time{
		time.Date(2026, 9, 25, 10, 15, 0, 0, time.UTC),
		time.Date(2026, 9, 25, 10, 30, 1, 0, time.UTC),
	} {
		if IsGridAligned(instant, 30) {
			t.Fatalf("IsGridAligned(%v, 30) = true, want false", instant)
		}
	}
	if IsGridAligned(time.Now(), 0) {
		t.Fatal("nonpositive grid interval accepted")
	}
	if IsGridAligned(time.Time{}, 30) {
		t.Fatal("zero instant accepted as grid aligned")
	}
}

func TestBusinessHoursContainCompleteInterval(t *testing.T) {
	hours := mustInterval(t, "2026-09-25T09:00:00Z", "2026-09-25T17:00:00Z")
	for _, candidate := range []Interval{
		mustInterval(t, "2026-09-25T09:00:00Z", "2026-09-25T10:00:00Z"),
		mustInterval(t, "2026-09-25T16:00:00Z", "2026-09-25T17:00:00Z"),
		mustInterval(t, "2026-09-25T09:00:00Z", "2026-09-25T17:00:00Z"),
	} {
		if !candidate.Within(hours) {
			t.Fatalf("%v should be within business hours", candidate)
		}
	}
	outside := mustInterval(t, "2026-09-25T16:30:00Z", "2026-09-25T17:30:00Z")
	if outside.Within(hours) {
		t.Fatal("interval extending past close was accepted")
	}
}

func mustInterval(t *testing.T, startRaw, endRaw string) Interval {
	t.Helper()
	start, err := time.Parse(time.RFC3339, startRaw)
	if err != nil {
		t.Fatal(err)
	}
	end, err := time.Parse(time.RFC3339, endRaw)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := NewInterval(start, end)
	if err != nil {
		t.Fatal(err)
	}
	return interval
}
