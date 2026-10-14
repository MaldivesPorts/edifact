package edifact

import (
	"strings"
	"testing"
)

// containerSpec mirrors the shape of a stowage message: a header, then a group
// repeating per unit, with a nested group inside it.
func containerSpec() Spec {
	return Spec{
		Key: Key{Type: Is("TSTMSG")},
		Nodes: []Node{
			{Tag: "BGM", Required: true},
			{Tag: "DTM", Qualifier: "137", Required: true},
			{Group: "SG2", Max: Unbounded, Nodes: []Node{
				{Tag: "LOC", Qualifier: "147", Required: true},
				{Tag: "MEA", Qualifier: "VGM"},
				{Tag: "LOC", Qualifier: "9"},
				{Tag: "LOC", Qualifier: "11"},
				{Group: "SG3", Nodes: []Node{
					{Tag: "EQD", Qualifier: "CN", Required: true},
					{Tag: "NAD", Qualifier: "CA"},
				}},
			}},
		},
	}
}

func buildMessage(t *testing.T, segs ...[]string) *Message {
	t.Helper()
	ix := NewInterchange()
	m := ix.AddMessage(Key{Type: Is("TSTMSG")})
	for _, s := range segs {
		m.Add(s[0], s[1:]...)
	}
	return m
}

func errorsOnly(diags []Diagnostic) []Diagnostic {
	var out []Diagnostic
	for _, d := range diags {
		if d.Severity == Error {
			out = append(out, d)
		}
	}
	return out
}

func TestValidateSplitsGroups(t *testing.T) {
	spec := containerSpec()
	m := buildMessage(t,
		[]string{"BGM", "", "REF001", "9"},
		[]string{"DTM", "137"},
		// first unit
		[]string{"LOC", "147", "0010204"},
		[]string{"MEA", "VGM"},
		[]string{"LOC", "9"},
		[]string{"LOC", "11"},
		[]string{"EQD", "CN", "AAAU0000001"},
		[]string{"NAD", "CA"},
		// second unit
		[]string{"LOC", "147", "0010206"},
		[]string{"MEA", "VGM"},
		[]string{"EQD", "CN", "AAAU0000002"},
	)

	diags := validateMessage(m, &spec)
	if got := errorsOnly(diags); len(got) != 0 {
		t.Fatalf("unexpected errors: %v", got)
	}

	units := m.FindAllGroups("SG2")
	if len(units) != 2 {
		t.Fatalf("got %d units, want 2", len(units))
	}
	if got := len(units[0].Segments()); got != 6 {
		t.Errorf("first unit holds %d segments, want 6", got)
	}
	if got := len(units[1].Segments()); got != 3 {
		t.Errorf("second unit holds %d segments, want 3", got)
	}
	if got := units[0].Find("EQD", "CN").Element(2); got != "AAAU0000001" {
		t.Errorf("first unit EQD = %q", got)
	}
	if got := units[1].Find("EQD", "CN").Element(2); got != "AAAU0000002" {
		t.Errorf("second unit EQD = %q", got)
	}

	// The nested group is built too, and points at the same segments.
	eq := units[0].FindAllGroups("SG3")
	if len(eq) != 1 || len(eq[0].Segments()) != 2 {
		t.Fatalf("nested group = %v", eq)
	}
	if eq[0].Segments()[0] != units[0].Segments()[4] {
		t.Error("nested group holds a copy rather than the message's segment")
	}
}

func TestValidateRepeatedTagInsideOneGroup(t *testing.T) {
	// LOC appears three times in a unit with different qualifiers. Only the
	// LOC+147 starts a new one.
	spec := containerSpec()
	m := buildMessage(t,
		[]string{"BGM"}, []string{"DTM", "137"},
		[]string{"LOC", "147"}, []string{"LOC", "9"}, []string{"LOC", "11"},
		[]string{"LOC", "147"}, []string{"LOC", "9"},
	)
	validateMessage(m, &spec)

	units := m.FindAllGroups("SG2")
	if len(units) != 2 {
		t.Fatalf("got %d units, want 2", len(units))
	}
	if len(units[0].Segments()) != 3 || len(units[1].Segments()) != 2 {
		t.Errorf("units hold %d and %d segments, want 3 and 2",
			len(units[0].Segments()), len(units[1].Segments()))
	}
}

