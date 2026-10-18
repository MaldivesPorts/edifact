package edifact

import (
	"errors"
	"io"
	"strings"
	"testing"
)

const sample = "UNB+UNOA:2+SENDER+RECIP+260921:1030+CTRL1'" +
	"UNH+MSG1+TSTMSG:D:95B:UN:AAA01'" +
	"BGM++REF001+9'" +
	"DTM+137:2609211030:203'" +
	"LOC+147+0010204::5'" +
	"EQD+CN+AAAU0000001+45G0+++5'" +
	"UNT+6+MSG1'" +
	"UNZ+1+CTRL1'"

func TestReadSample(t *testing.T) {
	ix, err := Read(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %v", ix.Diagnostics)
	}
	if ix.Sender() != "SENDER" || ix.Recipient() != "RECIP" {
		t.Errorf("parties = %q %q", ix.Sender(), ix.Recipient())
	}
	if ix.Date() != "260921" || ix.Time() != "1030" {
		t.Errorf("prepared = %q %q", ix.Date(), ix.Time())
	}

	m := ix.Messages()[0]
	if got := *m.Key().Type; got != "TSTMSG" {
		t.Errorf("Key().Type = %q", got)
	}
	if got := len(m.Segments()); got != 4 {
		t.Fatalf("got %d segments, want 4", got)
	}
	loc := m.Find("LOC", "147")
	if got := loc.Component(2, 1); got != "0010204" {
		t.Errorf("component 1 = %q", got)
	}
	if got := loc.Component(2, 2); got != "" {
		t.Errorf("empty middle component = %q", got)
	}
	if got := loc.Component(2, 3); got != "5" {
		t.Errorf("component 3 = %q", got)
	}
	if got := m.Find("EQD", "CN").Element(2); got != "AAAU0000001" {
		t.Errorf("EQD = %q", got)
	}
}

func TestReadWhitespaceAndUNA(t *testing.T) {
	spaced := strings.ReplaceAll(sample, "'", "'\n")
	a, err := Read(strings.NewReader(spaced))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Read(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Messages()[0].Segments()) != len(b.Messages()[0].Segments()) {
		t.Error("newlines between segments changed the result")
	}

	withUNA, err := Read(strings.NewReader("UNA:+.? '" + sample))
	if err != nil {
		t.Fatal(err)
	}
	if withUNA.Delimiters != StandardDelimiters() {
		t.Errorf("delimiters = %+v", withUNA.Delimiters)
	}
	if len(withUNA.Messages()) != 1 {
		t.Errorf("UNA was not skipped: %d messages", len(withUNA.Messages()))
	}
}

func TestReadCustomDelimiters(t *testing.T) {
	src := "UNA!*,#_;" +
		"UNB*UNOA!2*SENDER*RECIP*260921!1030*CTRL1;" +
		"UNH*MSG1*TSTMSG!D!95B!UN!AAA01;" +
		"BGM**REF001*9;" +
		"UNT*3*MSG1;" +
		"UNZ*1*CTRL1;"

	ix, err := Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %v", ix.Diagnostics)
	}
	if ix.Sender() != "SENDER" {
		t.Errorf("Sender() = %q", ix.Sender())
	}
	if got := *ix.Messages()[0].Key().Release; got != "95B" {
		t.Errorf("Release = %q", got)
	}
}

func TestReadEscapes(t *testing.T) {
	src := "UNB+UNOA:2+S+R+260921:1030+CTRL1'" +
		"UNH+MSG1+TSTMSG:D:95B:UN:AAA01'" +
		"FTX+AAA+++DON?'T ?+SPLIT?: OK'" +
		"UNT+3+MSG1'UNZ+1+CTRL1'"

	ix, err := Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	got := ix.Messages()[0].Find("FTX", "AAA").Element(4)
	if want := "DON'T +SPLIT: OK"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriteThenRead(t *testing.T) {
	ix := NewInterchange()
	ix.SetSender("TERMINAL", "")
	ix.SetRecipient("PARTNER", "")
	ix.SetDate("260921")
	ix.SetTime("1030")
	ix.SetControlRef("CTRL1")

	m := ix.AddMessage(Key{Type: Is("TSTMSG"), Version: Is("D"), Release: Is("95B"), Agency: Is("UN"), Association: Is("AAA01")})
	m.SetRef("MSG1")
	m.Add("BGM", "", "REF001", "9")
	m.AddComposite("LOC", []string{"147"}, []string{"0010204", "", "5"})
	m.Add("FTX", "AAA", "", "", "HAS+PLUS AND 'QUOTE")

	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.HasPrefix(out, "UNA:+.? '") {
		t.Errorf("no UNA: %.20q", out)
	}

	back, err := Read(strings.NewReader(out))
	if err != nil {
		t.Fatalf("re-reading what we wrote: %v\n%s", err, out)
	}
	if len(back.Diagnostics) != 0 {
		t.Fatalf("diagnostics on re-read: %v", back.Diagnostics)
	}

	bm := back.Messages()[0]
	if got := bm.Find("FTX", "AAA").Element(4); got != "HAS+PLUS AND 'QUOTE" {
		t.Errorf("escaped value came back as %q", got)
	}
	if got := bm.Find("LOC", "147").Component(2, 3); got != "5" {
		t.Errorf("LOC component 3 = %q", got)
	}
	if back.Sender() != "TERMINAL" || back.ControlRef() != "CTRL1" {
		t.Errorf("UNB = %q %q", back.Sender(), back.ControlRef())
	}
}

func TestWriteTrimsTrailingEmpties(t *testing.T) {
	ix := completeInterchange()
	ix.SetControlRef("C1")
	m := ix.AddMessage(testKey())
	m.SetRef("M1")
	s := m.Add("BGM", "", "REF001", "", "")
	s.Set(6, 3, "") // reach out to element 6 component 3, all empty

	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "BGM++REF001'") {
		t.Errorf("trailing empties survived: %q", sb.String())
	}
}

