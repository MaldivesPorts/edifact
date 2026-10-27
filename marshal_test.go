package edifact

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// docHeader is the document number and date from BGM and DTM+137.
type docHeader struct {
	Number string
	Date   time.Time
}

func (h *docHeader) UnmarshalEDIFACT(m *Message, f *Fields) error {
	if m.Key().Type == nil || *m.Key().Type != "BAPLIE" {
		return errors.New("not a bayplan")
	}
	h.Number = m.Find("BGM", "").Element(2)
	h.Date = f.Time(m.Find("DTM", "137"), 1, 2, "document date")
	return nil
}

func (h docHeader) MarshalEDIFACT(m *Message) error {
	m.Add("BGM", "", h.Number, "9")
	return m.Add("DTM", "137").SetTime(1, 2, h.Date, FormatCCYYMMDDHHMM)
}

// countOnly writes and never reads.
type countOnly struct{ N int }

func (c countOnly) MarshalEDIFACT(m *Message) error {
	m.Add("QTY").SetInt(1, 2, c.N)
	return nil
}

var (
	_ Unmarshaler = (*docHeader)(nil)
	_ Marshaler   = docHeader{}
	_ Marshaler   = countOnly{}
)

func TestUnmarshalReturnsDiagnosticsAndErrors(t *testing.T) {
	ix, err := ReadWithSpec(strings.NewReader(fieldsInterchange), fieldsSpec())
	if err != nil {
		t.Fatal(err)
	}

	var h docHeader
	diags, err := Unmarshal(ix.Message(1), &h)
	if err != nil || len(diags) != 0 || h.Number != "BAY1" || h.Date.Hour() != 12 {
		t.Errorf("message 1: %+v %v %v", h, diags, err)
	}

	h = docHeader{}
	diags, err = Unmarshal(ix.Message(2), &h)
	if err != nil || h.Number != "BAY2" || !h.Date.IsZero() {
		t.Errorf("message 2: %+v %v", h, err)
	}
	if len(diags) != 1 || diags[0].MessageIdx != 2 || diags[0].SegmentIdx != 3 || diags[0].Tag != "DTM" {
		t.Errorf("message 2 diagnostics: %v", diags)
	}

	other := NewInterchange().AddMessage(Key{Type: Is("MOVINS")})
	if _, err := Unmarshal(other, &h); err == nil || err.Error() != "not a bayplan" {
		t.Errorf("other type: err = %v", err)
	}
}

func TestMarshalWritesWhatTheTypeChooses(t *testing.T) {
	when := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)
	m := NewInterchange().AddMessage(Key{Type: Is("BAPLIE")})
	if err := Marshal(m, docHeader{Number: "BAY9", Date: when}); err != nil {
		t.Fatal(err)
	}
	if err := Marshal(m, countOnly{N: 3}); err != nil {
		t.Fatal(err)
	}
	segs := m.Segments()
	if len(segs) != 3 || segs[0].Element(2) != "BAY9" || segs[1].Component(1, 2) != "202601021504" ||
		segs[1].Component(1, 3) != "203" || segs[2].Component(1, 2) != "3" {
		t.Errorf("segments: %v", segs)
	}

	var back docHeader
	if diags, err := Unmarshal(m, &back); err != nil || len(diags) != 0 || back.Number != "BAY9" || !back.Date.Equal(when) {
		t.Errorf("read back: %+v %v %v", back, diags, err)
	}
}

func TestMarshalReturnsTheTypesError(t *testing.T) {
	m := NewInterchange().AddMessage(Key{})
	err := Marshal(m, docHeader{Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("valid header: %v", err)
	}
	if err := Marshal(m, marshalFunc(func(m *Message) error {
		return m.Add("DTM", "137").SetTime(1, 2, time.Now(), "999")
	})); !errors.Is(err, ErrDateFormat) {
		t.Errorf("err = %v, want ErrDateFormat", err)
	}
}

type marshalFunc func(*Message) error

func (f marshalFunc) MarshalEDIFACT(m *Message) error { return f(m) }

func TestNilMessage(t *testing.T) {
	if _, err := Unmarshal(nil, &docHeader{}); !errors.Is(err, ErrNoMessage) {
		t.Errorf("Unmarshal: err = %v", err)
	}
	if err := Marshal(nil, docHeader{}); !errors.Is(err, ErrNoMessage) {
		t.Errorf("Marshal: err = %v", err)
	}
}