func TestValidateUnexpectedSegmentIsAWarning(t *testing.T) {
	spec := containerSpec()
	m := buildMessage(t,
		[]string{"BGM"}, []string{"DTM", "137"},
		[]string{"FTX", "AAY"}, // declared nowhere
		[]string{"LOC", "147"}, []string{"EQD", "CN"},
	)

	diags := validateMessage(m, &spec)
	if got := errorsOnly(diags); len(got) != 0 {
		t.Fatalf("unexpected errors: %v", got)
	}
	if len(diags) != 1 || diags[0].Severity != Warning || diags[0].Tag != "FTX" {
		t.Fatalf("diags = %v, want one FTX warning", diags)
	}

	// Recovery: the units after the stray segment still parse.
	if got := len(m.FindAllGroups("SG2")); got != 1 {
		t.Errorf("got %d units after the stray segment, want 1", got)
	}
}

func TestValidateMissingRequiredSegment(t *testing.T) {
	spec := containerSpec()
	m := buildMessage(t,
		[]string{"BGM"},
		[]string{"LOC", "147"}, []string{"MEA", "VGM"},
	)

	errs := errorsOnly(validateMessage(m, &spec))
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if errs[0].Tag != "DTM" || !strings.Contains(errs[0].Message, "required segment") {
		t.Errorf("err = %v, want DTM missing", errs[0])
	}
}

func TestOptionalGroupAbsentDoesNotFireItsRequiredChildren(t *testing.T) {
	// SG3 is optional and holds a required EQD. With no EQD in the message the
	// group never starts, so nothing inside it is expected.
	spec := containerSpec()
	m := buildMessage(t,
		[]string{"BGM"}, []string{"DTM", "137"},
		[]string{"LOC", "147"}, []string{"MEA", "VGM"},
	)

	if errs := errorsOnly(validateMessage(m, &spec)); len(errs) != 0 {
		t.Fatalf("errors from an absent optional group: %v", errs)
	}
	if got := len(m.FindAllGroups("SG2")); got != 1 {
		t.Errorf("got %d units, want 1", got)
	}
}

func TestValidateMissingRequiredGroup(t *testing.T) {
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
		{Group: "SG1", Required: true, Nodes: []Node{
			{Tag: "TDT", Qualifier: "20", Required: true},
		}},
	}}
	m := buildMessage(t, []string{"BGM"})

	errs := errorsOnly(validateMessage(m, &spec))
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if errs[0].Tag != "SG1" || !strings.Contains(errs[0].Message, "required segment group") {
		t.Errorf("err = %v, want SG1 missing", errs[0])
	}
}

func TestValidateBeyondMaxIsUnconsumed(t *testing.T) {
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
		{Tag: "RFF", Qualifier: "BM", Max: 2},
	}}
	m := buildMessage(t,
		[]string{"BGM"},
		[]string{"RFF", "BM"}, []string{"RFF", "BM"}, []string{"RFF", "BM"},
	)

	diags := validateMessage(m, &spec)
	if len(errorsOnly(diags)) != 0 {
		t.Fatalf("errors: %v", diags)
	}
	if len(diags) != 1 || diags[0].Severity != Warning || diags[0].SegmentIdx != 5 {
		t.Fatalf("diags = %v, want one warning on segment 5", diags)
	}
}

