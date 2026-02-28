package utils

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	DateFormat         = "2006-01-02"
	TimeFormat         = "15:04:05"
	DateTimeFormat     = "2006-01-02 15:04:05"
	ISO8601Format      = "2006-01-02T15:04:05Z07:00"
	RFC3339Format      = time.RFC3339
	UnixTimestampFormat = "unix"
	
	DayDuration   = 24 * time.Hour
	WeekDuration  = 7 * DayDuration
	MonthDuration = 30 * DayDuration
	YearDuration  = 365 * DayDuration
)

var (
	ErrInvalidDuration = errors.New("invalid duration format")
	ErrInvalidOffset   = errors.New("invalid time offset")
	ErrInvalidFormat   = errors.New("invalid time format")
)

type TimeOfDay string

const (
	Morning   TimeOfDay = "morning"
	Afternoon TimeOfDay = "afternoon"
	Evening   TimeOfDay = "evening"
	Night     TimeOfDay = "night"
)

func Now() time.Time {
	return time.Now()
}

func NowUTC() time.Time {
	return time.Now().UTC()
}

func Unix(sec int64) time.Time {
	return time.Unix(sec, 0)
}

func UnixMilli(msec int64) time.Time {
	return time.Unix(msec/1000, (msec%1000)*1000000)
}

func ToUnix(t time.Time) int64 {
	return t.Unix()
}

func ToUnixMilli(t time.Time) int64 {
	return t.UnixNano() / 1000000
}

func FormatTime(t time.Time, format string) string {
	switch format {
	case "date":
		return t.Format(DateFormat)
	case "time":
		return t.Format(TimeFormat)
	case "datetime":
		return t.Format(DateTimeFormat)
	case "iso8601":
		return t.Format(ISO8601Format)
	case "rfc3339":
		return t.Format(RFC3339Format)
	case "unix":
		return strconv.FormatInt(t.Unix(), 10)
	default:
		return t.Format(format)
	}
}

func FormatNow(format string) string {
	return FormatTime(Now(), format)
}

func ParseTime(value, format string) (time.Time, error) {
	switch format {
	case "date":
		return time.Parse(DateFormat, value)
	case "time":
		return time.Parse(TimeFormat, value)
	case "datetime":
		return time.Parse(DateTimeFormat, value)
	case "iso8601":
		return time.Parse(ISO8601Format, value)
	case "rfc3339":
		return time.Parse(RFC3339Format, value)
	case "unix":
		sec, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return Unix(sec), nil
	default:
		return time.Parse(format, value)
	}
}

func AddDuration(t time.Time, duration time.Duration) time.Time {
	return t.Add(duration)
}

func AddDays(t time.Time, days int) time.Time {
	return t.AddDate(0, 0, days)
}

func AddWeeks(t time.Time, weeks int) time.Time {
	return t.AddDate(0, 0, weeks*7)
}

func AddMonths(t time.Time, months int) time.Time {
	return t.AddDate(0, months, 0)
}

func AddYears(t time.Time, years int) time.Time {
	return t.AddDate(years, 0, 0)
}

func ParseOffset(offset string) (time.Duration, error) {
	if offset == "" || offset == "0" {
		return 0, nil
	}

	sign := 1
	if strings.HasPrefix(offset, "+") {
		offset = strings.TrimPrefix(offset, "+")
	} else if strings.HasPrefix(offset, "-") {
		sign = -1
		offset = strings.TrimPrefix(offset, "-")
	}

	var value int
	var unit string

	for i, r := range offset {
		if r < '0' || r > '9' {
			value, _ = strconv.Atoi(offset[:i])
			unit = offset[i:]
			break
		}
	}

	if unit == "" {
		value, _ = strconv.Atoi(offset)
		unit = "d"
	}

	var duration time.Duration
	switch strings.ToLower(unit) {
	case "s", "sec", "second", "seconds":
		duration = time.Duration(value) * time.Second
	case "m", "min", "minute", "minutes":
		duration = time.Duration(value) * time.Minute
	case "h", "hr", "hour", "hours":
		duration = time.Duration(value) * time.Hour
	case "d", "day", "days":
		duration = time.Duration(value) * DayDuration
	case "w", "week", "weeks":
		duration = time.Duration(value) * WeekDuration
	case "mo", "month", "months":
		duration = time.Duration(value) * MonthDuration
	case "y", "year", "years":
		duration = time.Duration(value) * YearDuration
	default:
		return 0, ErrInvalidOffset
	}

	return time.Duration(sign) * duration, nil
}

