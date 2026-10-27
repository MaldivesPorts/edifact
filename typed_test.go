package edifact

import (
	"errors"
	"math"
	"testing"
	"time"
)

func seg(tag string, elems ...[]string) *Segment {
	s := &Segment{Tag: tag, elements: make([][][]string, len(elems))}
	for i, e := range elems {
		if len(e) > 0 {
			s.elements[i] = [][]string{e}
		}
	}
	return s
}

func TestFloatAcceptsBothDecimalMarks(t *testing.T) {
	s := seg("MEA", []string{"WT"}, nil, []string{"KGM", "19200.5"})
	got, err := s.Float(3, 2)
	if err != nil || got != 19200.5 {
		t.Fatalf("dot: %v %v", got, err)
	}

	s.Set(3, 2, "19200,5")
	got, err = s.Float(3, 2)
	if err != nil || got != 19200.5 {
		t.Fatalf("comma: %v %v", got, err)
	}
}

func TestFloatAndIntEdges(t *testing.T) {
	s := seg("MEA", []string{"WT"}, nil, []string{"KGM", ""})

	if _, err := s.Float(3, 2); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty Float err = %v, want ErrEmpty", err)
	}
	if _, err := s.Float(9, 9); !errors.Is(err, ErrEmpty) {
		t.Errorf("absent Float err = %v, want ErrEmpty", err)
	}

	s.Set(3, 2, "not a number")
	if _, err := s.Float(3, 2); err == nil || errors.Is(err, ErrEmpty) {
		t.Errorf("malformed Float err = %v, want a parse error", err)
	}

	s.Set(3, 2, "0010204")
	if got, err := s.Int(3, 2); err != nil || got != 10204 {
		t.Errorf("leading zeros: %v %v", got, err)
	}
}

func TestTimeReadsAdjacentFormat(t *testing.T) {
	// DTM composite C507: qualifier, value, format.
	s := seg("DTM", []string{"137", "2601131230", "201"})

	got, err := s.Time(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 13, 12, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}

	s2 := seg("DTM", []string{"132", "260122", "101"})
	got, err = s2.Time(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 1, 22, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTimeFormats(t *testing.T) {
	cases := []struct {
		format DateFormat
		value  string
		want   time.Time
	}{
		{"101", "260113", time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC)},
		{"102", "20260113", time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC)},
		{"201", "2601131230", time.Date(2026, 1, 13, 12, 30, 0, 0, time.UTC)},
		{"203", "202601131230", time.Date(2026, 1, 13, 12, 30, 0, 0, time.UTC)},
		{"202", "260113123045", time.Date(2026, 1, 13, 12, 30, 45, 0, time.UTC)},
		{"204", "20260113123045", time.Date(2026, 1, 13, 12, 30, 45, 0, time.UTC)},
		{"401", "1230", time.Date(0, 1, 1, 12, 30, 0, 0, time.UTC)},
		{"402", "123045", time.Date(0, 1, 1, 12, 30, 45, 0, time.UTC)},
		{"2", "130126", time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC)},
		{"3", "011326", time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC)},
		{"602", "2026", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"610", "202601", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		got, err := ParseTime(c.value, c.format)
		if err != nil {
			t.Errorf("format %s: %v", c.format, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("format %s: got %v, want %v", c.format, got, c.want)
		}
	}
}

func TestParseTimeOffset(t *testing.T) {
	got, err := ParseTime("202601131230+0500", "205")
	if err != nil {
		t.Fatal(err)
	}
	if got.UTC().Hour() != 7 || got.UTC().Minute() != 30 {
		t.Errorf("got %v, want 07:30 UTC", got.UTC())
	}
}

func TestParseTimeRejections(t *testing.T) {
	if _, err := ParseTime("20260113", "999"); !errors.Is(err, ErrDateFormat) {
		t.Errorf("unknown format err = %v, want ErrDateFormat", err)
	}
	if _, err := ParseTime("2026011320260114", "718"); !errors.Is(err, ErrDateRange) {
		t.Errorf("range format err = %v, want ErrDateRange", err)
	}
	if _, err := ParseTime("", "101"); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty err = %v, want ErrEmpty", err)
	}
	if _, err := ParseTime("NOTADATE", "101"); err == nil {
		t.Error("malformed value accepted")
	}
}

func TestTwoDigitYearPivot(t *testing.T) {
	cases := map[string]int{"680113": 2068, "690113": 1969, "990113": 1999, "000113": 2000}
	for v, want := range cases {
		got, err := ParseTime(v, "101")
		if err != nil {
			t.Fatal(err)
		}
		if got.Year() != want {
			t.Errorf("%s -> %d, want %d", v, got.Year(), want)
		}
	}
}

