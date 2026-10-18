package edifact_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/MaldivesPorts/edifact"
)

// bayplan is a short interchange cut down from a BAPLIE bayplan: two
// containers in their stowage cells. Newlines between segments are optional
// and ignored on read.
const bayplan = `UNA:+.? '
UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'
UNH+M1+BAPLIE:D:95B:UN'
BGM++BAY1+9'
DTM+137:202601011200:203'
LOC+147+0010204::5'
MEA+WT++KGM:12000'
LOC+9+XXAAA:139:6'
LOC+11+XXBBB:139:6'
EQD+CN+XXXU1234567+22G1'
LOC+147+0010206::5'
MEA+WT++KGM:8500.5'
LOC+9+XXAAA:139:6'
LOC+11+XXCCC:139:6'
EQD+CN+XXXU7654321+45G1'
UNT+14+M1'
UNZ+1+REF1'`

// bayplanSpec is a simplified specification after the BAPLIE D.95B segment
// table, which has more segments than this: a header, then one SG2 per stowage
// cell, each holding an SG3 for the container in it.
func bayplanSpec() edifact.Spec {
	return edifact.Spec{
		Key: edifact.Key{
			Type:    edifact.Is("BAPLIE"),
			Version: edifact.Is("D"),
			Release: edifact.Is("95B"),
			Agency:  edifact.Is("UN"),
		},
		Nodes: []edifact.Node{
			{Tag: "BGM", Required: true},
			{Tag: "DTM", Qualifier: "137", Required: true},
			{Group: "SG2", Max: edifact.Unbounded, Nodes: []edifact.Node{
				{Tag: "LOC", Qualifier: "147", Required: true},
				{Tag: "MEA", Qualifier: "WT"},
				{Tag: "LOC", Qualifiers: []string{"9", "11"}, Max: 9},
				{Group: "SG3", Max: 3, Nodes: []edifact.Node{
					{Tag: "EQD", Qualifier: "CN", Required: true},
				}},
			}},
		},
	}
}

// Reading a file and pulling values out by position.
func Example() {
	ix, err := edifact.Read(strings.NewReader(bayplan))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("from", ix.Sender(), "to", ix.Recipient())
	m := ix.Message(1)
	fmt.Println("message", m.Key())
	for _, eqd := range m.FindAll("EQD", "CN") {
		fmt.Println("container", eqd.Element(2), "size and type", eqd.Element(3))
	}
	// Output:
	// from SENDER to RECIPIENT
	// message BAPLIE:D:95B:UN:-
	// container XXXU1234567 size and type 22G1
	// container XXXU7654321 size and type 45G1
}

// Building an interchange and writing it.
func ExampleWrite() {
	ix := edifact.NewInterchange()
	ix.SetSender("SENDER", "")
	ix.SetRecipient("RECIPIENT", "")
	ix.SetDate("260101")
	ix.SetTime("1200")
	ix.SetControlRef("REF1")

	m := ix.AddMessage(edifact.Key{
		Type:    edifact.Is("BAPLIE"),
		Version: edifact.Is("D"),
		Release: edifact.Is("95B"),
		Agency:  edifact.Is("UN"),
	})
	m.SetRef("M1")
	m.Add("BGM", "", "BAY1", "9")
	m.AddComposite("DTM", []string{"137", "202601011200", "203"})
	m.AddComposite("LOC", []string{"147"}, []string{"0010204", "", "5"})
	m.Add("EQD", "CN", "XXXU1234567", "22G1")

	if err := edifact.Write(os.Stdout, ix, edifact.SegmentSuffixLF()); err != nil {
		fmt.Println(err)
	}
	// Output:
	// UNA:+.? '
	// UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'
	// UNH+M1+BAPLIE:D:95B:UN'
	// BGM++BAY1+9'
	// DTM+137:202601011200:203'
	// LOC+147+0010204::5'
	// EQD+CN+XXXU1234567+22G1'
	// UNT+6+M1'
	// UNZ+1+REF1'
}

// Reading against a specification sorts the segments into segment groups, so
// each container is read together with its own cell, weight and ports.
func ExampleRegistry_Read() {
	reg := edifact.NewRegistry()
	if err := reg.Register(bayplanSpec()); err != nil {
		fmt.Println(err)
		return
	}

	ix, err := reg.Read(strings.NewReader(bayplan))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(len(ix.Diagnostics), "diagnostics")

	for _, cell := range ix.Message(1).FindAllGroups("SG2") {
		weight, _ := cell.Find("MEA", "WT").Float(3, 2)
		container := cell.FindGroup("SG3").Find("EQD", "CN").Element(2)
		fmt.Printf("%s in %s, %.1f kg, %s to %s\n",
			container,
			cell.Find("LOC", "147").Component(2, 1),
			weight,
			cell.Find("LOC", "9").Component(2, 1),
			cell.Find("LOC", "11").Component(2, 1))
	}
	// Output:
	// 0 diagnostics
	// XXXU1234567 in 0010204, 12000.0 kg, XXAAA to XXBBB
	// XXXU7654321 in 0010206, 8500.5 kg, XXAAA to XXCCC
}

