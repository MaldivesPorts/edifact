package edifact

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// ErrUnterminated reports trailing content with no segment terminator.
var ErrUnterminated = errors.New("edifact: segment is not terminated")

// ErrSegmentTooLong reports a segment longer than [MaxSegmentSize] allows.
var ErrSegmentTooLong = errors.New("edifact: segment is too long")

// defaultMaxSegmentSize is the longest segment Read accepts without
// MaxSegmentSize, in bytes, not counting its terminator.
const defaultMaxSegmentSize = 16 << 20

// scanner yields one segment at a time, with whitespace between segments
// discarded. Delimiters must already be known, including any declared by a
// leading UNA.
type scanner struct {
	sc  *bufio.Scanner
	d   Delimiters
	max int
}

// newScanner returns a scanner accepting segments of up to max bytes, not
// counting the terminator.
func newScanner(r io.Reader, d Delimiters, max int) *scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, min(64*1024, max+1)), max+1)
	sc.Split(splitSegments(d))
	return &scanner{sc: sc, d: d, max: max}
}

// next returns the raw bytes of the next segment, without its terminator.
func (s *scanner) next() ([]byte, bool) {
	for s.sc.Scan() {
		if b := s.sc.Bytes(); len(b) > 0 {
			return b, true
		}
	}
	return nil, false
}

func (s *scanner) err() error {
	err := s.sc.Err()
	if errors.Is(err, bufio.ErrTooLong) {
		return fmt.Errorf("%w: over %d bytes", ErrSegmentTooLong, s.max)
	}
	return err
}

func splitSegments(d Delimiters) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (int, []byte, error) {
		// Whitespace before a segment is consumed separately and does not
		// count towards its length. At the end of input it is skipped in the
		// same call as the segment. Trailing spaces belong to the last value
		// and are kept.
		lead := len(data) - len(bytes.TrimLeftFunc(data, unicode.IsSpace))
		if lead > 0 && !atEOF {
			return lead, nil, nil
		}
		for i := lead; i < len(data); i++ {
			if data[i] == d.Segment && !isReleased(data, i, d.Release) {
				return i + 1, data[lead:i], nil
			}
		}
		if atEOF {
			if len(trimFiller(data)) > 0 {
				return len(data), nil, ErrUnterminated
			}
			return len(data), nil, nil
		}
		return 0, nil, nil
	}
}

// isReleased reports whether the character at i is preceded by an odd number of
// release characters, and so stands for itself rather than its meta meaning. A
// zero release means there is no release character.
func isReleased(b []byte, i int, release byte) bool {
	if release == 0 {
		return false
	}
	n := 0
	for j := i - 1; j >= 0 && b[j] == release; j-- {
		n++
	}
	return n%2 == 1
}

// split divides b on sep, ignoring released separators. Released characters are
// left in place.
func split(b []byte, sep, release byte) []string {
	if len(b) == 0 {
		return nil
	}
	var out []string
	start := 0
	for i := range b {
		if b[i] == sep && !isReleased(b, i, release) {
			out = append(out, string(b[start:i]))
			start = i + 1
		}
	}
	return append(out, string(b[start:]))
}

// unescape removes release characters, yielding the value as it was meant.
func unescape(v string, d Delimiters) string {
	if d.Release == 0 || !strings.ContainsRune(v, rune(d.Release)) {
		return v
	}
	var sb strings.Builder
	sb.Grow(len(v))
	for i := 0; i < len(v); i++ {
		if v[i] == d.Release && i+1 < len(v) {
			i++
			sb.WriteByte(v[i])
			continue
		}
		if v[i] == d.Release {
			continue
		}
		sb.WriteByte(v[i])
	}
	return sb.String()
}

// escape inserts release characters before every meta character in v.
func escape(v string, d Delimiters) string {
	var sb strings.Builder
	sb.Grow(len(v))
	for i := 0; i < len(v); i++ {
		if d.isMeta(v[i]) {
			sb.WriteByte(d.Release)
		}
		sb.WriteByte(v[i])
	}
	return sb.String()
}

// isFiller reports whether r is whitespace, a control character such as an STX
// or ETX around a payload, or a byte order mark.
func isFiller(r rune) bool { return r <= ' ' || r == 0x7f || r == '\uFEFF' }

// trimFiller strips filler from both ends of b.
func trimFiller(b []byte) []byte { return bytes.TrimFunc(b, isFiller) }

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// skipLeadingFiller discards filler and UTF-8 byte order marks from the start
// of br.
func skipLeadingFiller(br *bufio.Reader) error {
	for {
		if b, _ := br.Peek(len(utf8BOM)); bytes.Equal(b, utf8BOM) {
			if _, err := br.Discard(len(utf8BOM)); err != nil {
				return err
			}
			continue
		}
		b, err := br.Peek(1)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !isFiller(rune(b[0])) {
			return nil
		}
		if _, err := br.Discard(1); err != nil {
			return err
		}
	}
}
