package edifact

import "fmt"

// Validate checks every message in ix against its registered spec, and fills in
// each message's segment groups. A message whose Key resolves to no spec is
// reported and left ungrouped.
func (r *Registry) Validate(ix *Interchange) []Diagnostic {
	diags, _ := r.check(ix, false)
	return diags
}

// check validates ix, replacing each message's segment groups unless write is
// set. It also returns the segments that carry no data and match a segment
// node that is not required, which ISO 9735 omits on write.
func (r *Registry) check(ix *Interchange, write bool) ([]Diagnostic, map[*Segment]bool) {
	var diags []Diagnostic
	omit := map[*Segment]bool{}
	for i, m := range ix.messages {
		k := m.Key()
		var ds []Diagnostic
		var groups []*SegmentGroup
		if spec, ok := r.lookup(k); ok {
			var optional map[*Segment]bool
			groups, ds, optional = walkSegments(m.segments, spec)
			for s := range optional {
				if s.empty() {
					omit[s] = true
				}
			}
		} else {
			ds = []Diagnostic{{
				Severity:   Error,
				Tag:        "UNH",
				SegmentIdx: 1,
				Message:    fmt.Sprintf("no spec matches %s", k),
			}}
		}
		if !write {
			adopt(groups, m, nil)
			m.groups = groups
		}
		for j := range ds {
			ds[j].MessageIdx = i + 1
			ds[j].MessageRef = m.Ref()
		}
		diags = append(diags, ds...)
	}
	return diags, omit
}

// validateMessage walks a message's segments against a spec and replaces
// m.groups with the result.
func validateMessage(m *Message, s *Spec) []Diagnostic {
	groups, diags, _ := walkSegments(m.segments, s)
	adopt(groups, m, nil)
	m.groups = groups
	return diags
}

// walkSegments returns the segment groups a spec builds over a message's
// segments, what did not fit, and the segments matched to a segment node that
// is not required.
func walkSegments(segs []*Segment, s *Spec) ([]*SegmentGroup, []Diagnostic, map[*Segment]bool) {
	w := &walker{segs: segs, optional: map[*Segment]bool{}}
	groups, diags := w.walk(s.Nodes, "", nil)
	for w.cursor < len(w.segs) {
		diags = append(diags, w.unexpected(""))
	}
	return groups, diags, w.optional
}

// walker carries the cursor through one message's segments.
type walker struct {
	segs     []*Segment
	cursor   int
	optional map[*Segment]bool
}

// walk consumes segments against nodes, returning the segment groups it built.
//
// follow is what may come after nodes at every enclosing level. A segment that
// can start none of the remaining nodes and nothing in follow is reported and
// stepped over. One that can start something in follow is left for the
// enclosing level to take.
func (w *walker) walk(nodes []Node, path string, follow []Node) ([]*SegmentGroup, []Diagnostic) {
	var groups []*SegmentGroup
	var diags []Diagnostic

	for i, n := range nodes {
		for w.cursor < len(w.segs) && w.stray(nodes[i:], follow) {
			diags = append(diags, w.unexpected(path))
		}

		if n.IsSegment() {
			diags = append(diags, w.segment(n, path, nodes[i+1:], follow)...)
			continue
		}

		reps, d := w.group(n, nodes[i+1:], follow, path, &groups)
		diags = append(diags, d...)
		if reps == 0 && n.Required {
			diags = append(diags, missing(n, path, w.pos()))
		}
	}
	return groups, diags
}

// segment consumes the repetitions of one segment node. A match beyond Max that
// can start something in rest or follow is left alone, because inside a
// repeating segment group it starts the next repetition. Any other match beyond
// Max is reported as repeated too often and stepped over.
func (w *walker) segment(n Node, path string, rest, follow []Node) []Diagnostic {
	var diags []Diagnostic
	count := 0
	max := n.repeats()

	for w.cursor < len(w.segs) && matches(w.segs[w.cursor], n) {
		if max != Unbounded && count >= max {
			if w.stray(rest, follow) {
				diags = append(diags, w.tooMany(n, path))
				w.cursor++
				continue
			}
			break
		}
		if !n.Required {
			w.optional[w.segs[w.cursor]] = true
		}
		w.cursor++
		count++
	}

	if count == 0 && n.Required {
		diags = append(diags, missing(n, path, w.pos()))
	}
	return diags
}

