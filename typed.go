package edifact

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ErrEmpty reports a position that holds no value, which is distinct from one
// holding a value that will not convert.
var ErrEmpty = errors.New("edifact: value is empty")

// ErrNumeric reports a value that is not an ISO 9735 numeric: an optional
// minus sign, digits with at most one decimal mark followed by a digit, and an
// optional exponent such as E3 or e-3.
var ErrNumeric = errors.New("edifact: not a valid EDIFACT numeric value")

// ErrDateFormat reports a date format qualifier the package does not decode.
var ErrDateFormat = errors.New("edifact: unknown date format qualifier")

// ErrDateZone reports a time zone, written as letters, that is neither GMT nor
// UTC. The code list does not define the zone's format, so nothing else is
// guessed at.
var ErrDateZone = errors.New("edifact: unsupported time zone")

// UnknownTimezone is the location of a time read from a date format that
// carries no time zone. Its digits are as written and its offset is zero, which
// says nothing about the zone the sender meant.
var UnknownTimezone = time.FixedZone("UNKNOWN", 0)

// ErrDateRange reports a date format qualifier naming a period rather than a
// single point in time.
var ErrDateRange = errors.New("edifact: date format qualifier names a range")

// Float reads component comp of element elem, both counting from 1, as a
// number. Either a dot or a comma is accepted as the decimal mark, whatever the
// interchange declares.
func (s *Segment) Float(elem, comp int) (float64, error) {
	v := s.Component(elem, comp)
	if v == "" {
		return 0, ErrEmpty
	}
	if !isNumeric(v) {
		return 0, fmt.Errorf("edifact: element %d component %d: %w: %q", elem, comp, ErrNumeric, v)
	}
	f, err := strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
	if err != nil {
		return 0, fmt.Errorf("edifact: element %d component %d: %w", elem, comp, err)
	}
	return f, nil
}

// isNumeric reports whether v is an ISO 9735 numeric, accepting a dot or a
// comma as the decimal mark. A leading decimal mark and an exponent are also
// accepted.
func isNumeric(v string) bool {
	v = strings.TrimPrefix(v, "-")
	if i := strings.IndexAny(v, "Ee"); i >= 0 {
		exp := v[i+1:]
		if exp != "" && (exp[0] == '+' || exp[0] == '-') {
			exp = exp[1:]
		}
		if exp == "" || !isDigits(exp) {
			return false
		}
		v = v[:i]
	}
	whole, frac := v, ""
	if i := strings.IndexAny(v, ".,"); i >= 0 {
		whole, frac = v[:i], v[i+1:]
		if frac == "" {
			return false
		}
	}
	return (whole != "" || frac != "") && isDigits(whole) && isDigits(frac)
}

// isDigits reports whether v holds only the digits 0 to 9.
func isDigits(v string) bool {
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	return true
}

// Int reads component comp of element elem, both counting from 1, as a whole
// number: an optional minus sign and digits. Leading zeros are accepted.
func (s *Segment) Int(elem, comp int) (int, error) {
	v := s.Component(elem, comp)
	if v == "" {
		return 0, ErrEmpty
	}
	if !isNumeric(v) || strings.ContainsAny(v, ".,Ee") {
		return 0, fmt.Errorf("edifact: element %d component %d: %w: %q", elem, comp, ErrNumeric, v)
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("edifact: element %d component %d: %w", elem, comp, err)
	}
	return n, nil
}

// SetFloat writes v to component comp of element elem, both counting from 1.
// It is held with a dot as the decimal mark, and Write renders it with the mark
// the interchange declares. prec is the number of decimal places, or -1 for the
// fewest digits that parse back to v. An infinite or NaN v gives ErrNumeric and
// writes nothing.
func (s *Segment) SetFloat(elem, comp int, v float64, prec int) error {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return fmt.Errorf("edifact: element %d component %d: %w: %v", elem, comp, ErrNumeric, v)
	}
	s.Set(elem, comp, strconv.FormatFloat(v, 'f', prec, 64))
	if elem < 1 || comp < 1 {
		return nil
	}
	if s.floats == nil {
		s.floats = map[[2]int]bool{}
	}
	s.floats[[2]int{elem, comp}] = true
	return nil
}

// SetInt writes v to component comp of element elem, both counting from 1.
func (s *Segment) SetInt(elem, comp int, v int) {
	s.Set(elem, comp, strconv.Itoa(v))
}

// Time reads component comp of element elem, both counting from 1, as a date
// and time, taking its format qualifier from the component after it. That is
// the layout of composite C507, where a DTM carries the qualifier, the value
// and the format in one element.
func (s *Segment) Time(elem, comp int) (time.Time, error) {
	return s.TimeIn(elem, comp, s.Component(elem, comp+1))
}

// TimeIn reads component comp of element elem, both counting from 1, as a date
// and time in the given format, for segments that do not carry the format
// qualifier beside the value.
func (s *Segment) TimeIn(elem, comp int, format string) (time.Time, error) {
	v := s.Component(elem, comp)
	if v == "" {
		return time.Time{}, ErrEmpty
	}
	return ParseTime(v, format)
}

// SetTime writes t in the given format to component comp of element elem, both
// counting from 1, and the format qualifier to the component after it. For
// formats 301 and 404, whose zone is written as letters, it converts t to UTC
// and writes UTC. For other formats it writes t's own clock, and for 205 its
// offset as well.
func (s *Segment) SetTime(elem, comp int, t time.Time, format string) error {
	f, ok := dateFormats[format]
	if !ok {
		return fmt.Errorf("%w: %q", ErrDateFormat, format)
	}
	if f.parts == 2 {
		return fmt.Errorf("%w: %q", ErrDateRange, format)
	}
	v := t.Format(f.layout)
	if zonedFormats[format] {
		v = t.UTC().Format(f.layout) + "UTC"
	}
	s.Set(elem, comp, v)
	s.Set(elem, comp+1, format)
	return nil
}

