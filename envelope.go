package edifact

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Envelope structure the reader cannot carry on from.
var (
	ErrNoUNB      = errors.New("edifact: interchange does not open with UNB")
	ErrUnexpected = errors.New("edifact: service segment out of place")
	ErrUnclosed   = errors.New("edifact: interchange ends with a message or functional group still open")
)

// ErrNoContent reports an interchange with no messages, a functional group with
// no messages, or a message with no segments between UNH and UNT.
var ErrNoContent = errors.New("edifact: nothing to enclose")

// ErrGroupMismatch reports a message whose type, version, release or
// controlling agency differs from its functional group's, which syntax
// versions 1 to 3 forbid.
var ErrGroupMismatch = errors.New("edifact: message does not match its functional group")

// ErrIncompleteHeader reports a UNB, UNG or UNH missing an element ISO 9735
// makes mandatory.
var ErrIncompleteHeader = errors.New("edifact: header is missing mandatory elements")

// assembler distributes a stream of segments into an interchange, sorting
// service segments into headers and the rest into the open message.
type assembler struct {
	ix    *Interchange
	group *FunctionalGroup
	msg   *Message
	diags []Diagnostic

	idx        int // index of the segment last added, counting UNB as 0
	inGroup    int // messages seen in the open functional group
	groupCount int
	msgCount   int
	closed     bool
}

func newAssembler() *assembler {
	return &assembler{ix: &Interchange{Delimiters: StandardDelimiters()}}
}

// add feeds one segment. It errors only where the envelope is broken past
// recovery. Anything countable becomes a diagnostic.
func (a *assembler) add(s *Segment) error {
	defer func() { a.idx++ }()

	if a.closed {
		return fmt.Errorf("%w: %s after UNZ", ErrUnexpected, s.Tag)
	}
	if a.ix.header == nil && s.Tag != "UNB" {
		return fmt.Errorf("%w: found %s", ErrNoUNB, s.Tag)
	}

	switch s.Tag {
	case "UNB":
		if a.ix.header != nil {
			return fmt.Errorf("%w: a second UNB", ErrUnexpected)
		}
		a.ix.header = s
		a.checkHeader(s, unbMandatory, Diagnostic{SegmentIdx: a.idx + 1})

	case "UNG":
		if a.msg != nil || a.group != nil {
			return fmt.Errorf("%w: UNG inside an open %s", ErrUnexpected, a.openWhat())
		}
		a.group = &FunctionalGroup{header: s}
		a.ix.groups = append(a.ix.groups, a.group)
		a.checkHeader(s, ungRules(a.ix), Diagnostic{SegmentIdx: a.idx + 1})
		a.inGroup = 0
		a.groupCount++

	case "UNE":
		if a.msg != nil {
			return fmt.Errorf("%w: UNE inside an open message", ErrUnexpected)
		}
		if a.group == nil {
			return fmt.Errorf("%w: UNE with no open functional group", ErrUnexpected)
		}
		a.checkTrailer(s, "UNE", a.inGroup, a.group.ControlRef(), Diagnostic{SegmentIdx: a.idx + 1})
		if a.inGroup == 0 {
			a.diags = append(a.diags, Diagnostic{
				Severity: Error, Tag: "UNG", SegmentIdx: a.idx + 1,
				Message: "functional group has no messages",
			})
		}
		a.group = nil

	case "UNH":
		if a.msg != nil {
			return fmt.Errorf("%w: UNH inside an open message", ErrUnexpected)
		}
		a.msg = &Message{header: s, group: a.group}
		a.ix.messages = append(a.ix.messages, a.msg)
		a.inGroup++
		a.msgCount++
		a.checkHeader(s, unhRules(a.ix), Diagnostic{MessageIdx: a.msgCount, MessageRef: a.msg.Ref(), SegmentIdx: 1})
		if msg := groupMismatch(a.ix, a.msg); msg != "" {
			a.diags = append(a.diags, Diagnostic{
				Severity: Error, MessageIdx: a.msgCount, MessageRef: a.msg.Ref(),
				Tag: "UNH", SegmentIdx: 1, Message: msg,
			})
		}

	case "UNT":
		if a.msg == nil {
			return fmt.Errorf("%w: UNT with no open message", ErrUnexpected)
		}
		at := Diagnostic{MessageIdx: a.msgCount, MessageRef: a.msg.Ref(), SegmentIdx: countUNT(a.msg)}
		a.checkTrailer(s, "UNT", countUNT(a.msg), a.msg.Ref(), at)
		if len(a.msg.segments) == 0 {
			at.Severity, at.Tag, at.Message = Error, "UNT", "message has no segments between UNH and UNT"
			a.diags = append(a.diags, at)
		}
		a.msg = nil

	case "UNZ":
		if a.msg != nil || a.group != nil {
			return fmt.Errorf("%w: UNZ inside an open %s", ErrUnexpected, a.openWhat())
		}
		want := a.msgCount
		if a.groupCount > 0 {
			want = a.groupCount
		}
		a.checkTrailer(s, "UNZ", want, a.ix.ControlRef(), Diagnostic{SegmentIdx: a.idx + 1})
		a.closed = true

	default:
		if a.msg == nil {
			return fmt.Errorf("%w: %s outside any message", ErrUnexpected, s.Tag)
		}
		a.msg.segments = append(a.msg.segments, s)
	}
	return nil
}

