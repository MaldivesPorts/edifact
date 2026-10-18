package edifact

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ReadOption adjusts how an interchange is read. See [RejectReadOn],
// [MaxSegmentSize] and [DefaultDelimiters].
type ReadOption func(*readConfig)

type readConfig struct {
	reject     bool
	rejectAt   Severity
	maxSegment int
	delimiters *Delimiters
}

// RejectReadOn makes Read return the first diagnostic of severity s or above
// as an error. By default, Read rejects none.
func RejectReadOn(s Severity) ReadOption {
	return func(c *readConfig) { c.reject, c.rejectAt = true, s }
}

// MaxSegmentSize makes Read return ErrSegmentTooLong for a segment longer than
// n bytes, not counting its terminator or whitespace before it. By default,
// the limit is 16 MiB.
func MaxSegmentSize(n int) ReadOption {
	return func(c *readConfig) { c.maxSegment = n }
}

// DefaultDelimiters makes Read use d for an interchange with no UNA. A UNA
// still declares its own. By default, an interchange with no UNA is read with
// the level A separators.
func DefaultDelimiters(d Delimiters) ReadOption {
	return func(c *readConfig) { c.delimiters = &d }
}

// Read parses an interchange without consulting any spec, so the result
// carries the envelope's own diagnostics and nothing about message structure.
// The error covers syntax alone. Anything structural is collected onto the
// returned interchange's Diagnostics, unless [RejectReadOn] turns it into an
// error. To check messages against specs, use [Registry.Read] or
// [ReadWithSpec].
func Read(r io.Reader, opts ...ReadOption) (*Interchange, error) {
	return (*Registry)(nil).Read(r, opts...)
}

// ReadWithSpec parses an interchange, checking each message against s alone. A
// message whose UNH key s does not match is reported and left ungrouped.
func ReadWithSpec(r io.Reader, s Spec, opts ...ReadOption) (*Interchange, error) {
	reg := NewRegistry()
	if err := reg.Register(s); err != nil {
		return nil, err
	}
	return reg.Read(r, opts...)
}

// Read parses an interchange, checking each message against the specs in r. A
// nil r consults no spec.
func (r *Registry) Read(src io.Reader, opts ...ReadOption) (*Interchange, error) {
	cfg := readConfig{maxSegment: defaultMaxSegmentSize}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.maxSegment < 1 {
		return nil, fmt.Errorf("edifact: MaxSegmentSize %d is not positive", cfg.maxSegment)
	}
	if cfg.delimiters != nil {
		if err := cfg.delimiters.validate(); err != nil {
			return nil, err
		}
	}

	br := bufio.NewReader(src)
	if err := skipLeadingFiller(br); err != nil {
		return nil, err
	}

	head, _ := br.Peek(unaLen)
	d, present, err := ParseUNA(head)
	if err != nil {
		return nil, err
	}
	switch {
	case present:
		if _, err := br.Discard(unaLen); err != nil {
			return nil, err
		}
	case cfg.delimiters != nil:
		d = *cfg.delimiters
	default:
		if err := checkDefaultSeparators(head); err != nil {
			return nil, err
		}
	}
	// The UNB is parsed with no repetition separator. The syntax version it
	// names sets the separator for every segment after it.
	declared, isDeclared := d.Repetition, present || d.Repetition != 0
	d.Repetition = 0

	sc := newScanner(br, d, cfg.maxSegment)
	a := newAssembler()
	first := true
	var cs Charset
	for {
		raw, ok := sc.next()
		if !ok {
			break
		}
		s := parseSegment(raw, d)
		if first && s.Tag == "UNB" {
			d.Repetition = repetitionFor(s.Component(1, 2), declared, isDeclared)
			cs = a.charsetFrom(s)
		}
		first = false
		at := a.locate(s)
		if problem := cs.decodeValues(s); problem != "" {
			at.Severity, at.Tag, at.Message = Warning, s.Tag, problem
			if !strings.HasPrefix(s.Tag, "UN") {
				at.Qualifier = s.Qualifier()
			}
			a.diags = append(a.diags, at)
		}
		if err := a.add(s); err != nil {
			return nil, err
		}
	}
	if err := sc.err(); err != nil {
		return nil, err
	}

	ix, diags, err := a.done()
	if err != nil {
		return nil, err
	}
	ix.Delimiters = d

	if r != nil {
		diags = append(diags, r.Validate(ix)...)
		ix.Diagnostics = diags
	}
	if cfg.reject {
		for _, d := range ix.Diagnostics {
			if d.Severity >= cfg.rejectAt {
				return nil, fmt.Errorf("edifact: %s", d)
			}
		}
	}
	return ix, nil
}

// repetitionFor returns the repetition separator in force for a syntax
// version. Only version 4 has one, and it defaults to an asterisk when neither
// a UNA nor DefaultDelimiters declares it.
func repetitionFor(version string, declared byte, isDeclared bool) byte {
	switch {
	case version != "4":
		return 0
	case isDeclared:
		return declared
	default:
		return '*'
	}
}

// ErrUnsupportedSeparators reports an interchange with no UNA whose UNB uses
// the information separators IS3 and IS1, the defaults of syntax level B.
// Without a UNA, Read uses the level A separators unless [DefaultDelimiters]
// names others.
var ErrUnsupportedSeparators = errors.New("edifact: default separators other than level A need a UNA or DefaultDelimiters")

// checkDefaultSeparators reports an interchange that opens with UNB followed by
// IS3, naming its syntax identifier.
func checkDefaultSeparators(head []byte) error {
	const is1, is3 = 0x1F, 0x1D
	if len(head) < 4 || string(head[:3]) != "UNB" || head[3] != is3 {
		return nil
	}
	id := head[4:]
	if i := bytes.IndexAny(id, string([]byte{is1, is3})); i >= 0 {
		id = id[:i]
	}
	return fmt.Errorf("%w: syntax identifier %s", ErrUnsupportedSeparators, id)
}

// parseSegment turns one segment's bytes into its tag, explicit indices,
// elements, repetitions and components, with release characters and trailing
// spaces removed. Trailing empty repetitions are dropped.
func parseSegment(b []byte, d Delimiters) *Segment {
	parts := split(b, d.Element, d.Release)
	if len(parts) == 0 {
		return &Segment{}
	}

	s := &Segment{}
	if tag := split([]byte(parts[0]), d.Component, d.Release); len(tag) > 0 {
		s.Tag = unescape(tag[0], d)
		for _, idx := range tag[1:] {
			s.index = append(s.index, unescape(idx, d))
		}
	}
	if len(parts) == 1 {
		return s
	}

	s.elements = make([][][]string, len(parts)-1)
	for i, p := range parts[1:] {
		reps := []string{p}
		if d.Repetition != 0 {
			reps = split([]byte(p), d.Repetition, d.Release)
		}
		var e [][]string
		for _, r := range reps {
			comps := split([]byte(r), d.Component, d.Release)
			for j := range comps {
				comps[j] = strings.TrimRight(unescape(comps[j], d), " ")
			}
			e = append(e, comps)
		}
		for len(e) > 0 && len(e[len(e)-1]) == 0 {
			e = e[:len(e)-1]
		}
		s.elements[i] = e
	}
	return s
}