func TestBeyondMaxStartsTheNextRepetition(t *testing.T) {
	// A second DGS belongs to a new SG4 rather than overflowing the first.
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
		{Group: "SG4", Max: Unbounded, Nodes: []Node{
			{Tag: "DGS", Qualifier: "IMD", Required: true},
			{Tag: "FTX"},
		}},
	}}
	m := buildMessage(t,
		[]string{"BGM"},
		[]string{"DGS", "IMD"}, []string{"FTX"},
		[]string{"DGS", "IMD"},
		[]string{"DGS", "IMD"},
	)

	diags := validateMessage(m, &spec)
	if len(diags) != 0 {
		t.Fatalf("diags = %v", diags)
	}
	if got := len(m.FindAllGroups("SG4")); got != 3 {
		t.Errorf("got %d repetitions, want 3", got)
	}
}

func TestRegistryValidateUnknownKey(t *testing.T) {
	r := NewRegistry()
	ix := NewInterchange()
	ix.AddMessage(Key{Type: Is("NOSUCH"), Version: Is("D")})

	diags := r.Validate(ix)
	if len(diags) != 1 || diags[0].Severity != Error {
		t.Fatalf("diags = %v", diags)
	}
	if !strings.Contains(diags[0].Message, "no spec matches") {
		t.Errorf("message = %q", diags[0].Message)
	}
}

func TestRegistryValidateWalksEveryMessage(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(containerSpec()); err != nil {
		t.Fatal(err)
	}
	ix := NewInterchange()
	for i := 0; i < 2; i++ {
		m := ix.AddMessage(Key{Type: Is("TSTMSG")})
		m.Add("BGM")
		m.Add("DTM", "137")
		m.Add("LOC", "147")
		m.Add("EQD", "CN")
	}

	if diags := errorsOnly(r.Validate(ix)); len(diags) != 0 {
		t.Fatalf("errors: %v", diags)
	}
	for i, m := range ix.Messages() {
		if got := len(m.FindAllGroups("SG2")); got != 1 {
			t.Errorf("message %d has %d units, want 1", i, got)
		}
	}
}

// handlingSpec nests three levels, as stowage instructions do: a handling block
// holds units, and each unit holds an equipment group.
func handlingSpec() Spec {
	return Spec{
		Key: Key{Type: Is("TSTMSG")},
		Nodes: []Node{
			{Tag: "BGM", Required: true},
			{Group: "SG2", Max: Unbounded, Nodes: []Node{
				{Tag: "HAN", Required: true},
				{Group: "SG3", Max: Unbounded, Nodes: []Node{
					{Tag: "LOC", Qualifier: "147", Required: true},
					{Tag: "MEA", Required: true},
					{Group: "SG4", Nodes: []Node{
						{Tag: "EQD", Qualifier: "CN", Required: true},
						{Tag: "NAD", Qualifier: "CF"},
					}},
				}},
			}},
		},
	}
}

func TestStraySegmentInsideNestedGroup(t *testing.T) {
	spec := handlingSpec()
	m := buildMessage(t,
		[]string{"BGM"},
		[]string{"HAN", "LOA"},
		[]string{"LOC", "147", "0010204"}, []string{"MEA", "WT"}, []string{"EQD", "CN"},
		[]string{"EQN", "1"}, // undeclared, inside the equipment group
		[]string{"NAD", "CF"},
		[]string{"LOC", "147", "0010206"}, []string{"MEA", "WT"}, []string{"EQD", "CN"}, []string{"NAD", "CF"},
		[]string{"LOC", "147", "0010208"}, []string{"MEA", "WT"}, []string{"EQD", "CN"}, []string{"NAD", "CF"},
		[]string{"HAN", "DIS"},
		[]string{"LOC", "147", "0020402"}, []string{"MEA", "WT"}, []string{"EQD", "CN"},
	)

	diags := validateMessage(m, &spec)
	if len(diags) != 1 || diags[0].Tag != "EQN" || diags[0].Severity != Warning {
		t.Fatalf("diags = %v, want one EQN warning", diags)
	}

	blocks := m.FindAllGroups("SG2")
	if len(blocks) != 2 {
		t.Fatalf("got %d handling blocks, want 2", len(blocks))
	}
	if got := len(blocks[0].FindAllGroups("SG3")); got != 3 {
		t.Errorf("first block holds %d units, want 3", got)
	}
	if got := len(blocks[1].FindAllGroups("SG3")); got != 1 {
		t.Errorf("second block holds %d units, want 1", got)
	}
	// The stray is stepped over, and the segment after it stays in its group.
	if blocks[0].FindGroup("SG3").FindGroup("SG4").Find("NAD", "CF") == nil {
		t.Error("NAD after the stray segment left its equipment group")
	}
}