// TimeRange reads component comp of element elem, both counting from 1, as a
// period, taking its format qualifier from the component after it.
func (s *Segment) TimeRange(elem, comp int) (start, end time.Time, err error) {
	v := s.Component(elem, comp)
	if v == "" {
		return time.Time{}, time.Time{}, ErrEmpty
	}
	return ParseTimeRange(v, s.Component(elem, comp+1))
}

// ParseTimeRange decodes a value against a format qualifier naming a period,
// such as 718 or 719. The two halves may be joined by a hyphen or run together.
func ParseTimeRange(value, format string) (start, end time.Time, err error) {
	if value == "" {
		return time.Time{}, time.Time{}, ErrEmpty
	}
	f, ok := dateFormats[format]
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: %q", ErrDateFormat, format)
	}
	if f.parts != 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("edifact: format %s names a point, not a range", format)
	}

	half := len(f.layout)
	lo, hi := value, ""
	switch {
	case len(value) == 2*half+1 && value[half] == '-':
		lo, hi = value[:half], value[half+1:]
	case len(value) == 2*half:
		lo, hi = value[:half], value[half:]
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("edifact: format %s: value is %d characters, want %d or %d",
			format, len(value), 2*half, 2*half+1)
	}

	if start, err = parseLayout(f.layout, lo); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("edifact: format %s start: %w", format, err)
	}
	if end, err = parseLayout(f.layout, hi); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("edifact: format %s end: %w", format, err)
	}
	return start, end, nil
}

// ParseTime decodes a value against a date format qualifier from code list
// 2379. A format with no time zone gives a time in UnknownTimezone. A zone
// written as GMT or UTC gives UTC, and an offset gives that offset.
//
// Two-digit years follow the Go convention, so 00 to 68 read as 2000 to 2068
// and 69 to 99 as 1969 to 1999.
func ParseTime(value, format string) (time.Time, error) {
	if value == "" {
		return time.Time{}, ErrEmpty
	}
	f, ok := dateFormats[format]
	if !ok {
		return time.Time{}, fmt.Errorf("%w: %q", ErrDateFormat, format)
	}
	if f.parts == 2 {
		return time.Time{}, fmt.Errorf("%w: %q", ErrDateRange, format)
	}
	if zonedFormats[format] {
		return parseZoned(value, format, f.layout)
	}
	t, err := parseLayout(f.layout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("edifact: format %s: %w", format, err)
	}
	return t, nil
}

// parseLayout parses v against layout, in UnknownTimezone unless the layout
// carries an offset. Only digits are accepted, apart from the offset's sign.
func parseLayout(layout, v string) (time.Time, error) {
	if i := strings.Index(layout, "-0700"); i >= 0 {
		if len(v) != len(layout) || (v[i] != '+' && v[i] != '-') || !isDigits(v[:i]+v[i+1:]) {
			return time.Time{}, fmt.Errorf("value %q is not in the form %s", v, layout)
		}
		return time.Parse(layout, v)
	}
	if !isDigits(v) {
		return time.Time{}, fmt.Errorf("value %q is not all digits", v)
	}
	return time.ParseInLocation(layout, v, UnknownTimezone)
}

// parseZoned decodes a value ending in a three-letter time zone.
func parseZoned(value, format, layout string) (time.Time, error) {
	if len(value) < 3 {
		return time.Time{}, fmt.Errorf("edifact: format %s: value %q has no time zone", format, value)
	}
	body, zone := value[:len(value)-3], value[len(value)-3:]
	if zone != "GMT" && zone != "UTC" {
		return time.Time{}, fmt.Errorf("%w: %q", ErrDateZone, zone)
	}
	if !isDigits(body) {
		return time.Time{}, fmt.Errorf("edifact: format %s: value %q is not all digits", format, body)
	}
	t, err := time.ParseInLocation(layout, body, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("edifact: format %s: %w", format, err)
	}
	return t, nil
}

// dateFormat is a Go layout plus how many of them the value holds. A range
// format carries two, so it decodes to a period rather than a point.
type dateFormat struct {
	layout string
	parts  int
}

// dateFormats maps code list 2379 qualifiers to Go layouts.
var dateFormats = map[string]dateFormat{
	"2":   {"020106", 1},            // DDMMYY
	"3":   {"010206", 1},            // MMDDYY
	"101": {"060102", 1},            // YYMMDD
	"102": {"20060102", 1},          // CCYYMMDD
	"201": {"0601021504", 1},        // YYMMDDHHMM
	"202": {"060102150405", 1},      // YYMMDDHHMMSS
	"203": {"200601021504", 1},      // CCYYMMDDHHMM
	"204": {"20060102150405", 1},    // CCYYMMDDHHMMSS
	"205": {"200601021504-0700", 1}, // CCYYMMDDHHMMZHHMM
	"301": {"0601021504", 1},        // YYMMDDHHMMZZZ
	"401": {"1504", 1},              // HHMM
	"402": {"150405", 1},            // HHMMSS
	"404": {"150405", 1},            // HHMMSSZZZ
	"602": {"2006", 1},              // CCYY
	"610": {"200601", 1},            // CCYYMM
	"718": {"20060102", 2},          // CCYYMMDD-CCYYMMDD
	"719": {"200601021504", 2},      // CCYYMMDDHHMM-CCYYMMDDHHMM
}

// zonedFormats end in a time zone written as three letters, such as GMT. The
// layout in dateFormats covers everything before it.
var zonedFormats = map[string]bool{
	"301": true,
	"404": true,
}
