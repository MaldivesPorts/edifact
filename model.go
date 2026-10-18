package edifact

import "strings"

// Interchange is everything between UNB and UNZ.
type Interchange struct {
	// messages is the sole storage and carries write order.
	messages []*Message
	// groups holds UNG and UNE data, and is nil when there are no functional
	// groups.
	groups []*FunctionalGroup

	// header is the UNB segment.
	header *Segment

	// Delimiters are the separators read from the UNA, or the defaults, and
	// the ones Write uses.
	Delimiters Delimiters
	// Diagnostics are the problems Read found that did not stop it.
	Diagnostics []Diagnostic
}

// FunctionalGroup is the UNG and UNE pair bracketing a set of messages. It
// holds no messages. Each message points at its functional group.
type FunctionalGroup struct {
	// header is the UNG segment.
	header *Segment
	// ix is the interchange holding the functional group.
	ix *Interchange
}

// Message is everything between UNH and UNT.
type Message struct {
	// header is the UNH segment.
	header *Segment
	// group is nil when the message is not in a functional group.
	group *FunctionalGroup

	// segments is the sole storage and carries write order.
	segments []*Segment
	// groups is the top-level segment groups, a view over segments built by
	// validation or by AddGroup.
	groups []*SegmentGroup
}

// SegmentGroup is one repetition of a segment group declared by a spec, such as
// SG2. Its segments are pointers to the message's own, not copies, so the
// message stays the single storage and carries write order.
type SegmentGroup struct {
	// Name is the segment group's label in the spec, such as SG2.
	Name     string
	segments []*Segment
	groups   []*SegmentGroup

	msg    *Message
	parent *SegmentGroup
}

// Segment is one tagged line.
type Segment struct {
	// Tag is the segment code, without any explicit nesting and repetition
	// indices.
	Tag string
	// index holds the explicit nesting and repetition indices that follow the
	// segment code, as in DDD:1:2.
	index []string
	// floats holds the element and component positions, in the first
	// repetition, last written by SetFloat.
	floats map[[2]int]bool
	// elements holds each element's repetitions, and each repetition's
	// components. An empty element has no repetitions.
	elements [][][]string
}

// NewInterchange returns an empty interchange with the standard delimiters and
// a UNB naming syntax UNOA version 2.
func NewInterchange() *Interchange {
	ix := &Interchange{
		Delimiters: StandardDelimiters(),
		header:     &Segment{Tag: "UNB"},
	}
	ix.SetCharset(UNOA)
	ix.SetSyntaxVersion(2)
	return ix
}

// AddMessage appends a message outside any functional group whose UNH names k,
// and returns it.
func (ix *Interchange) AddMessage(k Key) *Message {
	return ix.AddMessageTo(nil, k)
}

// AddMessageTo appends a message in functional group g whose UNH names k, and
// returns it. A nil g adds the message outside any functional group.
func (ix *Interchange) AddMessageTo(g *FunctionalGroup, k Key) *Message {
	m := &Message{header: &Segment{Tag: "UNH"}, group: g}
	m.SetKey(k)
	ix.messages = append(ix.messages, m)
	return m
}

// AddFunctionalGroup appends a functional group whose UNG names messageType,
// and returns it.
func (ix *Interchange) AddFunctionalGroup(messageType string) *FunctionalGroup {
	g := &FunctionalGroup{header: &Segment{Tag: "UNG"}, ix: ix}
	g.SetMessageType(messageType)
	ix.groups = append(ix.groups, g)
	return g
}

// Grouped reports whether every message belongs to a functional group.
func (ix *Interchange) Grouped() bool {
	if len(ix.messages) == 0 {
		return false
	}
	for _, m := range ix.messages {
		if m.group == nil {
			return false
		}
	}
	return true
}

// Header returns the UNB segment, creating an empty one if there is none.
func (ix *Interchange) Header() *Segment {
	if ix.header == nil {
		ix.header = &Segment{Tag: "UNB"}
	}
	return ix.header
}

// Message returns message n, counting from 1, or nil when there is none.
func (ix *Interchange) Message(n int) *Message {
	if ix == nil || n < 1 || n > len(ix.messages) {
		return nil
	}
	return ix.messages[n-1]
}

// Messages returns the messages in write order. The slice is a copy.
func (ix *Interchange) Messages() []*Message {
	return append([]*Message(nil), ix.messages...)
}

