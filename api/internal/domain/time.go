package domain

import (
	"errors"
	"regexp"
	"time"
)

var explicitOffset = regexp.MustCompile(`(?:Z|[+-][0-9]{2}:[0-9]{2})$`)

// ParseOffsetDateTime parses an RFC 3339 instant that explicitly declares its
// UTC offset and normalizes the result to UTC.
func ParseOffsetDateTime(raw string) (time.Time, error) {
	if !explicitOffset.MatchString(raw) {
		return time.Time{}, errors.New("date-time must include an explicit UTC offset")
	}
	if raw[len(raw)-1] != 'Z' {
		offsetHour := raw[len(raw)-5 : len(raw)-3]
		offsetMinute := raw[len(raw)-2:]
		if offsetHour > "23" || offsetMinute > "59" {
			return time.Time{}, errors.New("date-time UTC offset is invalid")
		}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errors.New("date-time must be valid RFC 3339")
	}
	_, offsetSeconds := parsed.Zone()
	if offsetSeconds <= -24*60*60 || offsetSeconds >= 24*60*60 {
		return time.Time{}, errors.New("date-time UTC offset must be less than 24 hours")
	}
	return parsed.UTC(), nil
}

// Duration is a validated, immutable service duration. Its sealed interface
// prevents callers outside this package from constructing an invalid zero
// value while retaining simple accessors for persistence and application code.
type Duration interface {
	Minutes() int
	End(start time.Time) time.Time
	domainDuration()
}

type duration struct {
	minutes int
}

func NewDuration(minutes int) (Duration, error) {
	if minutes <= 0 {
		return nil, errors.New("duration must be positive")
	}
	const maxDurationMinutes = (1<<63 - 1) / int64(time.Minute)
	if int64(minutes) > maxDurationMinutes {
		return nil, errors.New("duration exceeds supported range")
	}
	return duration{minutes: minutes}, nil
}

func (d duration) Minutes() int { return d.minutes }
func (d duration) End(start time.Time) time.Time {
	return start.Add(time.Duration(d.minutes) * time.Minute)
}
func (duration) domainDuration() {}

// Interval is a validated, immutable half-open interval [start, end). The
// sealed interface makes invalid construction unavailable to other packages.
type Interval interface {
	Start() time.Time
	End() time.Time
	Overlaps(other Interval) bool
	Within(container Interval) bool
	domainInterval()
}

type interval struct {
	start time.Time
	end   time.Time
}

func NewInterval(start, end time.Time) (Interval, error) {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil, errors.New("interval end must be after start")
	}
	return interval{start: start.UTC(), end: end.UTC()}, nil
}

func (i interval) Start() time.Time { return i.start }
func (i interval) End() time.Time   { return i.end }

func (i interval) Overlaps(other Interval) bool {
	o, ok := other.(interval)
	return ok && i.start.Before(o.end) && o.start.Before(i.end)
}

func (i interval) Within(container Interval) bool {
	c, ok := container.(interval)
	return ok && !i.start.Before(c.start) && !i.end.After(c.end)
}

func (interval) domainInterval() {}

// IsGridAligned reports whether an instant lies exactly on a grid boundary in
// its declared location. Seconds and sub-second values must be zero.
func IsGridAligned(instant time.Time, intervalMinutes int) bool {
	if instant.IsZero() || intervalMinutes <= 0 || instant.Second() != 0 || instant.Nanosecond() != 0 {
		return false
	}
	minutesSinceMidnight := instant.Hour()*60 + instant.Minute()
	return minutesSinceMidnight%intervalMinutes == 0
}