func TestWriteSegmentSuffix(t *testing.T) {
	ix := completeInterchange()
	ix.SetControlRef("C1")
	m := ix.AddMessage(testKey())
	m.SetRef("M1")
	m.Add("BGM", "", "REF1")

	var plain, lf, crlf strings.Builder
	for _, w := range []struct {
		sb   *strings.Builder
		opts []WriteOption
	}{{&plain, nil}, {&lf, []WriteOption{SegmentSuffixLF()}}, {&crlf, []WriteOption{SegmentSuffixCRLF()}}} {
		if err := Write(w.sb, ix, w.opts...); err != nil {
			t.Fatal(err)
		}
	}
	if strings.ContainsAny(plain.String(), "\r\n") {
		t.Error("default output contains line breaks")
	}
	// UNA, UNB, UNH, BGM, UNT, UNZ.
	if n := strings.Count(lf.String(), "\n"); n != 6 || strings.Contains(lf.String(), "\r") {
		t.Errorf("LF output: %q", lf.String())
	}
	if n := strings.Count(crlf.String(), "\r\n"); n != 6 {
		t.Errorf("CRLF output: %q", crlf.String())
	}
	for _, out := range []string{lf.String(), crlf.String()} {
		if _, err := Read(strings.NewReader(out)); err != nil {
			t.Errorf("output does not read back: %v", err)
		}
	}
}

func TestWriteRefusesOnStructuralError(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(containerSpec()); err != nil {
		t.Fatal(err)
	}
	ix := NewInterchange()
	ix.SetControlRef("C1")
	m := ix.AddMessage(Key{Type: Is("TSTMSG")})
	m.SetRef("M1")
	m.Add("LOC", "147") // BGM and DTM are required and absent

	var sb strings.Builder
	err := r.Write(&sb, ix)
	if err == nil {
		t.Fatal("wrote a message missing required segments")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("err = %v", err)
	}
	if sb.Len() != 0 {
		t.Errorf("wrote %d bytes before refusing", sb.Len())
	}
}

func TestMaxSegmentSize(t *testing.T) {
	// The longest segment in sample is its 41-byte UNB.
	if _, err := Read(strings.NewReader(sample), MaxSegmentSize(41)); err != nil {
		t.Errorf("a limit at the longest segment refused it: %v", err)
	}
	_, err := Read(strings.NewReader(sample), MaxSegmentSize(40))
	if !errors.Is(err, ErrSegmentTooLong) || err.Error() != "edifact: segment is too long: over 40 bytes" {
		t.Errorf("err = %v, want ErrSegmentTooLong over 40 bytes", err)
	}
	lines := strings.ReplaceAll(sample, "'", "'\r\n")
	if _, err := Read(strings.NewReader(lines), MaxSegmentSize(41)); err != nil {
		t.Errorf("line breaks between segments counted towards the limit: %v", err)
	}
	if _, err := Read(strings.NewReader(sample), MaxSegmentSize(0)); err == nil {
		t.Error("a zero limit was accepted")
	}
}

func TestDefaultDelimiters(t *testing.T) {
	// The syntax level B separators: IS1 between components, IS3 between
	// elements and IS4 after each segment, with no release character.
	const is1, is3, is4 = "\x1f", "\x1d", "\x1c"
	levelB := strings.NewReplacer(":", is1, "+", is3, "'", is4).Replace(
		"UNB+UNOB:2+SENDER+RECIP+260921:1030+CTRL1'" +
			"UNH+MSG1+TSTMSG:D:95B:UN'BGM++REF001+9'UNT+3+MSG1'UNZ+1+CTRL1'")
	b := Delimiters{Component: 0x1f, Element: 0x1d, Segment: 0x1c}

	if _, err := Read(strings.NewReader(levelB)); !errors.Is(err, ErrUnsupportedSeparators) {
		t.Errorf("without the option: err = %v, want ErrUnsupportedSeparators", err)
	}

	ix, err := Read(strings.NewReader(levelB), DefaultDelimiters(b))
	if err != nil {
		t.Fatal(err)
	}
	if got := ix.Message(1).Find("BGM", "").Element(2); got != "REF001" {
		t.Errorf("BGM element 2 = %q, want REF001", got)
	}

	// A UNA still declares its own delimiters.
	ix, err = Read(strings.NewReader("UNA:+.? '"+sample), DefaultDelimiters(b))
	if err != nil {
		t.Fatal(err)
	}
	if got := ix.Message(1).Find("BGM", "").Element(2); got != "REF001" {
		t.Errorf("with a UNA, BGM element 2 = %q, want REF001", got)
	}

	if _, err := Read(strings.NewReader(sample), DefaultDelimiters(Delimiters{})); !errors.Is(err, ErrInvalidDelimiters) {
		t.Errorf("empty delimiters: err = %v, want ErrInvalidDelimiters", err)
	}
}

func TestDefaultDelimitersRepetition(t *testing.T) {
	src := strings.Replace(sample, "UNOA:2", "UNOC:4", 1)
	src = strings.Replace(src, "BGM++REF001+9'", "BGM++REF001!REF002+9'", 1)
	d := StandardDelimiters()
	d.Repetition = '!'

	ix, err := Read(strings.NewReader(src), DefaultDelimiters(d))
	if err != nil {
		t.Fatal(err)
	}
	if got := ix.Message(1).Find("BGM", "").Repetition(2, 2); len(got) != 1 || got[0] != "REF002" {
		t.Errorf("second repetition = %q, want [REF002]", got)
	}
}