// Read reports what does not fit the specification as diagnostics rather than
// failing, so a partly wrong file can still be read.
func ExampleDiagnostic() {
	reg := edifact.NewRegistry()
	if err := reg.Register(bayplanSpec()); err != nil {
		fmt.Println(err)
		return
	}

	broken := strings.Replace(bayplan, "DTM+137:202601011200:203'", "FTX+AAI+++NOTE'", 1)
	ix, err := reg.Read(strings.NewReader(broken))
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, d := range ix.Diagnostics {
		fmt.Println(d)
	}
	// Output:
	// warning: message 1 (M1), segment 3, FTX+AAI: segment is not declared here
	// error: message 1 (M1), segment 4, DTM+137: required segment is missing
}

// Element and Component count from 1, the way the published segment tables
// number positions. Typed accessors convert one component.
func ExampleSegment_Time() {
	ix, err := edifact.Read(strings.NewReader(bayplan))
	if err != nil {
		fmt.Println(err)
		return
	}
	dtm := ix.Message(1).Find("DTM", "137")

	fmt.Println(dtm.Component(1, 2), "in format", dtm.Component(1, 3))
	t, err := dtm.Time(1, 2)
	fmt.Println(t.Format(time.RFC3339), err)
	// Output:
	// 202601011200 in format 203
	// 2026-01-01T12:00:00Z <nil>
}

// Float accepts a dot or a comma as the decimal mark, and rejects anything
// ISO 9735 does not allow as a number.
func ExampleSegment_Float() {
	ix := edifact.NewInterchange()
	m := ix.AddMessage(edifact.Key{})
	mea := m.AddComposite("MEA", []string{"WT"}, nil, []string{"KGM", "8500,5"})

	v, err := mea.Float(3, 2)
	fmt.Println(v, err)

	mea.Set(3, 2, "1,000.5")
	_, err = mea.Float(3, 2)
	fmt.Println(errors.Is(err, edifact.ErrNumeric))
	// Output:
	// 8500.5 <nil>
	// true
}

// Add on a segment group places the new segment inside that segment group,
// even when it is not the last one, so the written order matches the segment
// group.
func ExampleSegmentGroup_Add() {
	reg := edifact.NewRegistry()
	spec := bayplanSpec()
	spec.Nodes[2].Nodes = append(spec.Nodes[2].Nodes, edifact.Node{Tag: "FTX", Qualifier: "AAA"})
	if err := reg.Register(spec); err != nil {
		fmt.Println(err)
		return
	}
	ix, err := reg.Read(strings.NewReader(bayplan))
	if err != nil {
		fmt.Println(err)
		return
	}

	first := ix.Message(1).FindGroup("SG2")
	first.Add("FTX", "AAA", "", "", "HANDLE WITH CARE")

	for _, s := range ix.Message(1).Segments() {
		fmt.Println(strings.TrimSpace(s.Tag + " " + s.Qualifier()))
	}
	// Output:
	// BGM
	// DTM 137
	// LOC 147
	// MEA WT
	// LOC 9
	// LOC 11
	// EQD CN
	// FTX AAA
	// LOC 147
	// MEA WT
	// LOC 9
	// LOC 11
	// EQD CN
}

// In syntax version 4 an element can repeat within one segment, separated by
// the character in the fifth position of the UNA.
func ExampleSegment_Repetition() {
	src := "UNA:+.?*'" +
		"UNB+UNOA:4+SENDER+RECIPIENT+20260101:1200+REF1'" +
		"UNH+M1+TSTMSG:D:95B:UN'" +
		"COM+111:TE*222:FX'" +
		"UNT+3+M1'UNZ+1+REF1'"
	ix, err := edifact.Read(strings.NewReader(src))
	if err != nil {
		fmt.Println(err)
		return
	}
	com := ix.Message(1).Find("COM", "")

	for r := 1; r <= com.Repetitions(1); r++ {
		fmt.Println(com.Repetition(1, r))
	}
	fmt.Println(com.Element(1))
	// Output:
	// [111 TE]
	// [222 FX]
	// 111
}

