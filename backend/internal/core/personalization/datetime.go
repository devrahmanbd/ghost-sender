package personalization

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type DateTimeService struct {
	mu         sync.RWMutex
	location   *time.Location
	cacheLimit int
	cache      map[string]string
}

func NewDateTimeService(loc *time.Location) *DateTimeService {
	if loc == nil {
		loc = time.Local
	}
	return &DateTimeService{
		location:   loc,
		cacheLimit: 2048,
		cache:      make(map[string]string, 256),
	}
}

func (s *DateTimeService) SetLocation(loc *time.Location) {
	if loc == nil {
		return
	}
	s.mu.Lock()
	s.location = loc
	s.cache = make(map[string]string, 256)
	s.mu.Unlock()
}

func (s *DateTimeService) Location() *time.Location {
	s.mu.RLock()
	loc := s.location
	s.mu.RUnlock()
	return loc
}

func (s *DateTimeService) Now() time.Time {
	return time.Now().In(s.Location())
}

func (s *DateTimeService) TimeOfDay(t time.Time) string {
	h := t.In(s.Location()).Hour()
	switch {
	case h >= 5 && h < 12:
		return "morning"
	case h >= 12 && h < 17:
		return "afternoon"
	case h >= 17 && h < 21:
		return "evening"
	default:
		return "night"
	}
}

func (s *DateTimeService) Format(t time.Time, format string) string {
	format = strings.TrimSpace(format)
	if format == "" {
		format = "Jan 02, 2006"
	}
	layout := normalizeDateFormat(format)
	key := fmt.Sprintf("%d|%s|%s", t.In(s.Location()).UnixNano(), layout, s.Location().String())

	s.mu.RLock()
	if v, ok := s.cache[key]; ok {
		s.mu.RUnlock()
		return v
	}
	s.mu.RUnlock()

	out := t.In(s.Location()).Format(layout)

	s.mu.Lock()
	if len(s.cache) >= s.cacheLimit {
		s.cache = make(map[string]string, 256)
	}
	s.cache[key] = out
	s.mu.Unlock()

	return out
}

func (s *DateTimeService) CurrentDate(format string) string {
	return s.Format(s.Now(), format)
}

func (s *DateTimeService) CustomDate(daysOffset int, format string) string {
	return s.Format(s.Now().AddDate(0, 0, daysOffset), format)
}

func (s *DateTimeService) ParseOffsetExpr(expr string) (int, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, false
	}
	i, err := strconv.Atoi(expr)
	if err != nil {
		return 0, false
	}
	return i, true
}

var (
	reCustomDate = regexp.MustCompile(`(?i)^CUSTOM_DATE(?:_([+-]?\d+))?(?:_(.+))?$`)
)

type CustomDateSpec struct {
	OffsetDays int
	Format     string
}

func (s *DateTimeService) ParseCustomDateVariable(varName string) (CustomDateSpec, bool) {
	m := reCustomDate.FindStringSubmatch(strings.TrimSpace(varName))
	if len(m) == 0 {
		return CustomDateSpec{}, false
	}

	offset := 0
	if m[1] != "" {
		if v, ok := s.ParseOffsetExpr(m[1]); ok {
			offset = v
		}
	}

	format := strings.TrimSpace(m[2])
	if format == "" {
		format = "Jan 02, 2006"
	}

	return CustomDateSpec{OffsetDays: offset, Format: format}, true
}

func (s *DateTimeService) EvaluateVariable(varName string) (string, bool) {
	name := strings.TrimSpace(varName)
	if name == "" {
		return "", false
	}

	switch strings.ToUpper(name) {
	case "CURRENT_DATE", "CURRENTDATE", "DATE":
		return s.CurrentDate("Jan 02, 2006"), true
	case "SHORT_DATE", "SHORTDATE":
		return s.CurrentDate("01/02/2006"), true
	case "ISO_DATE", "ISODATE":
		return s.CurrentDate("2006-01-02"), true
	case "ISO_DATETIME", "ISODATETIME":
		return s.Format(s.Now(), time.RFC3339), true
	case "TIME_OF_DAY", "TIMEOFDAY":
		return s.TimeOfDay(s.Now()), true
	}

	if spec, ok := s.ParseCustomDateVariable(name); ok {
		return s.CustomDate(spec.OffsetDays, spec.Format), true
	}

	return "", false
}

func normalizeDateFormat(format string) string {
	f := strings.TrimSpace(format)
	if f == "" {
		return "Jan 02, 2006"
	}

	up := strings.ToUpper(f)
	if strings.Contains(up, "YYYY") || strings.Contains(up, "MM") || strings.Contains(up, "DD") {
		f = strings.ReplaceAll(f, "YYYY", "2006")
		f = strings.ReplaceAll(f, "yyyy", "2006")
		f = strings.ReplaceAll(f, "YY", "06")
		f = strings.ReplaceAll(f, "yy", "06")
		f = strings.ReplaceAll(f, "MM", "01")
		f = strings.ReplaceAll(f, "mm", "01")
		f = strings.ReplaceAll(f, "DD", "02")
		f = strings.ReplaceAll(f, "dd", "02")
	}

	switch strings.ToUpper(f) {
	case "RFC3339":
		return time.RFC3339
	case "RFC3339NANO":
		return time.RFC3339Nano
	}

	return f
}