func ApplyOffset(t time.Time, offset string) (time.Time, error) {
	duration, err := ParseOffset(offset)
	if err != nil {
		return t, err
	}
	return t.Add(duration), nil
}

func GetTimeOfDay(t time.Time) TimeOfDay {
	hour := t.Hour()

	if hour >= 5 && hour < 12 {
		return Morning
	} else if hour >= 12 && hour < 17 {
		return Afternoon
	} else if hour >= 17 && hour < 21 {
		return Evening
	} else {
		return Night
	}
}

func IsTimeOfDay(t time.Time, tod TimeOfDay) bool {
	return GetTimeOfDay(t) == tod
}

func StartOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func EndOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 23, 59, 59, 999999999, t.Location())
}

func StartOfWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return StartOfDay(t.AddDate(0, 0, -weekday+1))
}

func EndOfWeek(t time.Time) time.Time {
	return EndOfDay(StartOfWeek(t).AddDate(0, 0, 6))
}

func StartOfMonth(t time.Time) time.Time {
	year, month, _ := t.Date()
	return time.Date(year, month, 1, 0, 0, 0, 0, t.Location())
}

func EndOfMonth(t time.Time) time.Time {
	return StartOfMonth(t).AddDate(0, 1, 0).Add(-time.Nanosecond)
}

func StartOfYear(t time.Time) time.Time {
	year, _, _ := t.Date()
	return time.Date(year, 1, 1, 0, 0, 0, 0, t.Location())
}

func EndOfYear(t time.Time) time.Time {
	year, _, _ := t.Date()
	return time.Date(year, 12, 31, 23, 59, 59, 999999999, t.Location())
}

func DurationBetween(start, end time.Time) time.Duration {
	return end.Sub(start)
}

func DaysBetween(start, end time.Time) int {
	duration := DurationBetween(StartOfDay(start), StartOfDay(end))
	return int(duration.Hours() / 24)
}

func HoursBetween(start, end time.Time) int {
	return int(DurationBetween(start, end).Hours())
}

func MinutesBetween(start, end time.Time) int {
	return int(DurationBetween(start, end).Minutes())
}

func SecondsBetween(start, end time.Time) int {
	return int(DurationBetween(start, end).Seconds())
}

func IsBefore(t1, t2 time.Time) bool {
	return t1.Before(t2)
}

func IsAfter(t1, t2 time.Time) bool {
	return t1.After(t2)
}

func IsBetween(t, start, end time.Time) bool {
	return (t.After(start) || t.Equal(start)) && (t.Before(end) || t.Equal(end))
}

func IsSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func IsToday(t time.Time) bool {
	return IsSameDay(t, Now())
}

func IsTomorrow(t time.Time) bool {
	return IsSameDay(t, Now().AddDate(0, 0, 1))
}

func IsYesterday(t time.Time) bool {
	return IsSameDay(t, Now().AddDate(0, 0, -1))
}

func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	if d < DayDuration {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}

	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}

func FormatHumanDuration(d time.Duration) string {
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	if d < DayDuration {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	if d < WeekDuration {
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
	if d < MonthDuration {
		weeks := int(d.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week"
		}
		return fmt.Sprintf("%d weeks", weeks)
	}
	if d < YearDuration {
		months := int(d.Hours() / 24 / 30)
		if months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", months)
	}

	years := int(d.Hours() / 24 / 365)
	if years == 1 {
		return "1 year"
	}
	return fmt.Sprintf("%d years", years)
}

func FormatTimeAgo(t time.Time) string {
	return FormatHumanDuration(time.Since(t)) + " ago"
}

func FormatTimeUntil(t time.Time) string {
	return "in " + FormatHumanDuration(time.Until(t))
}

func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrInvalidDuration
	}

	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}

	return ParseOffset(s)
}