func TestStraySegmentBeforeARequiredChild(t *testing.T) {
	spec := handlingSpec()
	m := buildMessage(t,
		[]string{"BGM"},
		[]string{"HAN", "LOA"},
		[]string{"LOC", "147", "0010204"},
		[]string{"FTX", "AAA"}, // undeclared, between LOC+147 and the required MEA
		[]string{"MEA", "WT"}, []string{"EQD", "CN"},
		[]string{"LOC", "147", "0010206"}, []string{"MEA", "WT"}, []string{"EQD", "CN"},
	)

	diags := validateMessage(m, &spec)
	if len(diags) != 1 || diags[0].Tag != "FTX" {
		t.Fatalf("diags = %v, want one FTX warning and no missing-segment errors", diags)
	}
	if got := len(m.FindGroup("SG2").FindAllGroups("SG3")); got != 2 {
		t.Errorf("got %d units, want 2", got)
	}
}

func TestSegmentGroupAddLeavesTheMessageAlone(t *testing.T) {
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Group: "SG2", Max: Unbounded, Nodes: []Node{
			{Tag: "LOC", Qualifier: "147", Required: true},
			{Tag: "MEA"},
			{Tag: "FTX"},
		}},
	}}

	// Whether the old slices had spare room depended on the unit count, so try
	// several.
	for units := 1; units <= 8; units++ {
		var segs [][]string
		for i := 0; i < units; i++ {
			segs = append(segs, []string{"LOC", "147", "C" + string(rune('A'+i))}, []string{"MEA", "VGM"})
		}
		m := buildMessage(t, segs...)
		validateMessage(m, &spec)

		before := m.Segments()
		views := make([][]*Segment, len(m.Groups()))
		for i, g := range m.Groups() {
			views[i] = append([]*Segment(nil), g.Segments()...)
		}

		s := m.FindGroup("SG2").Add("FTX", "AAA", "", "", "NOTE")

		if got := len(m.Segments()); got != len(before)+1 {
			t.Fatalf("%d units: message holds %d segments, want %d", units, got, len(before)+1)
		}
		for i := 1; i < len(m.Groups()); i++ {
			for j, seg := range m.Groups()[i].Segments() {
				if seg != views[i][j] {
					t.Fatalf("%d units: unit %d's view was overwritten at %d", units, i+1, j)
				}
			}
		}
		if got := m.Groups()[0].Find("FTX", "AAA"); got != s {
			t.Errorf("%d units: unit 1 does not hold the added segment", units)
		}
	}
}

func TestSegmentGroupAddToAnEarlierGroup(t *testing.T) {
	spec := containerSpec()
	spec.Nodes[2].Nodes = append(spec.Nodes[2].Nodes, Node{Tag: "FTX", Qualifier: "AAA"})
	m := buildMessage(t,
		[]string{"BGM"}, []string{"DTM", "137"},
		[]string{"LOC", "147", "C1"}, []string{"MEA", "VGM"},
		[]string{"EQD", "CN", "XXXU0000001"},
		[]string{"LOC", "147", "C2"}, []string{"MEA", "VGM"},
	)
	validateMessage(m, &spec)
	units := m.FindAllGroups("SG2")
	unit2Before := tagsOf(units[1].Segments())

	units[0].Add("FTX", "AAA", "", "", "NOTE")
	units[0].FindGroup("SG3").Add("NAD", "CA")

	want := "BGM+ DTM+137 LOC+147 MEA+VGM EQD+CN NAD+CA FTX+AAA LOC+147 MEA+VGM"
	if got := tagsOf(m.Segments()); got != want {
		t.Errorf("message order:\n got %s\nwant %s", got, want)
	}
	if got := tagsOf(units[0].Segments()); got != "LOC+147 MEA+VGM EQD+CN NAD+CA FTX+AAA" {
		t.Errorf("unit 1 = %s", got)
	}
	if got := tagsOf(units[0].FindGroup("SG3").Segments()); got != "EQD+CN NAD+CA" {
		t.Errorf("unit 1 SG3 = %s", got)
	}
	if got := tagsOf(units[1].Segments()); got != unit2Before {
		t.Errorf("unit 2 changed: %s", got)
	}

	// Re-validating finds the same structure.
	if diags := validateMessage(m, &spec); len(diags) != 0 {
		t.Fatalf("diagnostics = %v", diags)
	}
	if got := tagsOf(m.FindGroup("SG2").Segments()); got != "LOC+147 MEA+VGM EQD+CN NAD+CA FTX+AAA" {
		t.Errorf("unit 1 after re-validation = %s", got)
	}
}