func TestRejectWriteOn(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(containerSpec()); err != nil {
		t.Fatal(err)
	}
	read := func(src string) *Interchange {
		t.Helper()
		ix, err := Read(strings.NewReader(src))
		if err != nil {
			t.Fatal(err)
		}
		return ix
	}
	// An FTX the spec does not declare is a warning. A missing DTM is an error.
	undeclared := strings.Replace(sample, "BGM++REF001+9'", "BGM++REF001+9'FTX+AAA+++NOTE'", 1)
	missingDTM := strings.Replace(sample, "DTM+137:2609211030:203'", "", 1)

	if err := r.Write(io.Discard, read(undeclared)); err != nil {
		t.Errorf("default refused a warning: %v", err)
	}

	var sb strings.Builder
	err := r.Write(&sb, read(undeclared), RejectWriteOn(Warning))
	if err == nil || !strings.Contains(err.Error(), "warning: message 1 (MSG1), segment 3, FTX+AAA") {
		t.Errorf("err = %v, want the undeclared FTX", err)
	}
	if sb.Len() != 0 {
		t.Errorf("wrote %d bytes before refusing", sb.Len())
	}

	if err := r.Write(io.Discard, read(missingDTM), RejectWriteOn(Severity(9))); err == nil {
		t.Error("a severity above Error loosened the default")
	}
}

func TestReadWithRegistryCollectsStructuralDiagnostics(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(containerSpec()); err != nil {
		t.Fatal(err)
	}

	ix, err := r.Read(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	// The sample holds a DTM the spec requires, and an EQD inside the unit.
	if len(ix.Messages()[0].FindAllGroups("SG2")) != 1 {
		t.Errorf("groups not built: %v", ix.Messages()[0].Groups())
	}

	// Without a registry the same read reports nothing structural.
	plain, err := Read(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.Diagnostics) != 0 || plain.Messages()[0].Groups() != nil {
		t.Errorf("a nil registry produced structure: %v", plain.Diagnostics)
	}
}

func TestRejectReadOn(t *testing.T) {
	// No sender in UNB is a warning, and a UNT count of 9 is an error.
	src := strings.Replace(sample, "UNOA:2+SENDER+", "UNOA:2++", 1)
	src = strings.Replace(src, "UNT+6+", "UNT+9+", 1)
	warnOnly := strings.Replace(sample, "UNOA:2+SENDER+", "UNOA:2++", 1)

	ix, err := Read(strings.NewReader(src))
	if err != nil || len(ix.Diagnostics) != 2 {
		t.Fatalf("default read: err = %v, diagnostics = %v", err, ix.Diagnostics)
	}

	for _, c := range []struct {
		name string
		src  string
		at   Severity
		want string
	}{
		{"error rejects an error", src, Error, "error: message 1 (MSG1), segment 6, UNT: count is 9, counted 6"},
		{"error passes a warning", warnOnly, Error, ""},
		{"warning rejects a warning", warnOnly, Warning, "warning: segment 1, UNB: missing mandatory elements: sender"},
	} {
		t.Run(c.name, func(t *testing.T) {
			ix, err := Read(strings.NewReader(c.src), RejectReadOn(c.at))
			if c.want == "" {
				if err != nil || ix == nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err == nil || ix != nil {
				t.Fatalf("ix = %v, err = %v, want an error and no interchange", ix, err)
			}
			if err.Error() != "edifact: "+c.want {
				t.Errorf("err = %q, want %q", err, "edifact: "+c.want)
			}
		})
	}
}

func TestRejectReadOnCoversSpecDiagnostics(t *testing.T) {
	missingDTM := strings.Replace(sample, "DTM+137:2609211030:203'", "", 1)
	missingDTM = strings.Replace(missingDTM, "UNT+6+", "UNT+5+", 1)

	ix, err := ReadWithSpec(strings.NewReader(missingDTM), containerSpec(), RejectReadOn(Error))
	if err == nil || ix != nil {
		t.Fatalf("ix = %v, err = %v, want the missing DTM as an error", ix, err)
	}
	if !strings.Contains(err.Error(), "DTM+137: required segment is missing") {
		t.Errorf("err = %v", err)
	}
}

// A UNA puts a space in the release position to declare no release character.
const noRelease = "UNA:+.  'UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+C1'" +
	"UNH+M1+TSTMSG:D:95B:UN'" +
	"FTX+AAA+++TWO WORDS'" +
	"FTX+AAA+++FIRST LINE :SECOND LINE'" +
	"UNT+4+M1'UNZ+1+C1'"

func TestReadNoReleaseCharacter(t *testing.T) {
	ix, err := Read(strings.NewReader(noRelease))
	if err != nil {
		t.Fatal(err)
	}
	if ix.Delimiters.Release != 0 {
		t.Errorf("Release = %q, want none", ix.Delimiters.Release)
	}
	ftx := ix.Messages()[0].FindAll("FTX", "AAA")
	if got := ftx[0].Element(4); got != "TWO WORDS" {
		t.Errorf("spaces lost: %q", got)
	}
	if got := ftx[1].Components(4); len(got) != 2 || got[0] != "FIRST LINE" || got[1] != "SECOND LINE" {
		t.Errorf("a space before a separator merged components: %q", got)
	}
}

func TestWriteNoReleaseCharacter(t *testing.T) {
	ix, err := Read(strings.NewReader(noRelease))
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sb.String(), "UNA:+.  '") {
		t.Errorf("UNA does not declare the missing release character: %.12q", sb.String())
	}
	back, err := Read(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	if got := back.Messages()[0].Find("FTX", "AAA").Element(4); got != "TWO WORDS" {
		t.Errorf("after a write and re-read: %q", got)
	}

	// A separator inside a value cannot be escaped, so nothing is written.
	back.Messages()[0].Find("FTX", "AAA").Set(4, 1, "A+B")
	sb.Reset()
	if err := Write(&sb, back); !errors.Is(err, ErrUnreleasable) {
		t.Fatalf("err = %v, want ErrUnreleasable", err)
	}
	if sb.Len() != 0 {
		t.Errorf("wrote %d bytes before refusing", sb.Len())
	}
}

func TestReadRejectsSegmentsAfterUNZ(t *testing.T) {
	cases := map[string]string{
		"a further message":      sample + "UNH+M2+TSTMSG:D:95B:UN:AAA01'BGM++REF2+9'UNT+3+M2'",
		"a second interchange":   sample + sample,
		"a single stray segment": sample + "BGM++REF2+9'",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Read(strings.NewReader(src)); !errors.Is(err, ErrUnexpected) {
				t.Errorf("err = %v, want ErrUnexpected", err)
			}
		})
	}

	// Filler after the trailer is not a segment.
	for name, tail := range map[string]string{"newline": "\n", "ETX": "\x03", "spaces": "  \r\n"} {
		if _, err := Read(strings.NewReader(sample + tail)); err != nil {
			t.Errorf("%s after UNZ: %v", name, err)
		}
	}
}