// Groups returns the functional groups in write order, or nil when there are
// none. The slice is a copy.
func (ix *Interchange) Groups() []*FunctionalGroup {
	return append([]*FunctionalGroup(nil), ix.groups...)
}

// Header returns the UNG segment, creating an empty one if there is none. A nil
// g gives nil.
func (g *FunctionalGroup) Header() *Segment {
	if g == nil {
		return nil
	}
	if g.header == nil {
		g.header = &Segment{Tag: "UNG"}
	}
	return g.header
}

// Header returns the UNH segment, creating an empty one if there is none. A nil
// m gives nil.
func (m *Message) Header() *Segment {
	if m == nil {
		return nil
	}
	if m.header == nil {
		m.header = &Segment{Tag: "UNH"}
	}
	return m.header
}

// FunctionalGroup returns the message's functional group, or nil when it is not
// in one or m is nil.
func (m *Message) FunctionalGroup() *FunctionalGroup {
	if m == nil {
		return nil
	}
	return m.group
}

// Segments returns the segments between UNH and UNT in write order. The slice
// is a copy, and the segments are the message's own.
func (m *Message) Segments() []*Segment {
	if m == nil {
		return nil
	}
	return append([]*Segment(nil), m.segments...)
}

// Groups returns the top-level segment groups, built by validation or AddGroup.
// The slice is a copy.
func (m *Message) Groups() []*SegmentGroup {
	if m == nil {
		return nil
	}
	return append([]*SegmentGroup(nil), m.groups...)
}

// Add appends a segment built from elements, one component each, and returns
// it.
func (m *Message) Add(tag string, elems ...string) *Segment {
	s := simpleSegment(tag, elems)
	m.segments = append(m.segments, s)
	return s
}

// AddComposite appends a segment whose elements each carry several components.
// The segment holds copies of elems.
func (m *Message) AddComposite(tag string, elems ...[]string) *Segment {
	s := compositeSegment(tag, elems)
	m.segments = append(m.segments, s)
	return s
}

func simpleSegment(tag string, elems []string) *Segment {
	s := &Segment{Tag: tag, elements: make([][][]string, len(elems))}
	for i, e := range elems {
		s.elements[i] = [][]string{{e}}
	}
	return s
}

func compositeSegment(tag string, elems [][]string) *Segment {
	s := &Segment{Tag: tag, elements: make([][][]string, len(elems))}
	for i, e := range elems {
		if len(e) > 0 {
			s.elements[i] = [][]string{append([]string(nil), e...)}
		}
	}
	return s
}

// AddGroup appends a segment group. name is the label used by the published
// tables, such as "SG2".
func (m *Message) AddGroup(name string) *SegmentGroup {
	g := &SegmentGroup{Name: name, msg: m}
	m.groups = append(m.groups, g)
	return g
}

// Find returns the first segment with the given tag whose element 1 matches
// qualifier, or nil. An empty qualifier matches any. A nil m gives nil.
func (m *Message) Find(tag, qualifier string) *Segment {
	if m == nil {
		return nil
	}
	return find(m.segments, tag, qualifier)
}

// FindAll returns every segment matching tag and qualifier.
func (m *Message) FindAll(tag, qualifier string) []*Segment {
	if m == nil {
		return nil
	}
	return findAll(m.segments, tag, qualifier)
}

// FindGroup returns the first repetition of the named top-level segment group,
// or nil.
func (m *Message) FindGroup(name string) *SegmentGroup {
	return first(m.FindAllGroups(name))
}

// FindAllGroups returns every repetition of the named top-level segment group.
func (m *Message) FindAllGroups(name string) []*SegmentGroup {
	if m == nil {
		return nil
	}
	return selectGroups(m.groups, name)
}

// Segments returns the segment group's segments in message order. The slice is
// a copy, and the segments are the message's own.
func (g *SegmentGroup) Segments() []*Segment {
	if g == nil {
		return nil
	}
	return append([]*Segment(nil), g.segments...)
}

// Groups returns the nested segment groups. The slice is a copy.
func (g *SegmentGroup) Groups() []*SegmentGroup {
	if g == nil {
		return nil
	}
	return append([]*SegmentGroup(nil), g.groups...)
}