func TestNewSegmentGroupGoesAfterItsEarlierRepetition(t *testing.T) {
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
		{Group: "SG2", Max: Unbounded, Nodes: []Node{{Tag: "LOC", Qualifier: "147", Required: true}}},
		{Tag: "CNT"},
	}}
	m := buildMessage(t, []string{"BGM"}, []string{"LOC", "147", "A"}, []string{"CNT", "1"})
	validateMessage(m, &spec)

	m.AddGroup("SG2").Add("LOC", "147", "B")

	if got := tagsOf(m.Segments()); got != "BGM+ LOC+147 LOC+147 CNT+1" {
		t.Errorf("message order = %s", got)
	}
	if diags := validateMessage(m, &spec); len(diags) != 0 {
		t.Errorf("diagnostics = %v", diags)
	}
}

func TestNewNestedSegmentGroupGoesBeforeLaterSiblings(t *testing.T) {
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
		{Group: "SG2", Max: Unbounded, Nodes: []Node{
			{Tag: "LOC", Qualifier: "147", Required: true},
			{Group: "SG3", Max: 3, Nodes: []Node{{Tag: "EQD", Required: true}}},
			{Group: "SG4", Max: 9, Nodes: []Node{{Tag: "DGS", Required: true}}},
		}},
	}}
	m := buildMessage(t, []string{"BGM"}, []string{"LOC", "147"}, []string{"EQD", "CN", "A"}, []string{"DGS", "IMD"})
	validateMessage(m, &spec)

	unit := m.FindGroup("SG2")
	unit.AddGroup("SG3").Add("EQD", "CN", "B")

	if got := tagsOf(m.Segments()); got != "BGM+ LOC+147 EQD+CN EQD+CN DGS+IMD" {
		t.Errorf("message order = %s", got)
	}
	if got := tagsOf(unit.Segments()); got != "LOC+147 EQD+CN EQD+CN DGS+IMD" {
		t.Errorf("unit order = %s", got)
	}
	if diags := validateMessage(m, &spec); len(diags) != 0 {
		t.Errorf("diagnostics = %v", diags)
	}
}

func TestFirstSegmentGroupGoesAfterTheMessagesOwnSegments(t *testing.T) {
	m := NewInterchange().AddMessage(Key{})
	m.Add("BGM", "")
	m.Add("DTM", "137")
	m.AddGroup("SG2").Add("LOC", "147")
	if got := tagsOf(m.Segments()); got != "BGM+ DTM+137 LOC+147" {
		t.Errorf("message order = %s", got)
	}
}