func TestSetTimeAndBack(t *testing.T) {
	s := seg("DTM", []string{"137"})
	when := time.Date(2026, 9, 21, 14, 5, 0, 0, time.UTC)

	if err := s.SetTime(1, 2, when, "201"); err != nil {
		t.Fatal(err)
	}
	if got := s.Component(1, 2); got != "2609211405" {
		t.Errorf("value = %q", got)
	}
	if got := s.Component(1, 3); got != "201" {
		t.Errorf("format qualifier = %q", got)
	}
	back, err := s.Time(1, 2)
	if err != nil || !back.Equal(when) {
		t.Errorf("back = %v %v", back, err)
	}

	if err := s.SetTime(1, 2, when, "719"); !errors.Is(err, ErrDateRange) {
		t.Errorf("range SetTime err = %v", err)
	}
	if err := s.SetTime(1, 2, when, "999"); !errors.Is(err, ErrDateFormat) {
		t.Errorf("unknown SetTime err = %v", err)
	}
}

func TestSetFloat(t *testing.T) {
	s := seg("MEA", []string{"VGM"}, nil, []string{"KGM"})
	s.SetFloat(3, 2, 30360, -1)
	if got := s.Component(3, 2); got != "30360" {
		t.Errorf("got %q, want 30360", got)
	}
	s.SetFloat(3, 2, 19200.5, 1)
	if got := s.Component(3, 2); got != "19200.5" {
		t.Errorf("got %q, want 19200.5", got)
	}
}

func TestParseTimeRange(t *testing.T) {
	cases := []struct {
		format         DateFormat
		value          string
		wantLo, wantHi time.Time
	}{
		{"718", "2026011320260114",
			time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)},
		{"718", "20260113-20260114",
			time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)},
		{"719", "202601131200202601141800",
			time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
			time.Date(2026, 1, 14, 18, 0, 0, 0, time.UTC)},
		{"719", "202601131200-202601141800",
			time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
			time.Date(2026, 1, 14, 18, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		lo, hi, err := ParseTimeRange(c.value, c.format)
		if err != nil {
			t.Errorf("format %s %q: %v", c.format, c.value, err)
			continue
		}
		if !lo.Equal(c.wantLo) || !hi.Equal(c.wantHi) {
			t.Errorf("format %s %q: got %v..%v", c.format, c.value, lo, hi)
		}
	}

	if _, _, err := ParseTimeRange("20260113", "101"); err == nil {
		t.Error("a point format was accepted as a range")
	}
	if _, _, err := ParseTimeRange("2026", "718"); err == nil {
		t.Error("a short value was accepted")
	}
}