// Add builds a segment from elements, one component each, places it after the
// segment group's last segment, both in the segment group and in the message,
// and returns it. A segment group with no segments yet places it after the
// previous repetition of the same segment group, or else after the nearest
// earlier segment group beside it, or else before the nearest later one, or
// else after its container's own segments.
func (g *SegmentGroup) Add(tag string, elems ...string) *Segment {
	return g.insert(simpleSegment(tag, elems))
}

// AddComposite is Add for a segment whose elements each carry several
// components. The segment holds copies of elems.
func (g *SegmentGroup) AddComposite(tag string, elems ...[]string) *Segment {
	return g.insert(compositeSegment(tag, elems))
}

// insert places s at g's anchor in the message, g and every enclosing segment
// group. The message gets a new backing array, so views of other segment groups
// keep the segments they held.
func (g *SegmentGroup) insert(s *Segment) *Segment {
	after, before := g.anchor()
	place := func(segs []*Segment) []*Segment {
		if before != nil && contains(segs, before) {
			return insertAt(segs, index(segs, before), s)
		}
		i := len(segs)
		if after != nil && contains(segs, after) {
			i = index(segs, after) + 1
		}
		return insertAt(segs, i, s)
	}
	if g.msg != nil {
		g.msg.segments = place(g.msg.segments)
	}
	for a := g; a != nil; a = a.parent {
		a.segments = place(a.segments)
	}
	return s
}

// anchor returns where a new segment of g goes: after the segment after, or
// before the segment before. Both nil means the end of the message.
func (g *SegmentGroup) anchor() (after, before *Segment) {
	if n := len(g.segments); n > 0 {
		return g.segments[n-1], nil
	}

	var own []*Segment
	var siblings []*SegmentGroup
	switch {
	case g.parent != nil:
		own, siblings = g.parent.segments, g.parent.groups
	case g.msg != nil:
		own, siblings = g.msg.segments, g.msg.groups
	default:
		return nil, nil
	}

	i := len(siblings)
	for j, sib := range siblings {
		if sib == g {
			i = j
			break
		}
	}
	for _, sameName := range []bool{true, false} {
		for j := i - 1; j >= 0; j-- {
			sib := siblings[j]
			if n := len(sib.segments); n > 0 && (!sameName || sib.Name == g.Name) {
				return sib.segments[n-1], nil
			}
		}
	}
	for _, sib := range siblings[min(i+1, len(siblings)):] {
		if len(sib.segments) > 0 {
			return nil, sib.segments[0]
		}
	}

	nested := map[*Segment]bool{}
	for _, sib := range siblings {
		for _, seg := range sib.segments {
			nested[seg] = true
		}
	}
	for j := len(own) - 1; j >= 0; j-- {
		if !nested[own[j]] {
			return own[j], nil
		}
	}
	if g.parent != nil {
		return g.parent.anchor()
	}
	return nil, nil
}

// insertAt returns a new slice holding segs with s at position i.
func insertAt(segs []*Segment, i int, s *Segment) []*Segment {
	out := make([]*Segment, 0, len(segs)+1)
	out = append(out, segs[:i]...)
	out = append(out, s)
	return append(out, segs[i:]...)
}

// index returns the position of the last occurrence of s in segs, or -1.
func index(segs []*Segment, s *Segment) int {
	for j := len(segs) - 1; j >= 0; j-- {
		if segs[j] == s {
			return j
		}
	}
	return -1
}

func contains(segs []*Segment, s *Segment) bool { return index(segs, s) >= 0 }

// adopt points each segment group in groups, and those nested within, at m and
// at its enclosing segment group.
func adopt(groups []*SegmentGroup, m *Message, parent *SegmentGroup) {
	for _, g := range groups {
		g.msg, g.parent = m, parent
		adopt(g.groups, m, g)
	}
}

// AddGroup appends a nested segment group.
func (g *SegmentGroup) AddGroup(name string) *SegmentGroup {
	n := &SegmentGroup{Name: name, msg: g.msg, parent: g}
	g.groups = append(g.groups, n)
	return n
}

// Find returns the first segment in the segment group matching tag and
// qualifier.
func (g *SegmentGroup) Find(tag, qualifier string) *Segment {
	if g == nil {
		return nil
	}
	return find(g.segments, tag, qualifier)
}

// FindAll returns every segment in the segment group matching tag and
// qualifier.
func (g *SegmentGroup) FindAll(tag, qualifier string) []*Segment {
	if g == nil {
		return nil
	}
	return findAll(g.segments, tag, qualifier)
}

