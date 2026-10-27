package edifact

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// fieldsInterchange holds two messages. The second has a stowage cell with a
// weight that is not a number, a container with a date that is not a date,
// and a period and count that will not convert.
const fieldsInterchange = `UNA:+.? '
UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'
UNH+M1+BAPLIE:D:95B:UN'
BGM++BAY1+9'
DTM+137:202601011200:203'
UNT+4+M1'
UNH+M2+BAPLIE:D:95B:UN'
BGM++BAY2+9'
DTM+137:2026010112AB:203'
LOC+147+0010204::5'
MEA+WT++KGM:12A'
QTY+1:X'
EQD+CN+XXXU1234567+22G1'
DTM+7:20260101-2026010:718'
UNT+9+M2'
UNZ+2+REF1'`

func fieldsSpec() Spec {
	return Spec{
		Key: Key{Type: Is("BAPLIE")},
		Nodes: []Node{
			{Tag: "BGM", Required: true},
			{Tag: "DTM", Qualifier: "137", Required: true},
			{Group: "SG2", Max: Unbounded, Nodes: []Node{
				{Tag: "LOC", Qualifier: "147", Required: true},
				{Tag: "MEA", Qualifier: "WT"},
				{Tag: "QTY"},
				{Group: "SG3", Max: 3, Nodes: []Node{
					{Tag: "EQD", Qualifier: "CN", Required: true},
					{Tag: "DTM", Qualifier: "7"},
				}},
			}},
		},
	}
}

func readFields(t *testing.T) *Message {
	t.Helper()
	ix, err := ReadWithSpec(strings.NewReader(fieldsInterchange), fieldsSpec())
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Diagnostics) > 0 {
		t.Fatalf("diagnostics: %v", ix.Diagnostics)
	}
	return ix.Message(2)
}

func TestFieldsRecordsLocatedWarnings(t *testing.T) {
	m := readFields(t)
	f := NewFields(m)
	cell := m.FindGroup("SG2")
	eq := cell.FindGroup("SG3")

	if v := f.Float(cell.Find("MEA", "WT"), 3, 2, "weight"); v != 0 {
		t.Errorf("weight = %v, want 0", v)
	}
	if v := f.Int(cell.Find("QTY", ""), 1, 2, "count"); v != 0 {
		t.Errorf("count = %v, want 0", v)
	}
	if v := f.Time(m.Find("DTM", "137"), 1, 2, "document date"); !v.IsZero() {
		t.Errorf("document date = %v, want zero", v)
	}
	if lo, hi := f.TimeRange(eq.Find("DTM", "7"), 1, 2, ""); !lo.IsZero() || !hi.IsZero() {
		t.Errorf("period = %v %v, want zero", lo, hi)
	}

	want := []Diagnostic{
		{Severity: Warning, MessageIdx: 2, MessageRef: "M2", Path: "/SG2", Tag: "MEA", Qualifier: "WT", SegmentIdx: 5,
			Message: `weight: element 3 component 2: not a valid EDIFACT numeric value: "12A"`},
		{Severity: Warning, MessageIdx: 2, MessageRef: "M2", Path: "/SG2", Tag: "QTY", Qualifier: "1", SegmentIdx: 6,
			Message: `count: element 1 component 2: not a valid EDIFACT numeric value: "X"`},
		{Severity: Warning, MessageIdx: 2, MessageRef: "M2", Tag: "DTM", Qualifier: "137", SegmentIdx: 3,
			Message: `document date: element 1 component 2: format 203: value "2026010112AB" is not all digits`},
		{Severity: Warning, MessageIdx: 2, MessageRef: "M2", Path: "/SG2/SG3", Tag: "DTM", Qualifier: "7", SegmentIdx: 8,
			Message: `element 1 component 2: format 718 end: value "-2026010" is not all digits`},
	}
	if len(f.Diagnostics) != len(want) {
		t.Fatalf("got %d diagnostics, want %d: %v", len(f.Diagnostics), len(want), f.Diagnostics)
	}
	wantErr := []error{ErrNumeric, ErrNumeric, nil, nil}
	for i, d := range f.Diagnostics {
		err := d.Err
		d.Err = nil
		if d != want[i] {
			t.Errorf("diagnostic %d:\n got  %+v\n want %+v", i, d, want[i])
		}
		if err == nil || (wantErr[i] != nil && !errors.Is(err, wantErr[i])) {
			t.Errorf("diagnostic %d: Err = %v, want one wrapping %v", i, err, wantErr[i])
		}
	}
}