func TestTrailingSpaces(t *testing.T) {
	src := "UNB+UNOA:2+S+R+260101:1200+C1'" +
		"UNH+M1+TSTMSG:D:95B:UN'" +
		"FTX+AAA+++LEFT  :RIGHT  +NEXT  '" +
		"FTX+AAA+++  LEADING'" +
		"UNT+4+M1'UNZ+1+C1'"

	ix, err := Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	ftx := ix.Messages()[0].FindAll("FTX", "AAA")
	if got := ftx[0].Components(4); got[0] != "LEFT" || got[1] != "RIGHT" {
		t.Errorf("components = %q, want trailing spaces gone from both", got)
	}
	if got := ftx[0].Element(5); got != "NEXT" {
		t.Errorf("last element = %q", got)
	}
	if got := ftx[1].Element(4); got != "  LEADING" {
		t.Errorf("leading spaces should stay: %q", got)
	}

	m := ix.Messages()[0]
	m.Find("FTX", "AAA").Set(4, 1, "PADDED   ")
	m.Add("FTX", "AAA", "", "", "    ") // only spaces, so nothing is left
	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, "FTX+AAA+++PADDED:RIGHT+NEXT'") {
		t.Errorf("trailing spaces were written: %q", out)
	}
	if strings.Contains(out, "    ") {
		t.Errorf("a value of spaces was written: %q", out)
	}
}

func TestReadStoresOneRepetition(t *testing.T) {
	ix, err := Read(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	eqd := ix.Messages()[0].Find("EQD", "CN")
	for i, want := range []int{1, 1, 1, 0, 0, 1} {
		if got := eqd.Repetitions(i + 1); got != want {
			t.Errorf("EQD element %d: %d repetitions, want %d", i+1, got, want)
		}
	}
}

const repeating = "UNA:+.?*'UNB+UNOA:4+S+R+20260101:1200+C1'" +
	"UNH+M1+TSTMSG:D:95B:UN'FTX+AAA+++A:1*B?*C*D:2'FTX+AAA+++E**'UNT+4+M1'UNZ+1+C1'"

func TestReadRepetitions(t *testing.T) {
	ix, err := Read(strings.NewReader(repeating))
	if err != nil {
		t.Fatal(err)
	}
	ftx := ix.Messages()[0].FindAll("FTX", "AAA")

	if got := ftx[0].Repetitions(4); got != 3 {
		t.Fatalf("%d repetitions, want 3", got)
	}
	for r, want := range [][]string{{"A", "1"}, {"B*C"}, {"D", "2"}} {
		got := ftx[0].Repetition(4, r+1)
		if strings.Join(got, ":") != strings.Join(want, ":") {
			t.Errorf("repetition %d = %q, want %q", r+1, got, want)
		}
	}
	if got := ftx[0].Element(4); got != "A" {
		t.Errorf("Element reads %q, want the first repetition", got)
	}
	if got := ftx[1].Repetitions(4); got != 1 {
		t.Errorf("trailing empty repetitions kept: %d", got)
	}
}

func TestWriteRepetitions(t *testing.T) {
	ix, err := Read(strings.NewReader(repeating))
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.HasPrefix(out, "UNA:+.?*'") {
		t.Errorf("UNA lost the repetition separator: %.9q", out)
	}
	if !strings.Contains(out, "FTX+AAA+++A:1*B?*C*D:2'FTX+AAA+++E'") {
		t.Errorf("repetitions not written back: %q", out)
	}
}

func TestSetRepetition(t *testing.T) {
	ix := completeInterchange()
	ix.SetCharset(UNOA)
	ix.SetSyntaxVersion(4)
	ix.SetDate("20260101")
	s := completeMessage(ix, nil, "M1").Add("FTX", "AAA")
	s.SetRepetition(4, 1, 1, "A")
	s.SetRepetition(4, 3, 2, "C")

	if s.Repetitions(4) != 3 || s.Repetition(4, 2) != nil || s.Component(4, 1) != "A" {
		t.Errorf("repetitions %d, second %q", s.Repetitions(4), s.Repetition(4, 2))
	}

	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "FTX+AAA+++A**:C'") {
		t.Errorf("an empty repetition was not kept in place: %q", sb.String())
	}
}

func TestWriteRepetitionsWithoutASeparator(t *testing.T) {
	ix := completeInterchange()
	s := completeMessage(ix, nil, "M1").Add("FTX", "AAA")
	s.SetRepetition(4, 2, 1, "B")

	var sb strings.Builder
	if err := Write(&sb, ix); !errors.Is(err, ErrNoRepetitionSeparator) {
		t.Fatalf("err = %v, want ErrNoRepetitionSeparator", err)
	}
	if sb.Len() != 0 {
		t.Errorf("wrote %q before failing", sb.String())
	}
}

func TestAsteriskIsDataBelowVersion4(t *testing.T) {
	src := "UNA:+.? 'UNB+UNOA:3+S+R+260101:1200+C1'" +
		"UNH+M1+TSTMSG:D:95B:UN'FTX+AAA+++A*B'UNT+3+M1'UNZ+1+C1'"
	ix, err := Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	ftx := ix.Messages()[0].Find("FTX", "AAA")
	if ftx.Repetitions(4) != 1 || ftx.Element(4) != "A*B" {
		t.Errorf("%d repetitions, element 4 %q", ftx.Repetitions(4), ftx.Element(4))
	}

	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "FTX+AAA+++A*B'") {
		t.Errorf("the asterisk was escaped: %q", sb.String())
	}
}