// done closes the assembler, returning what it built.
func (a *assembler) done() (*Interchange, []Diagnostic, error) {
	if a.ix.header == nil {
		return nil, nil, ErrNoUNB
	}
	if a.msg != nil || a.group != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnclosed, a.openWhat())
	}
	if !a.closed {
		a.diags = append(a.diags, Diagnostic{
			Severity:   Warning,
			Tag:        "UNZ",
			SegmentIdx: a.idx,
			Message:    "interchange has no UNZ",
		})
	}
	if len(a.ix.messages) == 0 {
		a.diags = append(a.diags, Diagnostic{Severity: Error, Tag: "UNB", SegmentIdx: 1, Message: "interchange has no messages"})
	}
	if msg := mixedGrouping(a.ix); msg != "" {
		a.diags = append(a.diags, Diagnostic{Severity: Error, Tag: "UNG", Message: msg})
	}
	a.ix.Diagnostics = a.diags
	return a.ix, a.diags, nil
}

func (a *assembler) openWhat() string {
	if a.msg != nil {
		return "message"
	}
	if a.group != nil {
		return "functional group"
	}
	return "interchange"
}

// checkHeader reports, as a warning at at, the mandatory components s lacks.
func (a *assembler) checkHeader(s *Segment, rules []mandatory, at Diagnostic) {
	if missing := missingIn(s, rules); len(missing) > 0 {
		at.Severity, at.Tag = Warning, s.Tag
		at.Message = "missing mandatory elements: " + strings.Join(missing, ", ")
		a.diags = append(a.diags, at)
	}
}

// checkTrailer compares a trailer's count and reference against what was seen.
// Counts are compared numerically, since a sender may pad them. at carries the
// trailer's location.
func (a *assembler) checkTrailer(s *Segment, tag string, wantCount int, wantRef string, at Diagnostic) {
	report := func(msg string) {
		d := at
		d.Severity, d.Tag, d.Message = Error, tag, msg
		a.diags = append(a.diags, d)
	}

	gotCount, err := strconv.Atoi(s.Element(1))
	switch {
	case err != nil:
		report(fmt.Sprintf("count %q is not a number", s.Element(1)))
	case gotCount != wantCount:
		report(fmt.Sprintf("count is %d, counted %d", gotCount, wantCount))
	}

	if got := s.Element(2); got != wantRef {
		report(fmt.Sprintf("reference %q does not match %q", got, wantRef))
	}
}

// mandatory is a component ISO 9735 requires. When optional is set, its
// element is conditional and the component is required only if the element is
// present.
type mandatory struct {
	elem, comp int
	name       string
	optional   bool
}

var unbMandatory = []mandatory{
	{1, 1, "syntax identifier", false},
	{1, 2, "syntax version", false},
	{2, 1, "sender", false},
	{3, 1, "recipient", false},
	{4, 1, "date of preparation", false},
	{4, 2, "time of preparation", false},
	{5, 1, "control reference", false},
	{6, 1, "recipient reference", true},
}

// ungMandatory applies to syntax versions 1 to 3.
var ungMandatory = []mandatory{
	{1, 1, "functional group identification", false},
	{2, 1, "application sender identification", false},
	{3, 1, "application recipient identification", false},
	{4, 1, "date of preparation", false},
	{4, 2, "time of preparation", false},
	{5, 1, "functional group reference", false},
	{6, 1, "controlling agency", false},
	{7, 1, "message version", false},
	{7, 2, "message release", false},
}