// Load registers several specs at once. When more than one matches a message,
// the one constraining the most Key fields is used.
func ExampleRegistry_Load() {
	plain := edifact.Spec{
		Key: edifact.Key{
			Type:    edifact.Is("BAPLIE"),
			Version: edifact.Is("D"),
			Release: edifact.Is("95B"),
			Agency:  edifact.Is("UN"),
		},
		Nodes: []edifact.Node{
			{Tag: "BGM", Required: true},
			{Tag: "DTM", Qualifier: "137", Required: true},
		},
	}

	// The same message with an association code, as an industry guide might
	// define it.
	variant := plain.Clone()
	variant.Key.Association = edifact.Is("XX01")
	variant.Nodes = append(variant.Nodes, edifact.Node{Tag: "FTX", Qualifier: "AAA"})

	reg := edifact.NewRegistry()
	if err := reg.Load([]edifact.Spec{plain, variant}); err != nil {
		fmt.Println(err)
		return
	}

	src := "UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'" +
		"UNH+M1+BAPLIE:D:95B:UN'BGM++BAY1+9'DTM+137:202601011200:203'UNT+4+M1'" +
		"UNH+M2+BAPLIE:D:95B:UN:XX01'BGM++BAY2+9'DTM+137:202601011200:203'FTX+AAA+++NOTE'UNT+5+M2'" +
		"UNZ+2+REF1'"
	ix, err := reg.Read(strings.NewReader(src))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(len(ix.Diagnostics), "diagnostics")
	for _, m := range ix.Messages() {
		spec, _ := reg.Lookup(m.Key())
		fmt.Println(m.Ref(), m.Key(), "uses", spec.Key)
	}
	// Output:
	// 0 diagnostics
	// M1 BAPLIE:D:95B:UN:- uses BAPLIE:D:95B:UN:*
	// M2 BAPLIE:D:95B:UN:XX01 uses BAPLIE:D:95B:UN:XX01
}

// Validate checks an interchange built in code against the registry's specs,
// before Write would refuse it.
func ExampleRegistry_Validate() {
	reg := edifact.NewRegistry()
	if err := reg.Register(bayplanSpec()); err != nil {
		fmt.Println(err)
		return
	}

	ix := edifact.NewInterchange()
	m := ix.AddMessage(edifact.Key{
		Type:    edifact.Is("BAPLIE"),
		Version: edifact.Is("D"),
		Release: edifact.Is("95B"),
		Agency:  edifact.Is("UN"),
	})
	m.SetRef("M1")
	m.Add("BGM", "", "BAY1", "9")
	m.AddComposite("LOC", []string{"147"}, []string{"0010204", "", "5"})

	for _, d := range reg.Validate(ix) {
		fmt.Println(d)
	}
	// Output:
	// error: message 1 (M1), segment 3, DTM+137: required segment is missing
}

// Clone copies a spec deeply, so a variant can change its key and nodes without
// touching the original.
func ExampleSpec_Clone() {
	base := bayplanSpec()

	variant := base.Clone()
	variant.Key.Association = edifact.Is("XX01")
	unit := &variant.Nodes[2]
	unit.Nodes = append(unit.Nodes, edifact.Node{Tag: "FTX", Qualifier: "AAA"})

	fmt.Println(base.Key, len(base.Nodes[2].Nodes), "nodes in SG2")
	fmt.Println(variant.Key, len(variant.Nodes[2].Nodes), "nodes in SG2")
	// Output:
	// BAPLIE:D:95B:UN:* 4 nodes in SG2
	// BAPLIE:D:95B:UN:XX01 5 nodes in SG2
}

// A nil Key field matches anything. Absent matches only an empty component,
// which tells a plain UN message from an industry variant of it.
func ExampleKey() {
	anyBaplie := edifact.Key{Type: edifact.Is("BAPLIE")}
	plain := edifact.Key{
		Type:        edifact.Is("BAPLIE"),
		Version:     edifact.Is("D"),
		Release:     edifact.Is("95B"),
		Agency:      edifact.Is("UN"),
		Association: edifact.Absent(),
	}

	fmt.Println(anyBaplie)
	fmt.Println(plain)
	// Output:
	// BAPLIE:*:*:*:*
	// BAPLIE:D:95B:UN:-
}

