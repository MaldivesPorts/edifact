package edifact

import (
	"fmt"
	"io"
	"strings"
)

// WriteOption adjusts how an interchange is serialised. See
// [SegmentSuffixLF], [SegmentSuffixCRLF], [AllowIncompleteHeaders] and
// [RejectWriteOn].
type WriteOption func(*writeConfig)

type writeConfig struct {
	suffix          string
	allowIncomplete bool
	rejectAt        Severity
}

// RejectWriteOn makes Write return the first diagnostic of severity s or
// above as an error. By default, Write rejects errors. A severity above Error
// counts as Error.
func RejectWriteOn(s Severity) WriteOption {
	return func(c *writeConfig) { c.rejectAt = min(s, Error) }
}

// AllowIncompleteHeaders writes a UNB, UNG or UNH that lacks elements ISO 9735
// makes mandatory, rather than returning [ErrIncompleteHeader].
func AllowIncompleteHeaders() WriteOption {
	return func(c *writeConfig) { c.allowIncomplete = true }
}

// SegmentSuffixLF writes a line feed after every segment terminator. Without a
// suffix option, segments run together.
func SegmentSuffixLF() WriteOption {
	return func(c *writeConfig) { c.suffix = "\n" }
}

// SegmentSuffixCRLF writes a carriage return and a line feed after every
// segment terminator. Without a suffix option, segments run together.
func SegmentSuffixCRLF() WriteOption {
	return func(c *writeConfig) { c.suffix = "\r\n" }
}

// Write serialises ix without validating it against any spec. ix is left as it
// was, including its messages' segment groups. To validate first, use a
// Registry's Write or WriteWithSpec.
//
// Write always emits a UNA declaring ix.Delimiters.
func Write(w io.Writer, ix *Interchange, opts ...WriteOption) error {
	return (*Registry)(nil).Write(w, ix, opts...)
}

// WriteWithSpec is Write with every message validated against s alone.
func WriteWithSpec(w io.Writer, ix *Interchange, s Spec, opts ...WriteOption) error {
	reg := NewRegistry()
	if err := reg.Register(s); err != nil {
		return err
	}
	return reg.Write(w, ix, opts...)
}

// Write validates ix against the specs in r and then serialises it, stopping
// at the first structural error, or the first warning under
// RejectWriteOn(Warning). A nil r skips validation.
func (r *Registry) Write(w io.Writer, ix *Interchange, opts ...WriteOption) error {
	cfg := writeConfig{rejectAt: Error}
	for _, o := range opts {
		o(&cfg)
	}

	var omit map[*Segment]bool
	if r != nil {
		var diags []Diagnostic
		diags, omit = r.check(ix, true)
		for _, d := range diags {
			if d.Severity >= cfg.rejectAt {
				return fmt.Errorf("edifact: %s", d)
			}
		}
	}

	if ix.header != nil && !cfg.allowIncomplete {
		if err := checkHeaders(ix); err != nil {
			return err
		}
	}

	segs, err := envelopeSegments(ix, omit)
	if err != nil {
		return err
	}

	d := ix.Delimiters
	if d == (Delimiters{}) {
		d = StandardDelimiters()
	}
	if d.Decimal != ',' {
		d.Decimal = '.'
	}
	switch {
	case ix.SyntaxVersion() != "4":
		d.Repetition = 0
	case d.Repetition == 0:
		d.Repetition = '*'
	}
	if err := d.validate(); err != nil {
		return err
	}

	// Every segment is rendered before anything is written, so a value that
	// cannot be represented leaves w untouched.
	rendered := make([]string, len(segs))
	for i, s := range segs {
		if rendered[i], err = formatSegment(s, d); err != nil {
			return err
		}
	}

	if err := d.writeUNA(w); err != nil {
		return err
	}
	if _, err := io.WriteString(w, cfg.suffix); err != nil {
		return err
	}
	for _, r := range rendered {
		if _, err := io.WriteString(w, r); err != nil {
			return err
		}
		if _, err := io.WriteString(w, cfg.suffix); err != nil {
			return err
		}
	}
	return nil
}

// formatSegment renders one segment, escaping values and dropping trailing
// empty components and elements, which carry no meaning.
func formatSegment(s *Segment, d Delimiters) (string, error) {
	elems, err := renderElements(s, d)
	if err != nil {
		return "", err
	}
	if d.unreleasable(s.Tag) {
		return "", fmt.Errorf("%w: tag %q", ErrUnreleasable, s.Tag)
	}
	for i, idx := range s.index {
		if d.unreleasable(idx) {
			return "", fmt.Errorf("%w: %s index %d", ErrUnreleasable, s.Tag, i+1)
		}
	}
	var sb strings.Builder
	sb.WriteString(escape(s.Tag, d))
	for _, idx := range trimEmpty(append([]string(nil), s.index...)) {
		sb.WriteByte(d.Component)
		sb.WriteString(escape(idx, d))
	}
	for _, e := range trimEmpty(elems) {
		sb.WriteByte(d.Element)
		sb.WriteString(e)
	}
	sb.WriteByte(d.Segment)
	return sb.String(), nil
}

func renderElements(s *Segment, d Delimiters) ([]string, error) {
	out := make([]string, len(s.elements))
	for i, e := range s.elements {
		reps := make([]string, len(e))
		for r, comps := range e {
			escaped := make([]string, len(comps))
			for j, c := range comps {
				// Trailing spaces are insignificant under ISO 9735.
				c = strings.TrimRight(c, " ")
				if r == 0 && d.Decimal == ',' && s.floats[[2]int{i + 1, j + 1}] {
					c = strings.Replace(c, ".", ",", 1)
				}
				if d.unreleasable(c) {
					return nil, fmt.Errorf("%w: %s element %d component %d", ErrUnreleasable, s.Tag, i+1, j+1)
				}
				escaped[j] = escape(c, d)
			}
			reps[r] = strings.Join(trimEmpty(escaped), string(d.Component))
		}
		reps = trimEmpty(reps)
		if len(reps) > 1 && d.Repetition == 0 {
			return nil, fmt.Errorf("%w: %s element %d", ErrNoRepetitionSeparator, s.Tag, i+1)
		}
		out[i] = strings.Join(reps, string(d.Repetition))
	}
	return out, nil
}

// trimEmpty drops trailing empty entries, leaving interior ones in place.
func trimEmpty(vs []string) []string {
	end := len(vs)
	for end > 0 && vs[end-1] == "" {
		end--
	}
	return vs[:end]
}