var ungMandatoryV4 = []mandatory{
	{2, 1, "application sender identification", true},
	{3, 1, "application recipient identification", true},
	{4, 1, "date of preparation", true},
	{4, 2, "time of preparation", true},
	{5, 1, "functional group reference", false},
	{7, 1, "message version", true},
	{7, 2, "message release", true},
}

// unhMandatoryV1 applies to syntax version 1, where the message release and
// controlling agency are conditional.
var unhMandatoryV1 = []mandatory{
	{1, 1, "message reference", false},
	{2, 1, "message type", false},
	{2, 2, "message version", false},
	{4, 1, "sequence of transfers", true},
}

// unhMandatory applies from syntax version 2.
var unhMandatory = []mandatory{
	{1, 1, "message reference", false},
	{2, 1, "message type", false},
	{2, 2, "message version", false},
	{2, 3, "message release", false},
	{2, 4, "controlling agency", false},
	{4, 1, "sequence of transfers", true},
	{5, 1, "message subset identification", true},
	{6, 1, "implementation guideline identification", true},
	{7, 1, "scenario identification", true},
}

// missingIn names the mandatory components of s that are empty.
func missingIn(s *Segment, rules []mandatory) []string {
	var out []string
	for _, r := range rules {
		if r.optional && !present(s, r.elem) {
			continue
		}
		if s.Component(r.elem, r.comp) == "" {
			out = append(out, r.name)
		}
	}
	return out
}

// present reports whether element i has any non-empty component.
func present(s *Segment, i int) bool {
	for _, c := range s.Components(i) {
		if c != "" {
			return true
		}
	}
	return false
}

// ungRules returns the UNG rules for the syntax version ix declares.
func ungRules(ix *Interchange) []mandatory {
	if ix.SyntaxVersion() == "4" {
		return ungMandatoryV4
	}
	return ungMandatory
}

// unhRules returns the UNH rules for the syntax version ix declares.
func unhRules(ix *Interchange) []mandatory {
	if ix.SyntaxVersion() == "1" {
		return unhMandatoryV1
	}
	return unhMandatory
}

// checkHeaders reports the first UNB, UNG or UNH missing a mandatory component.
func checkHeaders(ix *Interchange) error {
	incomplete := func(where string, missing []string) error {
		return fmt.Errorf("%w: %s: %s", ErrIncompleteHeader, where, strings.Join(missing, ", "))
	}
	if missing := missingIn(ix.header, unbMandatory); len(missing) > 0 {
		return incomplete("UNB", missing)
	}
	ung := ungRules(ix)
	for i, g := range ix.groups {
		if missing := missingIn(g.header, ung); len(missing) > 0 {
			return incomplete(fmt.Sprintf("UNG of functional group %d (%s)", i+1, g.ControlRef()), missing)
		}
	}
	for i, m := range ix.messages {
		if missing := missingIn(m.header, unhRules(ix)); len(missing) > 0 {
			return incomplete(fmt.Sprintf("UNH of message %d (%s)", i+1, m.Ref()), missing)
		}
	}
	return nil
}

// envelopeTags are the service segments that frame messages, and so cannot
// appear among a message's own segments. UNS can, so it is not listed.
var envelopeTags = map[string]bool{
	"UNA": true, "UNB": true, "UNG": true, "UNH": true, "UNT": true, "UNE": true, "UNZ": true,
}

// groupMismatch describes how m differs from its functional group in type,
// version, release or controlling agency, or returns "" when it does not. Syntax version 4 allows several
// message types in one functional group, so nothing is compared there.
func groupMismatch(ix *Interchange, m *Message) string {
	g := m.group
	if g == nil || ix.SyntaxVersion() == "4" {
		return ""
	}
	k := m.Key()
	for _, f := range []struct{ name, group, msg string }{
		{"type", g.MessageType(), *k.Type},
		{"version", g.Version(), *k.Version},
		{"release", g.Release(), *k.Release},
		{"controlling agency", g.Agency(), *k.Agency},
	} {
		if f.group != "" && f.msg != f.group {
			return fmt.Sprintf("message %s %s differs from the functional group's %s", f.name, f.msg, f.group)
		}
	}
	return ""
}

