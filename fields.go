package edifact

import (
	"fmt"
	"strings"
	"time"
)

// Fields reads typed values out of a message's segments and records a warning
// for each value that is present but will not convert, so a caller can read
// many values and handle the problems once. An absent segment or an empty
// position gives the zero value and records nothing.
//
// The zero value records warnings that name only the segment's tag and
// qualifier.
type Fields struct {
	// Diagnostics holds the warnings in the order they were recorded.
	Diagnostics []Diagnostic
	// msg is the message the warnings are located in, or nil.
	msg *Message
}

// NewFields returns a Fields whose warnings are located in m: the message's
// position and reference, the segment's position, and the segment groups
// enclosing it.
func NewFields(m *Message) *Fields { return &Fields{msg: m} }

// Float reads component comp of element elem, both counting from 1, as a
// number, the way [Segment.Float] does. A value that will not convert gives 0
// and a warning starting with what.
func (f *Fields) Float(s *Segment, elem, comp int, what string) float64 {
	v := s.Component(elem, comp)
	if v == "" {
		return 0
	}
	n, err := parseFloat(v)
	f.check(s, elem, comp, what, err)
	return n
}

// Int reads component comp of element elem, both counting from 1, as a whole
// number, the way [Segment.Int] does. A value that will not convert gives 0 and
// a warning starting with what.
func (f *Fields) Int(s *Segment, elem, comp int, what string) int {
	v := s.Component(elem, comp)
	if v == "" {
		return 0
	}
	n, err := parseInt(v)
	f.check(s, elem, comp, what, err)
	return n
}

// Time reads component comp of element elem, both counting from 1, as a date
// and time in the format named by the component after it, the way
// [Segment.Time] does. A value that will not convert gives the zero time and a
// warning starting with what.
func (f *Fields) Time(s *Segment, elem, comp int, what string) time.Time {
	return f.TimeIn(s, elem, comp, DateFormat(s.Component(elem, comp+1)), what)
}

// TimeIn reads component comp of element elem, both counting from 1, as a date
// and time in the given format, the way [Segment.TimeIn] does. A value that
// will not convert gives the zero time and a warning starting with what.
func (f *Fields) TimeIn(s *Segment, elem, comp int, format DateFormat, what string) time.Time {
	v := s.Component(elem, comp)
	if v == "" {
		return time.Time{}
	}
	t, err := ParseTime(v, format)
	f.check(s, elem, comp, what, err)
	return t
}

// TimeRange reads component comp of element elem, both counting from 1, as a
// period in the format named by the component after it, the way
// [Segment.TimeRange] does. A value that will not convert gives zero times and
// a warning starting with what.
func (f *Fields) TimeRange(s *Segment, elem, comp int, what string) (start, end time.Time) {
	v := s.Component(elem, comp)
	if v == "" {
		return time.Time{}, time.Time{}
	}
	start, end, err := ParseTimeRange(v, DateFormat(s.Component(elem, comp+1)))
	f.check(s, elem, comp, what, err)
	return start, end
}

// Addf records a warning at segment s, with a message formatted from format
// and args, for a problem only the caller can see, such as a value that another
// value makes necessary. A nil s records it against the message as a whole.
func (f *Fields) Addf(s *Segment, format string, args ...any) {
	f.add(s, fmt.Sprintf(format, args...))
}

// check records a warning for err, naming what and the position.
func (f *Fields) check(s *Segment, elem, comp int, what string, err error) {
	if err == nil {
		return
	}
	msg := fmt.Sprintf("element %d component %d: %s", elem, comp, strings.ReplaceAll(err.Error(), "edifact: ", ""))
	if what != "" {
		msg = what + ": " + msg
	}
	f.add(s, msg)
}

// add records a warning at s, located in f's message when there is one.
func (f *Fields) add(s *Segment, msg string) {
	d := Diagnostic{Severity: Warning, Message: msg}
	if s != nil {
		d.Tag, d.Qualifier = s.Tag, s.Qualifier()
	}
	if m := f.msg; m != nil {
		d.MessageIdx, d.MessageRef = m.position(), m.Ref()
		if i := index(m.segments, s); s != nil && i >= 0 {
			d.SegmentIdx = i + 2
			d.Path = groupPath(m.groups, s)
		}
	}
	f.Diagnostics = append(f.Diagnostics, d)
}

// position returns the message's place in its interchange, counting from 1, or
// 0 when it is in none.
func (m *Message) position() int {
	if m.ix == nil {
		return 0
	}
	for i, o := range m.ix.messages {
		if o == m {
			return i + 1
		}
	}
	return 0
}

// groupPath names the segment groups enclosing s, such as /SG2/SG3, or gives
// "" when s is at the top level of the message.
func groupPath(groups []*SegmentGroup, s *Segment) string {
	for _, g := range groups {
		if contains(g.segments, s) {
			return "/" + g.Name + groupPath(g.groups, s)
		}
	}
	return ""
}