func TestRepetitionSeparatorWithoutRelease(t *testing.T) {
	ix := completeInterchange()
	ix.SetCharset(UNOA)
	ix.SetSyntaxVersion(4)
	ix.SetDate("20260101")
	ix.Delimiters.Release = 0
	completeMessage(ix, nil, "M1").Add("FTX", "AAA", "", "", "A*B")

	var sb strings.Builder
	if err := Write(&sb, ix); !errors.Is(err, ErrUnreleasable) {
		t.Fatalf("err = %v, want ErrUnreleasable", err)
	}
}

func TestReadSkipsLeadingFiller(t *testing.T) {
	body := "UNB+UNOA:2+S+R+260101:1200+C1'UNH+M1+TSTMSG:D:95B:UN'UNT+2+M1'UNZ+1+C1'"
	cases := map[string]string{
		"BOM":             "\xEF\xBB\xBF" + body,
		"STX":             "\x02" + body,
		"STX and ETX":     "\x02" + body + "\x03",
		"BOM before UNA":  "\xEF\xBB\xBF" + "UNA:+.? '" + body,
		"newline and BOM": "\r\n\xEF\xBB\xBF" + body,
	}
	for name, src := range cases {
		ix, err := Read(strings.NewReader(src))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if ix.ControlRef() != "C1" || len(ix.Messages()) != 1 {
			t.Errorf("%s: control ref %q, %d messages", name, ix.ControlRef(), len(ix.Messages()))
		}
	}

	ix, err := Read(strings.NewReader("\xEF\xBB\xBF" + "UNA:+.  '" + body))
	if err != nil {
		t.Fatal(err)
	}
	if ix.Delimiters.Release != 0 {
		t.Error("the UNA after a BOM was not applied")
	}
}

func TestReadOnlyFiller(t *testing.T) {
	if _, err := Read(strings.NewReader("\xEF\xBB\xBF\r\n")); !errors.Is(err, ErrNoUNB) {
		t.Errorf("err = %v, want ErrNoUNB", err)
	}
}

func TestDiagnosticsNameTheirMessage(t *testing.T) {
	src := "UNB+UNOA:2+S+R+260101:1200+C1'" +
		"UNH+M1+AAAAAA:D:95B:UN'BGM++REF1+9'UNT+3+M1'" +
		"UNH+M2+BBBBBB:D:95B:UN'BGM++REF2+9'UNT+9+M2'" +
		"UNZ+3+C1'"
	ix, err := NewRegistry().Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"error: message 2 (M2), segment 3, UNT: count is 9, counted 3",
		"error: segment 8, UNZ: count is 3, counted 2",
		"error: message 1 (M1), segment 1, UNH: no spec matches AAAAAA:D:95B:UN:-",
		"error: message 2 (M2), segment 1, UNH: no spec matches BBBBBB:D:95B:UN:-",
	}
	if len(ix.Diagnostics) != len(want) {
		t.Fatalf("diagnostics = %v", ix.Diagnostics)
	}
	for i, d := range ix.Diagnostics {
		if d.String() != want[i] {
			t.Errorf("diagnostic %d:\n got %s\nwant %s", i, d, want[i])
		}
	}
}

func TestValidationCountsSegmentsFromUNH(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
	}}); err != nil {
		t.Fatal(err)
	}
	src := "UNB+UNOA:2+S+R+260101:1200+C1'" +
		"UNH+M1+TSTMSG:D:95B:UN'BGM'FTX+AAA'UNT+4+M1'UNZ+1+C1'"
	ix, err := reg.Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %v", ix.Diagnostics)
	}
	d := ix.Diagnostics[0]
	if d.MessageIdx != 1 || d.MessageRef != "M1" || d.SegmentIdx != 3 || d.Tag != "FTX" {
		t.Errorf("diagnostic = %+v, want message 1 (M1), segment 3, FTX", d)
	}
}

func groupedForWrite(t *testing.T) (*Interchange, *Message, *Registry) {
	t.Helper()
	reg := NewRegistry()
	if err := reg.Register(Spec{Key: testKey(), Nodes: []Node{
		{Tag: "BGM", Required: true},
		{Group: "SG1", Max: Unbounded, Nodes: []Node{{Tag: "LOC", Required: true}}},
	}}); err != nil {
		t.Fatal(err)
	}

	ix := completeInterchange()
	m := ix.AddMessageTo(nil, testKey())
	m.SetRef("M1")
	m.Add("BGM", "", "REF1")
	m.AddGroup("OWN").Add("LOC", "147")
	return ix, m, reg
}

func TestWriteLeavesGroupsAlone(t *testing.T) {
	ix, m, reg := groupedForWrite(t)

	if err := reg.Write(&strings.Builder{}, ix); err != nil {
		t.Fatal(err)
	}
	if len(m.FindAllGroups("OWN")) != 1 || len(m.FindAllGroups("SG1")) != 0 {
		t.Errorf("groups after Write: %d OWN, %d SG1", len(m.FindAllGroups("OWN")), len(m.FindAllGroups("SG1")))
	}
}

func TestValidateClearsGroupsWithoutASpec(t *testing.T) {
	ix, m, _ := groupedForWrite(t)

	NewRegistry().Validate(ix)
	if m.Groups() != nil {
		t.Errorf("stale groups kept: %v", m.Groups())
	}
}