// countUNT returns what a message's UNT should carry, which counts every
// segment from UNH to UNT inclusive.
func countUNT(m *Message) int { return len(m.segments) + 2 }

// envelopeSegments returns every segment of ix in write order, with the
// trailers built from what is actually there. Nothing is serialised here.
//
// With functional groups, each is written in Groups order, holding its messages
// in Messages order, so a functional group whose messages are not contiguous in
// Messages is still written once. Segments in omit are left out.
//
// It refuses an interchange with no UNB, inconsistent grouping, no messages, a
// functional group with no messages, a message that differs from its
// functional group, an envelope segment inside a message, and a message left
// with no segments.
func envelopeSegments(ix *Interchange, omit map[*Segment]bool) ([]*Segment, error) {
	if ix.header == nil {
		return nil, ErrNoUNB
	}
	if err := checkGrouping(ix); err != nil {
		return nil, err
	}
	if len(ix.messages) == 0 {
		return nil, fmt.Errorf("%w: the interchange has no messages", ErrNoContent)
	}
	for i, g := range ix.groups {
		held := false
		for _, m := range ix.messages {
			held = held || m.group == g
		}
		if !held {
			return nil, fmt.Errorf("%w: functional group %d (%s) has no messages", ErrNoContent, i+1, g.ControlRef())
		}
	}
	for i, m := range ix.messages {
		if msg := groupMismatch(ix, m); msg != "" {
			return nil, fmt.Errorf("%w: message %d (%s): %s", ErrGroupMismatch, i+1, m.Ref(), msg)
		}
		kept := 0
		for _, s := range m.segments {
			if envelopeTags[s.Tag] {
				return nil, fmt.Errorf("%w: %s inside message %d (%s)", ErrUnexpected, s.Tag, i+1, m.Ref())
			}
			if !omit[s] {
				kept++
			}
		}
		if kept == 0 {
			return nil, fmt.Errorf("%w: message %d (%s) has no segments", ErrNoContent, i+1, m.Ref())
		}
	}

	out := []*Segment{ix.header}
	writeMessage := func(m *Message) {
		var segs []*Segment
		for _, s := range m.segments {
			if !omit[s] {
				segs = append(segs, s)
			}
		}
		out = append(out, m.header)
		out = append(out, segs...)
		out = append(out, trailer("UNT", len(segs)+2, m.Ref()))
	}

	if len(ix.groups) == 0 {
		for _, m := range ix.messages {
			writeMessage(m)
		}
		return append(out, trailer("UNZ", len(ix.messages), ix.ControlRef())), nil
	}

	for _, g := range ix.groups {
		out = append(out, g.header)
		n := 0
		for _, m := range ix.messages {
			if m.group == g {
				writeMessage(m)
				n++
			}
		}
		out = append(out, trailer("UNE", n, g.ControlRef()))
	}
	return append(out, trailer("UNZ", len(ix.groups), ix.ControlRef())), nil
}

// checkGrouping enforces the standard's all-or-nothing rule, and that every
// functional group a message names is one the interchange holds.
func checkGrouping(ix *Interchange) error {
	known := make(map[*FunctionalGroup]bool, len(ix.groups))
	for _, g := range ix.groups {
		known[g] = true
	}

	for _, m := range ix.messages {
		if m.group != nil && !known[m.group] {
			return fmt.Errorf("edifact: message %s names a functional group the interchange does not hold", m.Ref())
		}
	}
	if msg := mixedGrouping(ix); msg != "" {
		return fmt.Errorf("edifact: %s", msg)
	}
	return nil
}

// mixedGrouping describes how ix breaks the rule that an interchange holds only
// functional groups or only messages, or returns "" when it does not.
func mixedGrouping(ix *Interchange) string {
	var grouped, loose int
	for _, m := range ix.messages {
		if m.group == nil {
			loose++
		} else {
			grouped++
		}
	}
	if len(ix.groups) > 0 && loose > 0 {
		return fmt.Sprintf("%d messages are in a functional group and %d are not", grouped, loose)
	}
	return ""
}

func trailer(tag string, count int, ref string) *Segment {
	s := &Segment{Tag: tag}
	s.Set(1, 1, strconv.Itoa(count))
	s.Set(2, 1, ref)
	return s
}