func TestSegmentTimeRange(t *testing.T) {
	s := seg("DTM", []string{"324", "202601131200202601141800", "719"})
	lo, hi, err := s.TimeRange(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if lo.Hour() != 12 || hi.Hour() != 18 {
		t.Errorf("got %v..%v", lo, hi)
	}
}

func TestZonedTime(t *testing.T) {
	want := time.Date(0, 1, 1, 12, 30, 45, 0, time.UTC)
	for _, v := range []string{"123045GMT", "123045UTC"} {
		got, err := ParseTime(v, "404")
		if err != nil || !got.Equal(want) {
			t.Errorf("%s: got %v %v", v, got, err)
		}
	}

	if _, err := ParseTime("123045EST", "404"); !errors.Is(err, ErrDateZone) {
		t.Errorf("unknown zone err = %v, want ErrDateZone", err)
	}
	if _, err := ParseTime("123045", "404"); !errors.Is(err, ErrDateZone) {
		t.Errorf("missing zone err = %v, want ErrDateZone", err)
	}
	if _, err := ParseTime("GM", "404"); err == nil {
		t.Error("a value shorter than a zone was accepted")
	}
}

func TestSetZonedTime(t *testing.T) {
	s := seg("DTM", []string{"137"})
	// A time in another zone is written in UTC, under the UTC label.
	when := time.Date(2026, 1, 1, 17, 30, 45, 0, time.FixedZone("UTC+5", 5*3600))
	if err := s.SetTime(1, 2, when, "404"); err != nil {
		t.Fatal(err)
	}
	if got := s.Component(1, 2); got != "123045UTC" {
		t.Errorf("written %q, want 123045UTC", got)
	}
	back, err := s.Time(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if back.Hour() != 12 || back.Minute() != 30 || back.Second() != 45 || back.Location() != time.UTC {
		t.Errorf("read back %v", back)
	}
}

func TestZonedDateTime(t *testing.T) {
	want := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, v := range []string{"2601011200GMT", "2601011200UTC"} {
		got, err := ParseTime(v, "301")
		if err != nil || !got.Equal(want) {
			t.Errorf("%s: got %v %v", v, got, err)
		}
	}
	if _, err := ParseTime("2601011200", "301"); !errors.Is(err, ErrDateZone) {
		t.Errorf("missing zone err = %v, want ErrDateZone", err)
	}

	s := seg("DTM", []string{"137"})
	when := time.Date(2026, 1, 1, 17, 0, 0, 0, time.FixedZone("UTC+5", 5*3600))
	if err := s.SetTime(1, 2, when, "301"); err != nil {
		t.Fatal(err)
	}
	if got := s.Component(1, 2); got != "2601011200UTC" {
		t.Errorf("written %q, want 2601011200UTC", got)
	}
	if back, err := s.Time(1, 2); err != nil || !back.Equal(want) {
		t.Errorf("read back %v %v", back, err)
	}
}

func TestFloatAcceptsOnlyEDIFACTNumerics(t *testing.T) {
	s := seg("MEA", []string{"WT"}, nil, []string{"KGM", ""})
	for v, want := range map[string]float64{
		"0": 0, "007": 7, "-5": -5, "2.5": 2.5, "-0,25": -0.25, "19200.50": 19200.5,
		".5": 0.5, ",5": 0.5, "-.5": -0.5, "1e3": 1000, "2.5E-1": 0.25, "1E+2": 100,
	} {
		s.Set(3, 2, v)
		if got, err := s.Float(3, 2); err != nil || got != want {
			t.Errorf("%q: got %v %v, want %v", v, got, err, want)
		}
	}
	for _, v := range []string{
		"NaN", "Inf", "-Inf", "0x1p4", "2.", "2,", ".", "-", "+5", "E3", "1E", "1E+-3", "1E3.5",
		"1.000.000", "1,000.5", "1 000", "--5", "5-",
	} {
		s.Set(3, 2, v)
		if _, err := s.Float(3, 2); !errors.Is(err, ErrNumeric) {
			t.Errorf("%q: err = %v, want ErrNumeric", v, err)
		}
	}
}

func TestIntAcceptsOnlyWholeEDIFACTNumerics(t *testing.T) {
	s := seg("UNT", []string{""})
	for v, want := range map[string]int{"0": 0, "007": 7, "-5": -5, "39": 39} {
		s.Set(1, 1, v)
		if got, err := s.Int(1, 1); err != nil || got != want {
			t.Errorf("%q: got %v %v, want %v", v, got, err, want)
		}
	}
	for _, v := range []string{"+5", "2.5", "2,0", "-", "1e3", "0x10", "1 000", "5-"} {
		s.Set(1, 1, v)
		if _, err := s.Int(1, 1); !errors.Is(err, ErrNumeric) {
			t.Errorf("%q: err = %v, want ErrNumeric", v, err)
		}
	}
}

func TestSetFloatRefusesNonFiniteValues(t *testing.T) {
	s := seg("MEA", []string{"WT"}, nil, []string{"KGM", "1"})
	for _, v := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		if err := s.SetFloat(3, 2, v, -1); !errors.Is(err, ErrNumeric) {
			t.Errorf("%v: err = %v, want ErrNumeric", v, err)
		}
	}
	if got := s.Component(3, 2); got != "1" {
		t.Errorf("a refused value was written: %q", got)
	}
}

func TestTimesWithoutAZoneAreInUnknownTimezone(t *testing.T) {
	cases := []struct {
		value  string
		format DateFormat
		want   *time.Location
	}{
		{"202601011200", "203", UnknownTimezone},
		{"260101", "101", UnknownTimezone},
		{"2601011200UTC", "301", time.UTC},
		{"120000GMT", "404", time.UTC},
	}
	for _, c := range cases {
		got, err := ParseTime(c.value, c.format)
		if err != nil {
			t.Fatalf("%s %s: %v", c.format, c.value, err)
		}
		if got.Location() != c.want {
			t.Errorf("%s %s: location %v, want %v", c.format, c.value, got.Location(), c.want)
		}
	}

	got, err := ParseTime("202601011200+0000", "205")
	if err != nil || got.Location() == UnknownTimezone {
		t.Errorf("205 with an offset: %v %v, want a known zone", got.Location(), err)
	}

	start, end, err := ParseTimeRange("20260101-20260102", "718")
	if err != nil || start.Location() != UnknownTimezone || end.Location() != UnknownTimezone {
		t.Errorf("718: %v %v %v", start.Location(), end.Location(), err)
	}

	// The digits written back are the digits read.
	s := seg("DTM", []string{"137", "202601011200", "203"})
	read, _ := s.Time(1, 2)
	if err := s.SetTime(1, 2, read, "203"); err != nil || s.Component(1, 2) != "202601011200" {
		t.Errorf("round trip wrote %q %v", s.Component(1, 2), err)
	}
}

func TestParseTimeAcceptsOnlyDigits(t *testing.T) {
	for _, c := range []struct {
		value  string
		format DateFormat
	}{
		{"+10101", "101"}, {"-10101", "101"}, {"+1010112", "201"}, {"+10101", "2"},
		{"+101011200UTC", "301"}, {" 60101", "101"}, {"20260101120+0000", "205"},
		{"20260101-2026010+", "718"},
	} {
		if _, err := ParseTime(c.value, c.format); err == nil && c.format != "718" {
			t.Errorf("%s %q accepted", c.format, c.value)
		}
		if c.format == "718" {
			if _, _, err := ParseTimeRange(c.value, c.format); err == nil {
				t.Errorf("%s %q accepted", c.format, c.value)
			}
		}
	}
	for _, c := range []struct {
		value  string
		format DateFormat
	}{
		{"260101", "101"}, {"202601011200+0500", "205"}, {"202601011200-0130", "205"}, {"2601011200GMT", "301"},
	} {
		if _, err := ParseTime(c.value, c.format); err != nil {
			t.Errorf("%s %q: %v", c.format, c.value, err)
		}
	}
}