func TestReadWarnsOfIncompleteHeaders(t *testing.T) {
	src := "UNB+UNOA:2++R+260101:1200+C1'" +
		"UNG+TSTMSG+AS+AR+260101:1200+G1'" +
		"UNH+M1+TSTMSG:D::UN'BGM++REF1'UNT+3+M1'UNE+1+G1'UNZ+1+C1'"
	ix, err := Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"warning: segment 1, UNB: missing mandatory elements: sender",
		"warning: segment 2, UNG: missing mandatory elements: controlling agency, message version, message release",
		"warning: message 1 (M1), segment 1, UNH: missing mandatory elements: message release",
	}
	if len(ix.Diagnostics) != len(want) {
		t.Fatalf("diagnostics = %v", ix.Diagnostics)
	}
	for i, d := range ix.Diagnostics {
		if d.String() != want[i] {
			t.Errorf("diagnostic %d:\n got %s\nwant %s", i, d, want[i])
		}
	}
}

func TestAllowIncompleteHeaders(t *testing.T) {
	src := "UNB+UNOA:2++R+260101:1200+C1'UNH+M1+TSTMSG:D:95B:UN'BGM'UNT+3+M1'UNZ+1+C1'"
	ix, err := Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	if err := Write(&sb, ix); !errors.Is(err, ErrIncompleteHeader) {
		t.Fatalf("err = %v, want ErrIncompleteHeader", err)
	}
	if err := Write(&sb, ix, AllowIncompleteHeaders()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "UNB+UNOA:2++R+260101:1200+C1'") {
		t.Errorf("UNB not written as read: %q", sb.String())
	}
}

func TestTagIndices(t *testing.T) {
	src := "UNB+UNOA:2+S+R+260101:1200+C1'UNH+M1+TSTMSG:D:95B:UN'" +
		"BGM:1+REF1+9'DDD:1:2+X'EEE:2::1+Y'UNT+5+M1'UNZ+1+C1'"
	ix, err := Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	m := ix.Messages()[0]

	bgm := m.Find("BGM", "")
	if bgm == nil {
		t.Fatal("BGM not found by its segment code")
	}
	if got := bgm.Index(); len(got) != 1 || got[0] != "1" {
		t.Errorf("BGM index = %q", got)
	}
	if got := m.Find("DDD", "X").Index(); strings.Join(got, ":") != "1:2" {
		t.Errorf("DDD index = %q", got)
	}
	if got := m.Find("EEE", "Y").Index(); strings.Join(got, ":") != "2::1" {
		t.Errorf("EEE index = %q", got)
	}
	if got := m.Find("BGM", "").Element(1); got != "REF1" {
		t.Errorf("BGM element 1 = %q", got)
	}

	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "BGM:1+REF1+9'DDD:1:2+X'EEE:2::1+Y'") {
		t.Errorf("indices not written back: %q", sb.String())
	}
}

func TestTagIndicesMatchSpecs(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
	}}); err != nil {
		t.Fatal(err)
	}
	src := "UNB+UNOA:2+S+R+260101:1200+C1'UNH+M1+TSTMSG:D:95B:UN'BGM:1+REF1'UNT+3+M1'UNZ+1+C1'"
	ix, err := reg.Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Diagnostics) != 0 {
		t.Errorf("diagnostics = %v", ix.Diagnostics)
	}
}

func TestSetIndex(t *testing.T) {
	ix := completeInterchange()
	m := completeMessage(ix, nil, "M1")
	s := m.Add("DDD", "X")
	s.SetIndex("1", "2")

	idx := s.Index()
	idx[0] = "9"
	if s.Index()[0] != "1" {
		t.Error("editing Index reached the segment")
	}

	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "DDD:1:2+X'") {
		t.Errorf("index not written: %q", sb.String())
	}

	s.SetIndex()
	if s.Index() != nil {
		t.Errorf("SetIndex() left %q", s.Index())
	}
}

func TestReadLevelBWithoutUNA(t *testing.T) {
	const is1, is3, is4 = "\x1f", "\x1d", "\x1c"
	src := "UNB" + is3 + "UNOB" + is1 + "2" + is3 + "S" + is3 + "R" + is3 + "260101" + is1 + "1200" + is3 + "C1" + is4 +
		"UNZ" + is3 + "0" + is3 + "C1" + is4
	_, err := Read(strings.NewReader(src))
	if !errors.Is(err, ErrUnsupportedSeparators) || !strings.HasSuffix(err.Error(), "syntax identifier UNOB") {
		t.Errorf("err = %v, want ErrUnsupportedSeparators naming UNOB", err)
	}
}

func TestReadLevelBWithUNA(t *testing.T) {
	const is1, is3, is4 = "\x1f", "\x1d", "\x1c"
	src := "UNA" + is1 + is3 + ". " + " " + is4 +
		"UNB" + is3 + "UNOB" + is1 + "2" + is3 + "S" + is3 + "R" + is3 + "260101" + is1 + "1200" + is3 + "C1" + is4 +
		"UNZ" + is3 + "0" + is3 + "C1" + is4
	ix, err := Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := ix.Charset(); c != UNOB || ix.Date() != "260101" {
		t.Errorf("charset %v, date %q", c, ix.Date())
	}
}

func TestReadOtherIdentifiersWithLevelASeparators(t *testing.T) {
	src := "UNB+UNOC:3+S+R+260101:1200+C1'UNZ+0+C1'"
	if _, err := Read(strings.NewReader(src)); err != nil {
		t.Error(err)
	}
}

func TestDeclaredDecimalCommaRoundTrips(t *testing.T) {
	src := "UNA:+,? 'UNB+UNOA:2+S+R+260101:1200+C1'" +
		"UNH+M1+TSTMSG:D:95B:UN'MEA+WT++KGM:8500,5'UNT+3+M1'UNZ+1+C1'"
	ix, err := Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sb.String(), "UNA:+,? '") || !strings.Contains(sb.String(), "MEA+WT++KGM:8500,5'") {
		t.Errorf("comma not kept: %q", sb.String())
	}
}

