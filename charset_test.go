package edifact

import (
	"errors"
	"strings"
	"testing"
)

// charsetInterchange returns a complete interchange in charset c and syntax
// version v, with one message whose FTX holds text.
func charsetInterchange(t *testing.T, c Charset, v int, text string) *Interchange {
	t.Helper()
	ix := completeInterchange()
	ix.SetCharset(c)
	if err := ix.SetSyntaxVersion(v); err != nil {
		t.Fatal(err)
	}
	if v == 4 {
		ix.SetDate("20260101")
	}
	completeMessage(ix, nil, "M1").Add("FTX", "AAA", "", "", text)
	return ix
}

func TestWriteChecksTheCharset(t *testing.T) {
	for _, c := range []struct {
		name    string
		charset Charset
		version int
		text    string
		want    string // the FTX as written, or "" for a refusal
	}{
		{"UNOA upper case and punctuation", UNOA, 2, "NO. 1 (A/B) = 10%", "FTX+AAA+++NO. 1 (A/B) = 10%'"},
		{"UNOA service characters released", UNOA, 2, "A+B:C'D?", "FTX+AAA+++A?+B?:C?'D??'"},
		{"UNOA lower case", UNOA, 2, "Handle", ""},
		{"UNOA at sign", UNOA, 2, "A@B", ""},
		{"UNOB lower case", UNOB, 2, "Handle with care", "FTX+AAA+++Handle with care'"},
		{"UNOB accented", UNOB, 2, "Café", ""},
		{"UNOC accented as Latin-1", UNOC, 3, "Café", "FTX+AAA+++Caf\xe9'"},
		{"UNOC euro sign", UNOC, 3, "10 €", ""},
		{"UNOW anything", UNOW, 4, "Café 10 € Москва", "FTX+AAA+++Café 10 € Москва'"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var sb strings.Builder
			err := Write(&sb, charsetInterchange(t, c.charset, c.version, c.text))
			if c.want == "" {
				if !errors.Is(err, ErrCharset) || sb.Len() != 0 {
					t.Fatalf("err = %v, wrote %d bytes, want ErrCharset and nothing written", err, sb.Len())
				}
				if !strings.Contains(err.Error(), "FTX element 4 component 1") {
					t.Errorf("err = %v, want the position named", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(sb.String(), c.want) {
				t.Errorf("output %q does not hold %q", sb.String(), c.want)
			}
		})
	}
}

func TestWriteChecksHeaderValues(t *testing.T) {
	ix := charsetInterchange(t, UNOA, 2, "TEXT")
	ix.SetSender("sender", "")
	err := Write(&strings.Builder{}, ix)
	if !errors.Is(err, ErrCharset) || !strings.Contains(err.Error(), "UNB element 2 component 1") {
		t.Errorf("err = %v, want ErrCharset at UNB element 2", err)
	}
}

func TestWriteRefusesInvalidUTF8(t *testing.T) {
	err := Write(&strings.Builder{}, charsetInterchange(t, UNOW, 4, "A\xffB"))
	if !errors.Is(err, ErrCharset) || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Errorf("err = %v, want ErrCharset for invalid UTF-8", err)
	}
}

func TestWriteRefusesUNOWBelowVersion4(t *testing.T) {
	err := Write(&strings.Builder{}, charsetInterchange(t, UNOW, 3, "TEXT"))
	if !errors.Is(err, ErrSyntaxVersion) {
		t.Errorf("err = %v, want ErrSyntaxVersion", err)
	}
}

func TestWritePassesUnsupportedCharsetsThrough(t *testing.T) {
	ix := charsetInterchange(t, UNOA, 2, "anything @ here")
	ix.Header().Set(1, 1, "UNOD")
	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "FTX+AAA+++anything @ here'") {
		t.Errorf("output %q", sb.String())
	}
}