func TestFieldsReadsValidValuesWithoutWarnings(t *testing.T) {
	ix, err := ReadWithSpec(strings.NewReader(fieldsInterchange), fieldsSpec())
	if err != nil {
		t.Fatal(err)
	}
	m := ix.Message(1)
	f := NewFields(m)
	dtm := m.Find("DTM", "137")

	if got := f.Time(dtm, 1, 2, "document date"); !got.Equal(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("Time = %v", got)
	}
	if got := f.TimeIn(dtm, 1, 2, FormatCCYYMMDDHHMM, "document date"); got.Hour() != 12 {
		t.Errorf("TimeIn = %v", got)
	}
	if got := f.Int(dtm, 1, 1, "qualifier"); got != 137 {
		t.Errorf("Int = %v", got)
	}
	if got := f.Float(dtm, 1, 1, "qualifier"); got != 137 {
		t.Errorf("Float = %v", got)
	}
	if len(f.Diagnostics) > 0 {
		t.Errorf("diagnostics: %v", f.Diagnostics)
	}
}

func TestFieldsSkipsAbsentValues(t *testing.T) {
	m := readFields(t)
	f := NewFields(m)
	missing := m.Find("DTM", "999")
	bgm := m.Find("BGM", "")

	if f.Float(missing, 1, 1, "x") != 0 || f.Int(bgm, 1, 1, "x") != 0 ||
		!f.Time(missing, 1, 2, "x").IsZero() || !f.TimeIn(bgm, 9, 1, FormatCCYY, "x").IsZero() {
		t.Error("absent value did not give the zero value")
	}
	if lo, hi := f.TimeRange(nil, 1, 2, "x"); !lo.IsZero() || !hi.IsZero() {
		t.Error("absent period did not give zero times")
	}
	if len(f.Diagnostics) > 0 {
		t.Errorf("diagnostics: %v", f.Diagnostics)
	}
}

func TestFieldsTimeInUnknownFormat(t *testing.T) {
	m := readFields(t)
	f := NewFields(m)
	f.TimeIn(m.Find("BGM", ""), 2, 1, "999", "reference")
	if len(f.Diagnostics) != 1 || f.Diagnostics[0].Message != `reference: element 2 component 1: unknown date format qualifier: "999"` ||
		!errors.Is(f.Diagnostics[0].Err, ErrDateFormat) {
		t.Errorf("diagnostics: %v", f.Diagnostics)
	}
}

func TestFieldsAddf(t *testing.T) {
	m := readFields(t)
	f := NewFields(m)
	f.Addf(m.FindGroup("SG2").Find("LOC", "147"), "cell %s: no port of loading", "0010204")
	f.Addf(nil, "no transport details")

	if f.Diagnostics[0].Err != nil || f.Diagnostics[1].Err != nil {
		t.Error("Addf set Err")
	}
	got := []string{f.Diagnostics[0].String(), f.Diagnostics[1].String()}
	want := []string{
		"warning: message 2 (M2), segment 4, /SG2/LOC+147: cell 0010204: no port of loading",
		"warning: message 2 (M2): no transport details",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got  %q\nwant %q", got[i], want[i])
		}
	}
}

func TestFieldsZeroValueNamesOnlyTheSegment(t *testing.T) {
	var f Fields
	f.Float(seg("MEA", []string{"WT"}, nil, []string{"KGM", "X"}), 3, 2, "weight")
	f.Addf(nil, "plain")
	want := []string{
		`warning: MEA+WT: weight: element 3 component 2: not a valid EDIFACT numeric value: "X"`,
		"warning: plain",
	}
	for i, d := range f.Diagnostics {
		if d.String() != want[i] {
			t.Errorf("got  %q\nwant %q", d.String(), want[i])
		}
	}
}

func TestFieldsLocatesSegmentsInBuiltMessages(t *testing.T) {
	ix := NewInterchange()
	ix.AddMessage(Key{}).SetRef("A")
	m := ix.AddMessage(Key{})
	m.SetRef("B")
	m.Add("BGM", "", "X")
	g := m.AddGroup("SG1")
	tmp := g.AddGroup("SG2").AddComposite("TMP", []string{"2"}, []string{"HOT", "CEL"})

	f := NewFields(m)
	f.Float(tmp, 2, 1, "temperature")
	want := `warning: message 2 (B), segment 3, /SG1/SG2/TMP+2: temperature: element 2 component 1: not a valid EDIFACT numeric value: "HOT"`
	if len(f.Diagnostics) != 1 || f.Diagnostics[0].String() != want {
		t.Errorf("diagnostics: %v", f.Diagnostics)
	}

	var outside Fields
	outside.msg = &Message{}
	outside.Float(tmp, 2, 1, "temperature")
	if d := outside.Diagnostics[0]; d.MessageIdx != 0 || d.SegmentIdx != 0 || d.Tag != "TMP" {
		t.Errorf("segment outside the message: %+v", d)
	}
}