func TestSetFloatFollowsTheDeclaredDecimal(t *testing.T) {
	ix := completeInterchange()
	ix.Delimiters.Decimal = ','
	m := completeMessage(ix, nil, "M1")
	mea := m.Add("MEA", "WT")
	mea.Set(3, 1, "KGM")
	mea.SetFloat(3, 2, 8500.5, -1)
	ftx := m.Add("FTX", "AAA")
	ftx.SetFloat(4, 1, 1.5, -1)
	ftx.Set(4, 1, "1.5") // text now, written as set

	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.HasPrefix(out, "UNA:+,? '") {
		t.Errorf("UNA = %.9q, want a comma declared", out)
	}
	if !strings.Contains(out, "MEA+WT++KGM:8500,5'") {
		t.Errorf("SetFloat value not written with a comma: %q", out)
	}
	if !strings.Contains(out, "FTX+AAA+++1.5'") {
		t.Errorf("text overwriting a SetFloat value was changed: %q", out)
	}
	if got := mea.Component(3, 2); got != "8500.5" {
		t.Errorf("held value = %q, want 8500.5", got)
	}
}

func TestUndeclaredDecimalWritesADot(t *testing.T) {
	ix := completeInterchange()
	ix.Delimiters.Decimal = 0
	completeMessage(ix, nil, "M1").Add("BGM", "", "REF1")
	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sb.String(), "UNA:+.? '") {
		t.Errorf("UNA = %.9q, want a dot declared", sb.String())
	}
}

func emptySegmentsMessage() *Interchange {
	ix := completeInterchange()
	m := completeMessage(ix, nil, "M1")
	m.Add("BGM")
	m.Add("FTX", "", "", "", "")
	m.AddComposite("FTX", []string{"", " "}, nil)
	m.Add("DTM", "137", "202601011200")
	return ix
}

func TestWriteKeepsEmptySegmentsWithoutASpec(t *testing.T) {
	var sb strings.Builder
	if err := Write(&sb, emptySegmentsMessage()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "UNH+M1+TSTMSG:D:95B:UN'BGM'FTX'FTX'DTM+137+202601011200'UNT+6+M1'") {
		t.Errorf("empty segments not kept: %q", sb.String())
	}
}

func TestWriteOmitsOnlyConditionalEmptySegments(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
		{Tag: "FTX", Max: 9},
		{Tag: "DTM", Qualifier: "137", Required: true},
	}}); err != nil {
		t.Fatal(err)
	}
	ix := emptySegmentsMessage()
	var sb strings.Builder
	if err := reg.Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "UNH+M1+TSTMSG:D:95B:UN'BGM'DTM+137+202601011200'UNT+4+M1'") {
		t.Errorf("want the mandatory BGM kept and the conditional FTXs left out: %q", sb.String())
	}
	if n := len(ix.Messages()[0].Segments()); n != 4 {
		t.Errorf("Write removed segments from the message: %d left", n)
	}

	back, err := reg.Read(strings.NewReader(sb.String()))
	if err != nil || len(back.Diagnostics) != 0 {
		t.Errorf("re-read: %v %v", err, back.Diagnostics)
	}
}

func TestInvalidDelimiters(t *testing.T) {
	for _, una := range []string{"UNA:+.? :", "UNA:+:? '", "UNA++.? '", "UNA:+.?+'"} {
		_, err := Read(strings.NewReader(una + "UNB+UNOA:4+S+R+20260101:1200+C1'UNZ+0+C1'"))
		if !errors.Is(err, ErrInvalidDelimiters) {
			t.Errorf("%s: err = %v, want ErrInvalidDelimiters", una, err)
		}
	}

	for name, set := range map[string]func(*Delimiters){
		"component equal to element": func(d *Delimiters) { d.Component = '+' },
		"component equal to decimal": func(d *Delimiters) { d.Component = '.' },
		"no segment terminator":      func(d *Delimiters) { d.Segment = 0; d.Component = 0 },
	} {
		ix := completeInterchange()
		completeMessage(ix, nil, "M1").Add("BGM", "", "REF1")
		set(&ix.Delimiters)
		var sb strings.Builder
		if err := Write(&sb, ix); !errors.Is(err, ErrInvalidDelimiters) {
			t.Errorf("%s: err = %v, want ErrInvalidDelimiters", name, err)
		}
		if sb.Len() != 0 {
			t.Errorf("%s: wrote %q", name, sb.String())
		}
	}
}

func TestRepetitionsFollowTheSyntaxVersion(t *testing.T) {
	body := "UNH+M1+TSTMSG:D:95B:UN'COM+111:TE*222:FX'UNT+3+M1'UNZ+1+C1'"
	cases := []struct {
		name, src string
		reps      int
	}{
		{"version 4 without a UNA", "UNB+UNOA:4+S+R+20260101:1200+C1'" + body, 2},
		{"version 4 with a UNA declaring *", "UNA:+.?*'UNB+UNOA:4+S+R+20260101:1200+C1'" + body, 2},
		{"version 4 with a UNA declaring none", "UNA:+.? 'UNB+UNOA:4+S+R+20260101:1200+C1'" + body, 1},
		{"version 2 with a UNA declaring *", "UNA:+.?*'UNB+UNOA:2+S+R+260101:1200+C1'" + body, 1},
		{"version 2 without a UNA", "UNB+UNOA:2+S+R+260101:1200+C1'" + body, 1},
	}
	for _, c := range cases {
		ix, err := Read(strings.NewReader(c.src))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := ix.Messages()[0].Find("COM", "").Repetitions(1); got != c.reps {
			t.Errorf("%s: %d repetitions, want %d", c.name, got, c.reps)
		}
	}
}

func TestWriteRepetitionSeparatorFollowsTheSyntaxVersion(t *testing.T) {
	ix := completeInterchange()
	ix.Delimiters.Repetition = '*'
	completeMessage(ix, nil, "M1").Add("FTX", "AAA", "", "", "NOTE*1")
	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sb.String(), "UNA:+.? '") || !strings.Contains(sb.String(), "FTX+AAA+++NOTE*1'") {
		t.Errorf("version 2 output: %q", sb.String())
	}

	ix = completeInterchange()
	ix.SetCharset(UNOA)
	ix.SetSyntaxVersion(4)
	ix.SetDate("20260101")
	s := completeMessage(ix, nil, "M1").Add("COM")
	s.SetRepetition(1, 1, 1, "111")
	s.SetRepetition(1, 2, 1, "222")
	sb.Reset()
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sb.String(), "UNA:+.?*'") || !strings.Contains(sb.String(), "COM+111*222'") {
		t.Errorf("version 4 output: %q", sb.String())
	}
}