func TestFitCharset(t *testing.T) {
	for _, c := range []struct {
		name    string
		charset Charset
		version int
		text    string
		want    string // the FTX as written, or "" for a refusal
	}{
		{"UNOA upper-cases", UNOA, 2, "Handle with care", "FTX+AAA+++HANDLE WITH CARE'"},
		{"UNOA maps accents then upper-cases", UNOA, 2, "Café Æsir Straße", "FTX+AAA+++CAFE AESIR STRASSE'"},
		{"UNOA still refuses what has no mapping", UNOA, 2, "10 €", ""},
		{"UNOB maps accents and keeps case", UNOB, 2, "Café Straße", "FTX+AAA+++Cafe Strasse'"},
		{"UNOC keeps Latin-1 as it is", UNOC, 3, "Café", "FTX+AAA+++Caf\xe9'"},
		{"UNOC still refuses outside Latin-1", UNOC, 3, "Łódź", ""},
		{"UNOW changes nothing", UNOW, 4, "Café", "FTX+AAA+++Café'"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var sb strings.Builder
			err := Write(&sb, charsetInterchange(t, c.charset, c.version, c.text), FitCharset())
			if c.want == "" {
				if !errors.Is(err, ErrCharset) || sb.Len() != 0 {
					t.Fatalf("err = %v, wrote %d bytes, want ErrCharset and nothing written", err, sb.Len())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(sb.String(), c.want) {
				t.Errorf("output %q does not hold %q", sb.String(), c.want)
			}
		})
	}
}

func TestFitCharsetCoversEveryLatin1Letter(t *testing.T) {
	for r := rune(0xc0); r <= 0xff; r++ {
		if r == 0xd7 || r == 0xf7 { // multiplication and division signs
			continue
		}
		if _, ok := latin1Base[r]; !ok {
			t.Errorf("%q (U+%04X) has no base letter", r, r)
		}
	}
	if len(latin1Base) != 62 {
		t.Errorf("%d entries, want the 62 letters", len(latin1Base))
	}
}

func TestFitCharsetAppliesToHeaders(t *testing.T) {
	ix := charsetInterchange(t, UNOA, 2, "TEXT")
	ix.SetSender("sender", "")
	var sb strings.Builder
	if err := Write(&sb, ix, FitCharset()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "UNB+UNOA:2+SENDER+") {
		t.Errorf("output %q", sb.String())
	}
	if ix.Sender() != "sender" {
		t.Errorf("Write changed the interchange: sender %q", ix.Sender())
	}
}

// charsetFile is an interchange in the given syntax identifier and version
// whose one message holds an FTX with text.
func charsetFile(id, version, date, text string) string {
	return "UNB+" + id + ":" + version + "+SENDER+RECIPIENT+" + date + ":1200+REF1'" +
		"UNH+M1+TSTMSG:D:95B:UN'BGM++REF1+9'FTX+AAA+++" + text + "'UNT+4+M1'UNZ+1+REF1'"
}

func TestReadReportsCharacters(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{"UNOA clean", charsetFile("UNOA", "2", "260101", "HANDLE WITH CARE"), ""},
		{"UNOA lower case", charsetFile("UNOA", "2", "260101", "Handle"),
			"warning: message 1 (M1), segment 3, FTX+AAA: element 4 component 1: 'a' (U+0061) is not in UNOA"},
		{"UNOA high byte", charsetFile("UNOA", "2", "260101", "CAF\xc9"),
			"warning: message 1 (M1), segment 3, FTX+AAA: element 4 component 1: byte 0xC9 is not in UNOA"},
		{"UNOB lower case", charsetFile("UNOB", "2", "260101", "Handle with care"), ""},
		{"UNOC control byte", charsetFile("UNOC", "3", "260101", "A\x01B"),
			"warning: message 1 (M1), segment 3, FTX+AAA: element 4 component 1: byte 0x01 is not in UNOC"},
		{"UNOW invalid UTF-8", charsetFile("UNOW", "4", "20260101", "A\xffB"),
			"warning: message 1 (M1), segment 3, FTX+AAA: element 4 component 1: value is not valid UTF-8"},
		{"UNB value", strings.Replace(charsetFile("UNOA", "2", "260101", "TEXT"), "SENDER", "sender", 1),
			"warning: segment 1, UNB: element 2 component 1: 's' (U+0073) is not in UNOA"},
		{"unsupported", charsetFile("UNOD", "2", "260101", "anything"),
			"warning: segment 1, UNB: syntax identifier UNOD is not supported, so its values are read as they are"},
	} {
		t.Run(c.name, func(t *testing.T) {
			ix, err := Read(strings.NewReader(c.src))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, d := range ix.Diagnostics {
				got = append(got, d.String())
			}
			if c.want == "" && len(got) != 0 || c.want != "" && (len(got) != 1 || got[0] != c.want) {
				t.Errorf("diagnostics = %q, want %q", got, c.want)
			}
		})
	}
}

func TestReadDecodesUNOC(t *testing.T) {
	ix, err := Read(strings.NewReader(charsetFile("UNOC", "3", "260101", "Caf\xe9 \xc6SIR")))
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Diagnostics) != 0 {
		t.Errorf("diagnostics = %v", ix.Diagnostics)
	}
	if got := ix.Message(1).Find("FTX", "AAA").Element(4); got != "Café ÆSIR" {
		t.Errorf("FTX = %q, want the Latin-1 bytes decoded", got)
	}

	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "FTX+AAA+++Caf\xe9 \xc6SIR'") {
		t.Errorf("written back as %q", sb.String())
	}
}

func TestReadReportsOncePerSegment(t *testing.T) {
	ix, err := Read(strings.NewReader(charsetFile("UNOA", "2", "260101", "lower:case")))
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Diagnostics) != 1 {
		t.Errorf("diagnostics = %v, want one for the segment", ix.Diagnostics)
	}
}
