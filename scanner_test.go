package edifact

import (
	"reflect"
	"strings"
	"testing"
)

func TestScannerSegments(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"single", "BGM++REF+9'", []string{"BGM++REF+9"}},
		{"several", "BGM++REF+9'DTM+137:2601011200:203'", []string{"BGM++REF+9", "DTM+137:2601011200:203"}},
		{"newline separated", "BGM++REF+9'\nDTM+137:260101:102'\n", []string{"BGM++REF+9", "DTM+137:260101:102"}},
		{"crlf separated", "BGM++REF+9'\r\nDTM+137:260101:102'\r\n", []string{"BGM++REF+9", "DTM+137:260101:102"}},
		{"released terminator", "FTX+AAA+++DON?'T SPLIT'BGM++R+9'", []string{"FTX+AAA+++DON?'T SPLIT", "BGM++R+9"}},
		{"doubled release", "FTX+AAA+++ENDS??'BGM++R+9'", []string{"FTX+AAA+++ENDS??", "BGM++R+9"}},
		{"empty input", "", nil},
		{"whitespace only", "\n\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sc := newScanner(strings.NewReader(c.in), StandardDelimiters(), defaultMaxSegmentSize)
			var got []string
			for {
				b, ok := sc.next()
				if !ok {
					break
				}
				got = append(got, string(b))
			}
			if err := sc.err(); err != nil {
				t.Fatalf("err() = %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestScannerUnterminated(t *testing.T) {
	sc := newScanner(strings.NewReader("BGM++REF+9'DTM+137"), StandardDelimiters(), defaultMaxSegmentSize)
	for {
		if _, ok := sc.next(); !ok {
			break
		}
	}
	if err := sc.err(); err != ErrUnterminated {
		t.Fatalf("err() = %v, want ErrUnterminated", err)
	}
}

func TestScannerCustomDelimiters(t *testing.T) {
	d := Delimiters{Component: '!', Element: '*', Decimal: ',', Release: '#', Segment: ';'}
	sc := newScanner(strings.NewReader("BGM**REF*9;DTM*137!260101!102;"), d, defaultMaxSegmentSize)
	var got []string
	for {
		b, ok := sc.next()
		if !ok {
			break
		}
		got = append(got, string(b))
	}
	want := []string{"BGM**REF*9", "DTM*137!260101!102"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSplit(t *testing.T) {
	d := StandardDelimiters()
	cases := []struct {
		name string
		in   string
		sep  byte
		want []string
	}{
		{"elements", "LOC+147+0010204", d.Element, []string{"LOC", "147", "0010204"}},
		{"empty elements", "EQD+CN++45G0", d.Element, []string{"EQD", "CN", "", "45G0"}},
		{"trailing empty", "BGM++REF+", d.Element, []string{"BGM", "", "REF", ""}},
		{"components", "0010204::5", d.Component, []string{"0010204", "", "5"}},
		{"released separator", "A?+B+C", d.Element, []string{"A?+B", "C"}},
		{"doubled release then separator", "A??+B", d.Element, []string{"A??", "B"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := split([]byte(c.in), c.sep, d.Release)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestEscapeUnescapeRoundTrip(t *testing.T) {
	d := StandardDelimiters()
	for _, v := range []string{
		"PLAIN",
		"HAS+PLUS",
		"HAS:COLON",
		"HAS'QUOTE",
		"HAS?QUERY",
		"ALL+:'?TOGETHER",
		"",
	} {
		esc := escape(v, d)
		if got := unescape(esc, d); got != v {
			t.Errorf("escape(%q) = %q, unescape -> %q", v, esc, got)
		}
	}
}

func TestEscapeOutput(t *testing.T) {
	d := StandardDelimiters()
	cases := map[string]string{
		"A+B":   "A?+B",
		"A:B":   "A?:B",
		"A'B":   "A?'B",
		"A?B":   "A??B",
		"PLAIN": "PLAIN",
	}
	for in, want := range cases {
		if got := escape(in, d); got != want {
			t.Errorf("escape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseUNA(t *testing.T) {
	d, present, err := ParseUNA([]byte("UNA:+.? 'BGM++R+9'"))
	if err != nil || !present {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if want := StandardDelimiters(); d != want {
		t.Errorf("got %+v, want %+v", d, want)
	}
	if _, present, _ := ParseUNA([]byte("UNB+UNOA:2+")); present {
		t.Error("reported a UNA where there is none")
	}
	if _, _, err := ParseUNA([]byte("UNA:+")); err != ErrShortUNA {
		t.Errorf("err = %v, want ErrShortUNA", err)
	}
}

func TestNoReleaseCharacter(t *testing.T) {
	d, _, err := ParseUNA([]byte("UNA:+.  '"))
	if err != nil || d.Release != 0 {
		t.Fatalf("Release = %q err = %v", d.Release, err)
	}
	if isReleased([]byte("A +B"), 2, 0) {
		t.Error("a zero release character released a separator")
	}
	if got := split([]byte("A :B"), ':', 0); len(got) != 2 {
		t.Errorf("split = %q", got)
	}
	if got := unescape("TWO WORDS", d); got != "TWO WORDS" {
		t.Errorf("unescape = %q", got)
	}
	if d.isMeta(0) {
		t.Error("a NUL byte counted as a separator")
	}
}