func CalculateETA(completed, total int, elapsed time.Duration) time.Time {
	if completed == 0 || total == 0 {
		return time.Time{}
	}

	remaining := total - completed
	avgTimePerItem := elapsed / time.Duration(completed)
	remainingTime := avgTimePerItem * time.Duration(remaining)

	return Now().Add(remainingTime)
}

func CalculateRemainingTime(completed, total int, elapsed time.Duration) time.Duration {
	if completed == 0 || total == 0 {
		return 0
	}

	remaining := total - completed
	avgTimePerItem := elapsed / time.Duration(completed)
	return avgTimePerItem * time.Duration(remaining)
}

func Sleep(d time.Duration) {
	time.Sleep(d)
}

func SleepUntil(t time.Time) {
	duration := time.Until(t)
	if duration > 0 {
		time.Sleep(duration)
	}
}

func ExponentialBackoff(attempt int, baseDelay time.Duration, maxDelay time.Duration) time.Duration {
	delay := baseDelay * time.Duration(1<<uint(attempt))
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

func IsExpired(t time.Time) bool {
	return t.Before(Now())
}

func IsWithinDuration(t time.Time, duration time.Duration) bool {
	return time.Since(t) <= duration
}

func TimeUntil(t time.Time) time.Duration {
	return time.Until(t)
}

func TimeSince(t time.Time) time.Duration {
	return time.Since(t)
}

func ToLocation(t time.Time, loc *time.Location) time.Time {
	return t.In(loc)
}

func ToUTC(t time.Time) time.Time {
	return t.UTC()
}

func ToLocal(t time.Time) time.Time {
	return t.Local()
}

func LoadLocation(name string) (*time.Location, error) {
	return time.LoadLocation(name)
}

func Ticker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

func Timer(d time.Duration) *time.Timer {
	return time.NewTimer(d)
}

func After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

func Timeout(d time.Duration) <-chan time.Time {
	return time.After(d)
}

func Max(times ...time.Time) time.Time {
	if len(times) == 0 {
		return time.Time{}
	}

	max := times[0]
	for _, t := range times[1:] {
		if t.After(max) {
			max = t
		}
	}
	return max
}

func Min(times ...time.Time) time.Time {
	if len(times) == 0 {
		return time.Time{}
	}

	min := times[0]
	for _, t := range times[1:] {
		if t.Before(min) {
			min = t
		}
	}
	return min
}

func Clamp(t, min, max time.Time) time.Time {
	if t.Before(min) {
		return min
	}
	if t.After(max) {
		return max
	}
	return t
}

func IsZero(t time.Time) bool {
	return t.IsZero()
}

func IsWeekend(t time.Time) bool {
	weekday := t.Weekday()
	return weekday == time.Saturday || weekday == time.Sunday
}

func IsWeekday(t time.Time) bool {
	return !IsWeekend(t)
}

func NextWeekday(t time.Time) time.Time {
	next := t.AddDate(0, 0, 1)
	for IsWeekend(next) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func DurationToSeconds(d time.Duration) int64 {
	return int64(d.Seconds())
}

func DurationToMilliseconds(d time.Duration) int64 {
	return int64(d.Milliseconds())
}

func SecondsToDuration(seconds int64) time.Duration {
	return time.Duration(seconds) * time.Second
}

func MillisecondsToDuration(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

func Age(birthDate time.Time) int {
	now := Now()
	years := now.Year() - birthDate.Year()

	if now.Month() < birthDate.Month() ||
		(now.Month() == birthDate.Month() && now.Day() < birthDate.Day()) {
		years--
	}

	return years
}

func RoundToMinute(t time.Time) time.Time {
	return t.Truncate(time.Minute)
}

func RoundToHour(t time.Time) time.Time {
	return t.Truncate(time.Hour)
}

func RoundToDay(t time.Time) time.Time {
	return StartOfDay(t)
}

func CeilToMinute(t time.Time) time.Time {
	truncated := t.Truncate(time.Minute)
	if truncated.Equal(t) {
		return t
	}
	return truncated.Add(time.Minute)
}

func CeilToHour(t time.Time) time.Time {
	truncated := t.Truncate(time.Hour)
	if truncated.Equal(t) {
		return t
	}
	return truncated.Add(time.Hour)
}