// FindGroup returns the first repetition of the named nested segment group, or
// nil.
func (g *SegmentGroup) FindGroup(name string) *SegmentGroup {
	return first(g.FindAllGroups(name))
}

// FindAllGroups returns every repetition of the named nested segment group.
func (g *SegmentGroup) FindAllGroups(name string) []*SegmentGroup {
	if g == nil {
		return nil
	}
	return selectGroups(g.groups, name)
}

// Index returns a copy of the explicit nesting and repetition indices after the
// segment code, or nil when the tag carries none.
func (s *Segment) Index() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.index...)
}

// SetIndex replaces the explicit indices written after the segment code. No
// arguments removes them.
func (s *Segment) SetIndex(index ...string) {
	s.index = append([]string(nil), index...)
}

// empty reports whether s carries no data. Spaces count as no data, since
// trailing spaces are not written.
func (s *Segment) empty() bool {
	for _, e := range s.elements {
		for _, r := range e {
			for _, c := range r {
				if strings.TrimRight(c, " ") != "" {
					return false
				}
			}
		}
	}
	return true
}

// Len returns the number of elements.
func (s *Segment) Len() int {
	if s == nil {
		return 0
	}
	return len(s.elements)
}

// Repetitions returns the number of repetitions of element i, counting from 1,
// or 0 when the element is empty or absent.
func (s *Segment) Repetitions(i int) int {
	if s == nil || i < 1 || i > len(s.elements) {
		return 0
	}
	return len(s.elements[i-1])
}

// Repetition returns a copy of the components of repetition r of element i,
// both counting from 1, or nil when absent.
func (s *Segment) Repetition(i, r int) []string {
	return append([]string(nil), s.repetition(i, r)...)
}

// Components returns a copy of the components of the first repetition of
// element i, counting from 1, or nil when absent.
func (s *Segment) Components(i int) []string { return s.Repetition(i, 1) }

// Element returns component 1 of the first repetition of element i, counting
// from 1 as the published tables do, or "" when absent.
func (s *Segment) Element(i int) string { return s.Component(i, 1) }

// Component returns component j of the first repetition of element i, both
// counting from 1, or "" when absent.
func (s *Segment) Component(i, j int) string {
	c := s.repetition(i, 1)
	if j < 1 || j > len(c) {
		return ""
	}
	return c[j-1]
}

func (s *Segment) repetition(i, r int) []string {
	if s == nil || i < 1 || i > len(s.elements) {
		return nil
	}
	e := s.elements[i-1]
	if r < 1 || r > len(e) {
		return nil
	}
	return e[r-1]
}

// Qualifier returns element 1, the value that distinguishes segments sharing a
// tag.
func (s *Segment) Qualifier() string { return s.Element(1) }

// Set writes component j of the first repetition of element i, both counting
// from 1, extending the segment as needed.
func (s *Segment) Set(i, j int, v string) { s.SetRepetition(i, 1, j, v) }

// SetRepetition writes component j of repetition r of element i, all counting
// from 1, extending the segment as needed.
func (s *Segment) SetRepetition(i, r, j int, v string) {
	if i < 1 || r < 1 || j < 1 {
		return
	}
	if r == 1 {
		delete(s.floats, [2]int{i, j})
	}
	for len(s.elements) < i {
		s.elements = append(s.elements, nil)
	}
	e := s.elements[i-1]
	for len(e) < r {
		e = append(e, nil)
	}
	c := e[r-1]
	for len(c) < j {
		c = append(c, "")
	}
	c[j-1] = v
	e[r-1] = c
	s.elements[i-1] = e
}

func find(segs []*Segment, tag, qualifier string) *Segment {
	for _, s := range segs {
		if s.Tag == tag && (qualifier == "" || s.Qualifier() == qualifier) {
			return s
		}
	}
	return nil
}

func findAll(segs []*Segment, tag, qualifier string) []*Segment {
	var out []*Segment
	for _, s := range segs {
		if s.Tag == tag && (qualifier == "" || s.Qualifier() == qualifier) {
			out = append(out, s)
		}
	}
	return out
}

func first(groups []*SegmentGroup) *SegmentGroup {
	if len(groups) == 0 {
		return nil
	}
	return groups[0]
}

func selectGroups(groups []*SegmentGroup, name string) []*SegmentGroup {
	var out []*SegmentGroup
	for _, g := range groups {
		if g.Name == name {
			out = append(out, g)
		}
	}
	return out
}