func TestReadSkipsTrailingFiller(t *testing.T) {
	body := "UNB+UNOA:2+S+R+260101:1200+C1'UNH+M1+TSTMSG:D:95B:UN'BGM++REF1'UNT+3+M1'UNZ+1+C1'"
	for name, tail := range map[string]string{
		"BOM":             "\xEF\xBB\xBF",
		"ETX":             "\x03",
		"newline and BOM": "\r\n\xEF\xBB\xBF",
		"BOM and newline": "\xEF\xBB\xBF\n",
	} {
		if _, err := Read(strings.NewReader(body + tail)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Read(strings.NewReader(body + "XYZ")); !errors.Is(err, ErrUnterminated) {
		t.Errorf("trailing data: err = %v, want ErrUnterminated", err)
	}
}

func TestSyntaxVersionSurvivesFillerAfterTheUNA(t *testing.T) {
	body := "UNB+UNOA:4+S+R+20260101:1200+C1'UNH+M1+TSTMSG:D:95B:UN'COM+111:TE*222:FX'UNT+3+M1'UNZ+1+C1'"
	for name, gap := range map[string]string{"LF": "\n", "CRLF": "\r\n", "spaces": "  "} {
		ix, err := Read(strings.NewReader("UNA:+.?*'" + gap + body))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := ix.Message(1).Find("COM", "").Repetitions(1); got != 2 {
			t.Errorf("%s: %d repetitions, want 2", name, got)
		}
	}
}

func TestVersion4RoundTripsWithLineBreaks(t *testing.T) {
	ix := completeInterchange()
	ix.SetCharset(UNOA)
	ix.SetSyntaxVersion(4)
	ix.SetDate("20260101")
	s := completeMessage(ix, nil, "M1").Add("COM")
	s.SetRepetition(1, 1, 1, "111")
	s.SetRepetition(1, 2, 1, "222")

	out := ""
	for pass := 1; pass <= 3; pass++ {
		var sb strings.Builder
		if err := Write(&sb, ix, SegmentSuffixLF()); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sb.String(), "COM+111*222'") {
			t.Fatalf("pass %d: %q", pass, sb.String())
		}
		if out != "" && sb.String() != out {
			t.Fatalf("pass %d changed the output:\n%q\n%q", pass, out, sb.String())
		}
		out = sb.String()
		var err error
		if ix, err = Read(strings.NewReader(out)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadAndReadWithSpec(t *testing.T) {
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
		{Group: "SG1", Max: Unbounded, Nodes: []Node{{Tag: "LOC", Qualifier: "147", Required: true}}},
	}}
	src := "UNB+UNOA:2+S+R+260101:1200+C1'" +
		"UNH+M1+TSTMSG:D:95B:UN'BGM++REF1'LOC+147+A'UNT+4+M1'" +
		"UNH+M2+OTHER:D:95B:UN'BGM++REF2'UNT+3+M2'" +
		"UNZ+2+C1'"

	// Read consults no spec.
	ix, err := Read(strings.NewReader(src))
	if err != nil || ix.Message(1).FindGroup("SG1") != nil || len(ix.Diagnostics) != 0 {
		t.Fatalf("Read: %v %v", err, ix.Diagnostics)
	}

	// A registry's Read consults its specs.
	reg := NewRegistry()
	if err := reg.Register(spec); err != nil {
		t.Fatal(err)
	}
	ix, err = reg.Read(strings.NewReader(src))
	if err != nil || ix.Message(1).FindGroup("SG1") == nil {
		t.Fatalf("registry Read: %v", err)
	}
	if len(ix.Diagnostics) != 1 || ix.Diagnostics[0].Message != "no spec matches OTHER:D:95B:UN:-" {
		t.Errorf("diagnostics = %v", ix.Diagnostics)
	}

	// ReadWithSpec consults only the spec given.
	ix, err = ReadWithSpec(strings.NewReader(src), spec)
	if err != nil || ix.Message(1).FindGroup("SG1") == nil {
		t.Fatalf("ReadWithSpec: %v", err)
	}
	if len(ix.Diagnostics) != 1 || ix.Diagnostics[0].MessageRef != "M2" {
		t.Errorf("ReadWithSpec diagnostics = %v", ix.Diagnostics)
	}
	if _, err := ReadWithSpec(strings.NewReader(src), Spec{}); err == nil {
		t.Error("ReadWithSpec accepted an invalid spec")
	}
}

func TestWriteWithSpec(t *testing.T) {
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{{Tag: "BGM", Required: true}, {Tag: "DTM", Required: true}}}
	ix := completeInterchange()
	completeMessage(ix, nil, "M1").Add("BGM", "", "REF1")

	if err := Write(&strings.Builder{}, ix); err != nil {
		t.Errorf("Write with no spec: %v", err)
	}
	var sb strings.Builder
	if err := WriteWithSpec(&sb, ix, spec); err == nil || !strings.Contains(err.Error(), "DTM: required segment is missing") || sb.Len() != 0 {
		t.Errorf("WriteWithSpec: %v, wrote %d bytes", err, sb.Len())
	}

	var none *Registry
	if err := none.Write(&strings.Builder{}, ix); err != nil {
		t.Errorf("nil registry Write: %v", err)
	}
	if _, err := none.Read(strings.NewReader("UNB+UNOA:2+S+R+260101:1200+C1'UNH+M1+TSTMSG:D:95B:UN'BGM++A'UNT+3+M1'UNZ+1+C1'")); err != nil {
		t.Errorf("nil registry Read: %v", err)
	}
}