func TestRepetitionsBeyondMax(t *testing.T) {
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
		{Tag: "RFF", Qualifier: "BM", Max: 2},
		{Group: "SG1", Max: 2, Nodes: []Node{
			{Tag: "LOC", Qualifier: "147", Required: true},
			{Tag: "MEA"},
		}},
	}}
	m := buildMessage(t,
		[]string{"BGM"}, []string{"BGM"},
		[]string{"RFF", "BM"}, []string{"RFF", "BM"}, []string{"RFF", "BM"},
		[]string{"LOC", "147"}, []string{"MEA"},
		[]string{"LOC", "147"},
		[]string{"LOC", "147"}, []string{"MEA"},
	)

	var got []string
	for _, d := range validateMessage(m, &spec) {
		got = append(got, d.String())
	}
	want := []string{
		"warning: segment 3, BGM: repeated more than once",
		"warning: segment 6, RFF+BM: repeated more than 2 times",
		"warning: segment 10, SG1: repeated more than 2 times",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diagnostics:\n got %s\nwant %s", strings.Join(got, "\n    "), strings.Join(want, "\n     "))
	}
	if n := len(m.FindAllGroups("SG1")); n != 2 {
		t.Errorf("%d SG1 repetitions built, want 2", n)
	}
}

func TestQualifiersAcceptAnyListedValueInAnyOrder(t *testing.T) {
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
		{Group: "SG2", Max: Unbounded, Nodes: []Node{
			{Tag: "LOC", Qualifier: "147", Required: true},
			{Tag: "LOC", Qualifiers: []string{"9", "11", "13"}, Max: 9},
			{Group: "SG3", Max: 9, Nodes: []Node{
				{Tag: "EQD", Qualifiers: []string{"CN", "BB"}, Required: true},
			}},
		}},
	}}
	m := buildMessage(t,
		[]string{"BGM"},
		[]string{"LOC", "147", "A"}, []string{"LOC", "11"}, []string{"LOC", "9"}, []string{"LOC", "13"},
		[]string{"EQD", "BB"},
		[]string{"LOC", "147", "B"}, []string{"LOC", "9"},
		[]string{"EQD", "CN"}, []string{"EQD", "CN"},
	)
	if diags := validateMessage(m, &spec); len(diags) != 0 {
		t.Fatalf("diagnostics = %v", diags)
	}
	units := m.FindAllGroups("SG2")
	if len(units) != 2 || len(units[0].FindAll("LOC", "")) != 4 || len(units[1].FindAllGroups("SG3")) != 2 {
		t.Errorf("units built wrongly: %d units", len(units))
	}

	m = buildMessage(t, []string{"BGM"}, []string{"LOC", "147"}, []string{"LOC", "76"}, []string{"EQD", "ZZ"})
	var got []string
	for _, d := range validateMessage(m, &spec) {
		got = append(got, d.String())
	}
	want := []string{
		"warning: segment 4, /SG2/LOC+76: segment is not declared here",
		"warning: segment 5, /SG2/EQD+ZZ: segment is not declared here",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diagnostics:\n got %s\nwant %s", strings.Join(got, "\n    "), strings.Join(want, "\n     "))
	}
}

func TestMissingNodeNamesEveryQualifier(t *testing.T) {
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "BGM", Required: true},
		{Tag: "EQD", Qualifiers: []string{"CN", "BB"}, Required: true},
	}}
	diags := validateMessage(buildMessage(t, []string{"BGM"}), &spec)
	if len(diags) != 1 || diags[0].Qualifier != "CN/BB" {
		t.Errorf("diagnostics = %v", diags)
	}
}

func TestSameSegmentAtTwoPositions(t *testing.T) {
	spec := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{
		{Tag: "TDT", Required: true},
		{Tag: "RFF", Qualifier: "VON"},
		{Tag: "LOC", Qualifier: "5"},
		{Tag: "DTM", Max: 9},
		{Tag: "RFF", Qualifier: "VON"},
	}}
	reg := NewRegistry()
	if err := reg.Register(spec); err != nil {
		t.Fatal(err)
	}
	for _, segs := range [][][]string{
		{{"TDT"}, {"RFF", "VON"}, {"LOC", "5"}, {"DTM", "133"}},
		{{"TDT"}, {"LOC", "5"}, {"DTM", "133"}, {"RFF", "VON"}},
	} {
		m := buildMessage(t, segs...)
		if diags := validateMessage(m, &spec); len(diags) != 0 {
			t.Errorf("%v: diagnostics = %v", segs, diags)
		}
	}
}