// group consumes the repetitions of one segment group node, appending each to
// groups.
//
// rest is what may follow the segment group at this level, and follow what may
// come after that at enclosing levels. A segment that can start neither another
// repetition nor anything in rest or follow is reported and stepped over, so
// one stray segment costs a warning rather than ending the segment group and
// orphaning every repetition after it.
func (w *walker) group(n Node, rest, follow []Node, path string, groups *[]*SegmentGroup) (int, []Diagnostic) {
	var diags []Diagnostic
	reps := 0
	max := n.repeats()
	here := path + "/" + n.Group

	// Inside a repetition, what may come next is another repetition, then
	// whatever follows the segment group here, then whatever follows at
	// enclosing levels.
	inner := make([]Node, 0, 1+len(rest)+len(follow))
	inner = append(append(append(inner, n), rest...), follow...)

	for w.cursor < len(w.segs) {
		if !canStart(w.segs[w.cursor], n) {
			if !w.stray(rest, follow) {
				break
			}
			diags = append(diags, w.unexpected(path))
			continue
		}
		if max != Unbounded && reps == max {
			if !w.stray(rest, follow) {
				break
			}
			// The extra repetition is reported once, then walked and
			// discarded so its segments are not each reported.
			diags = append(diags, w.tooMany(n, path))
			start := w.cursor
			w.walk(n.Nodes, here, inner)
			if w.cursor == start {
				break
			}
			continue
		}

		start := w.cursor
		nested, d := w.walk(n.Nodes, here, inner)
		diags = append(diags, d...)

		if w.cursor == start {
			// The trigger matched but nothing was consumed, so another pass
			// would not move either.
			break
		}

		// The capacity is capped so that appending to a segment group allocates
		// rather than writing over the message's next segment.
		*groups = append(*groups, &SegmentGroup{
			Name:     n.Group,
			segments: w.segs[start:w.cursor:w.cursor],
			groups:   nested,
		})
		reps++
	}
	return reps, diags
}

// stray reports whether the segment at the cursor can start nothing in nodes
// and nothing in follow, so that no level of the walk could take it.
func (w *walker) stray(nodes, follow []Node) bool {
	s := w.segs[w.cursor]
	return !canStartAny(s, nodes) && !canStartAny(s, follow)
}

// tooMany reports a segment or segment group at the cursor that repeats more
// often than n allows.
func (w *walker) tooMany(n Node, path string) Diagnostic {
	s := w.segs[w.cursor]
	tag, qualifier := n.Tag, s.Qualifier()
	if !n.IsSegment() {
		tag, qualifier = n.Group, ""
	}
	times := fmt.Sprintf("%d times", n.repeats())
	if n.repeats() == 1 {
		times = "once"
	}
	return Diagnostic{
		Severity:   Warning,
		Path:       path,
		Tag:        tag,
		Qualifier:  qualifier,
		SegmentIdx: w.pos(),
		Message:    "repeated more than " + times,
	}
}

// pos returns the cursor's position in the message, counting UNH as 1.
func (w *walker) pos() int { return w.cursor + 2 }

// unexpected reports the segment at the cursor and steps past it.
func (w *walker) unexpected(path string) Diagnostic {
	s := w.segs[w.cursor]
	d := Diagnostic{
		Severity:   Warning,
		Path:       path,
		Tag:        s.Tag,
		Qualifier:  s.Qualifier(),
		SegmentIdx: w.pos(),
		Message:    "segment is not declared here",
	}
	w.cursor++
	return d
}

func missing(n Node, path string, idx int) Diagnostic {
	tag, what := n.Tag, "segment"
	if !n.IsSegment() {
		tag, what = n.Group, "segment group"
	}
	return Diagnostic{
		Severity:   Error,
		Path:       path,
		Tag:        tag,
		Qualifier:  n.qualifierText(),
		SegmentIdx: idx,
		Message:    "required " + what + " is missing",
	}
}

// matches reports whether seg satisfies a segment node.
func matches(seg *Segment, n Node) bool {
	return seg.Tag == n.Tag && n.accepts(seg.Qualifier())
}

// canStart reports whether seg could begin n, following a segment group down to
// the first segment that triggers it.
func canStart(seg *Segment, n Node) bool {
	if n.IsSegment() {
		return matches(seg, n)
	}
	for _, c := range n.Nodes {
		if canStart(seg, c) {
			return true
		}
		if c.Required {
			return false
		}
	}
	return false
}

// canStartAny reports whether seg could begin any of nodes.
func canStartAny(seg *Segment, nodes []Node) bool {
	for _, n := range nodes {
		if canStart(seg, n) {
			return true
		}
	}
	return false
}