// Without RejectReadOn, Read collects diagnostics on the interchange. With it,
// Read returns the first one at or above the severity as an error.
func ExampleRejectReadOn() {
	miscounted := strings.Replace(bayplan, "UNT+14+M1'", "UNT+15+M1'", 1)

	ix, err := edifact.Read(strings.NewReader(miscounted))
	fmt.Println(len(ix.Diagnostics), "diagnostics,", err)

	_, err = edifact.Read(strings.NewReader(miscounted), edifact.RejectReadOn(edifact.Error))
	fmt.Println(err)
	// Output:
	// 1 diagnostics, <nil>
	// edifact: error: message 1 (M1), segment 14, UNT: count is 15, counted 14
}

// A registry's Write refuses errors by default. RejectWriteOn(Warning) makes it
// refuse warnings too, such as a segment the spec does not declare. A segment
// added at the end of the message falls inside its last segment group.
func ExampleRejectWriteOn() {
	reg := edifact.NewRegistry()
	if err := reg.Register(bayplanSpec()); err != nil {
		fmt.Println(err)
		return
	}
	ix, err := edifact.Read(strings.NewReader(bayplan))
	if err != nil {
		fmt.Println(err)
		return
	}
	ix.Message(1).Add("FTX", "AAI", "", "", "NOTE")

	fmt.Println(reg.Write(io.Discard, ix))
	fmt.Println(reg.Write(io.Discard, ix, edifact.RejectWriteOn(edifact.Warning)))
	// Output:
	// <nil>
	// edifact: warning: message 1 (M1), segment 14, /SG2/FTX+AAI: segment is not declared here
}

// MaxSegmentSize bounds how long one segment may be, not counting its
// terminator.
func ExampleMaxSegmentSize() {
	_, err := edifact.Read(strings.NewReader(bayplan), edifact.MaxSegmentSize(20))
	fmt.Println(errors.Is(err, edifact.ErrSegmentTooLong), err)
	// Output:
	// true edifact: segment is too long: over 20 bytes
}

// A file without a UNA that uses the syntax level B separators reads once
// DefaultDelimiters names them: IS1 between components, IS3 between elements
// and IS4 after each segment.
func ExampleDefaultDelimiters() {
	levelB := strings.NewReplacer(":", "\x1f", "+", "\x1d", "'", "\x1c").Replace(
		"UNB+UNOB:2+SENDER+RECIPIENT+260101:1200+REF1'" +
			"UNH+M1+BAPLIE:D:95B:UN'BGM++BAY1+9'UNT+3+M1'UNZ+1+REF1'")

	_, err := edifact.Read(strings.NewReader(levelB))
	fmt.Println(errors.Is(err, edifact.ErrUnsupportedSeparators))

	ix, err := edifact.Read(strings.NewReader(levelB), edifact.DefaultDelimiters(edifact.Delimiters{
		Component: 0x1f,
		Element:   0x1d,
		Segment:   0x1c,
	}))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(ix.Message(1).Find("BGM", "").Element(2))
	// Output:
	// true
	// BAY1
}

// A file without a UNA may use a comma as the decimal mark. Write declares a dot
// and writes text values as they are, so a number read as text keeps its comma
// until it is set again with SetFloat.
func ExampleSegment_SetFloat() {
	src := "UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'" +
		"UNH+M1+BAPLIE:D:95B:UN'MEA+WT++KGM:8500,5'UNT+3+M1'UNZ+1+REF1'"
	ix, err := edifact.Read(strings.NewReader(src))
	if err != nil {
		fmt.Println(err)
		return
	}
	mea := ix.Message(1).Find("MEA", "WT")

	v, err := mea.Float(3, 2)
	if err != nil {
		fmt.Println(err)
		return
	}
	if err := mea.SetFloat(3, 2, v, -1); err != nil {
		fmt.Println(err)
		return
	}
	if err := edifact.Write(os.Stdout, ix, edifact.SegmentSuffixLF()); err != nil {
		fmt.Println(err)
	}
	// Output:
	// UNA:+.? '
	// UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'
	// UNH+M1+BAPLIE:D:95B:UN'
	// MEA+WT++KGM:8500.5'
	// UNT+3+M1'
	// UNZ+1+REF1'
}

// A message joins a functional group when it is created. With functional
// groups, UNZ counts functional groups rather than messages.
func ExampleInterchange_AddFunctionalGroup() {
	ix := edifact.NewInterchange()
	ix.SetSender("SENDER", "")
	ix.SetRecipient("RECIPIENT", "")
	ix.SetDate("260101")
	ix.SetTime("1200")
	ix.SetControlRef("REF1")

	g := ix.AddFunctionalGroup("BAPLIE")
	g.SetSender("SENDER", "")
	g.SetRecipient("RECIPIENT", "")
	g.SetDate("260101")
	g.SetTime("1200")
	g.SetControlRef("G1")
	g.SetAgency("UN")
	g.SetVersion("D")
	g.SetRelease("95B")

	m := ix.AddMessageTo(g, edifact.Key{
		Type:    edifact.Is("BAPLIE"),
		Version: edifact.Is("D"),
		Release: edifact.Is("95B"),
		Agency:  edifact.Is("UN"),
	})
	m.SetRef("M1")
	m.Add("BGM", "", "BAY1", "9")

	if err := edifact.Write(os.Stdout, ix, edifact.SegmentSuffixLF()); err != nil {
		fmt.Println(err)
	}
	// Output:
	// UNA:+.? '
	// UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'
	// UNG+BAPLIE+SENDER+RECIPIENT+260101:1200+G1+UN+D:95B'
	// UNH+M1+BAPLIE:D:95B:UN'
	// BGM++BAY1+9'
	// UNT+3+M1'
	// UNE+1+G1'
	// UNZ+1+REF1'
}

// A tag can carry its nesting level and repetition number as components.
// Specs match the plain tag, and Index returns the numbers.
func ExampleSegment_Index() {
	src := "UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'" +
		"UNH+M1+TSTMSG:D:95B:UN'DDD:1:2+VALUE'UNT+3+M1'UNZ+1+REF1'"
	ix, err := edifact.Read(strings.NewReader(src))
	if err != nil {
		fmt.Println(err)
		return
	}
	s := ix.Message(1).Find("DDD", "")
	fmt.Println(s.Tag, s.Index(), s.Element(1))
	// Output:
	// DDD [1 2] VALUE
}

// SetCharset declares the syntax level. Write refuses a value outside it, here
// lower case in UNOA, and accepts it once the interchange declares UNOB.
func ExampleInterchange_SetCharset() {
	ix := edifact.NewInterchange()
	ix.SetSender("SENDER", "")
	ix.SetRecipient("RECIPIENT", "")
	ix.SetDate("260101")
	ix.SetTime("1200")
	ix.SetControlRef("REF1")
	m := ix.AddMessage(edifact.Key{Type: edifact.Is("TSTMSG"), Version: edifact.Is("D"), Release: edifact.Is("95B"), Agency: edifact.Is("UN")})
	m.SetRef("M1")
	m.Add("FTX", "AAA", "", "", "Handle with care")

	err := edifact.Write(io.Discard, ix)
	fmt.Println(errors.Is(err, edifact.ErrCharset), err)

	ix.SetCharset(edifact.UNOB)
	fmt.Println(edifact.Write(io.Discard, ix))
	c, _ := ix.Charset()
	fmt.Println(c)
	// Output:
	// true FTX element 4 component 1: edifact: character outside the charset: 'a' (U+0061) is not in UNOA
	// <nil>
	// UNOB
}

// FitCharset converts what it can into the charset. In UNOA that means
// accented letters lose their accents and lower case becomes upper case.
func ExampleFitCharset() {
	ix := edifact.NewInterchange()
	ix.SetSender("SENDER", "")
	ix.SetRecipient("RECIPIENT", "")
	ix.SetDate("260101")
	ix.SetTime("1200")
	ix.SetControlRef("REF1")
	m := ix.AddMessage(edifact.Key{Type: edifact.Is("TSTMSG"), Version: edifact.Is("D"), Release: edifact.Is("95B"), Agency: edifact.Is("UN")})
	m.SetRef("M1")
	m.Add("FTX", "AAA", "", "", "Café, Straße")

	var sb strings.Builder
	if err := edifact.Write(&sb, ix, edifact.FitCharset(), edifact.SegmentSuffixLF()); err != nil {
		fmt.Println(err)
		return
	}
	for _, line := range strings.Split(sb.String(), "\n") {
		if strings.HasPrefix(line, "FTX") {
			fmt.Println(line)
		}
	}
	// Output:
	// FTX+AAA+++CAFE, STRASSE'
}

// SetDateTime writes the date of preparation in the format the syntax version
// requires, and DateTime reads it back without a time zone.
func ExampleInterchange_SetDateTime() {
	when := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)

	ix := edifact.NewInterchange()
	ix.SetDateTime(when)
	fmt.Println(ix.Date(), ix.Time())

	if err := ix.SetSyntaxVersion(4); err != nil {
		fmt.Println(err)
		return
	}
	ix.SetDateTime(when)
	fmt.Println(ix.Date(), ix.Time())

	t, err := ix.DateTime()
	fmt.Println(t.Format("2006-01-02 15:04"), t.Location(), err)
	// Output:
	// 260102 1504
	// 20260102 1504
	// 2026-01-02 15:04 UNKNOWN <nil>
}
