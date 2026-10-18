package edifact

import (
	"strings"
	"testing"
)

func TestSegmentLenAndComponents(t *testing.T) {
	s := seg("FTX", []string{"AAA"}, nil, nil, []string{"ONE", "TWO"})

	if s.Len() != 4 {
		t.Errorf("Len = %d, want 4", s.Len())
	}
	if got := s.Components(4); len(got) != 2 || got[0] != "ONE" || got[1] != "TWO" {
		t.Errorf("Components(4) = %q", got)
	}
	if got := s.Components(2); len(got) != 0 {
		t.Errorf("Components(2) = %q, want none", got)
	}
	if s.Components(0) != nil || s.Components(5) != nil {
		t.Error("an absent element returned components")
	}

	var nilSeg *Segment
	if nilSeg.Len() != 0 || nilSeg.Components(1) != nil {
		t.Error("a nil segment reported content")
	}
}

func TestComponentsReturnsACopy(t *testing.T) {
	s := seg("FTX", []string{"AAA"})
	s.Components(1)[0] = "BBB"
	if got := s.Element(1); got != "AAA" {
		t.Errorf("editing the result reached the segment: %q", got)
	}
}

func TestSegmentGroupAccessorsReturnCopies(t *testing.T) {
	m := NewInterchange().AddMessage(Key{})
	g := m.AddGroup("SG1")
	s := g.Add("LOC", "147")
	g.AddGroup("SG2")

	g.Segments()[0] = nil
	g.Groups()[0] = nil
	if g.Find("LOC", "147") != s {
		t.Error("editing Segments reached the group")
	}
	if len(g.FindAllGroups("SG2")) != 1 {
		t.Error("editing Groups reached the group")
	}
}

func TestAddMessageToSetsTheGroup(t *testing.T) {
	ix := NewInterchange()
	g := ix.AddFunctionalGroup("TSTMSG")

	if m := ix.AddMessageTo(g, Key{Type: Is("TSTMSG")}); m.FunctionalGroup() != g {
		t.Error("AddMessageTo did not place the message in g")
	}
	if m := ix.AddMessage(Key{Type: Is("TSTMSG")}); m.FunctionalGroup() != nil {
		t.Error("AddMessage placed the message in a group")
	}
	if m := ix.AddMessageTo(nil, Key{Type: Is("TSTMSG")}); m.FunctionalGroup() != nil {
		t.Error("AddMessageTo(nil) placed the message in a group")
	}
}

func TestMessageAccessorsReturnCopies(t *testing.T) {
	m := NewInterchange().AddMessage(Key{})
	s := m.Add("LOC", "147")
	m.AddGroup("SG1")

	m.Segments()[0] = nil
	m.Groups()[0] = nil
	if m.Find("LOC", "147") != s {
		t.Error("editing Segments reached the message")
	}
	if len(m.FindAllGroups("SG1")) != 1 {
		t.Error("editing Groups reached the message")
	}
}

func TestInterchangeAccessorsReturnCopies(t *testing.T) {
	ix := NewInterchange()
	if ix.Groups() != nil {
		t.Error("Groups is not nil before any group is added")
	}
	g := ix.AddFunctionalGroup("TSTMSG")
	m := ix.AddMessageTo(g, Key{Type: Is("TSTMSG")})

	ix.Messages()[0] = nil
	ix.Groups()[0] = nil
	if ix.Messages()[0] != m {
		t.Error("editing Messages reached the interchange")
	}
	if ix.Groups()[0] != g {
		t.Error("editing Groups reached the interchange")
	}
}

func TestRepetitions(t *testing.T) {
	s := &Segment{Tag: "TST", elements: [][][]string{
		{{"A"}},
		nil,
		{{"B", "1"}, {"C", "2"}},
	}}

	for i, want := range []int{1, 0, 2, 0} {
		if got := s.Repetitions(i + 1); got != want {
			t.Errorf("Repetitions(%d) = %d, want %d", i+1, got, want)
		}
	}
	if got := s.Repetition(3, 2); len(got) != 2 || got[0] != "C" || got[1] != "2" {
		t.Errorf("Repetition(3, 2) = %q", got)
	}
	if s.Repetition(3, 0) != nil || s.Repetition(3, 3) != nil || s.Repetition(2, 1) != nil {
		t.Error("an absent repetition returned components")
	}
	if got := s.Component(3, 2); got != "1" {
		t.Errorf("Component(3, 2) = %q, want the first repetition's", got)
	}

	s.Repetition(3, 2)[0] = "X"
	if s.Repetition(3, 2)[0] != "C" {
		t.Error("editing the result reached the segment")
	}
}

func TestEmptyElementsHaveNoRepetitions(t *testing.T) {
	m := NewInterchange().AddMessage(Key{})
	s := m.AddComposite("TST", []string{"A"}, nil, []string{})
	if s.Len() != 3 || s.Repetitions(2) != 0 || s.Repetitions(3) != 0 {
		t.Errorf("Len %d, repetitions %d and %d", s.Len(), s.Repetitions(2), s.Repetitions(3))
	}

	s.Set(2, 2, "B")
	if s.Repetitions(2) != 1 || s.Component(2, 2) != "B" {
		t.Errorf("Set on an empty element: %d repetitions, %q", s.Repetitions(2), s.Component(2, 2))
	}
}

func TestAddCompositeCopiesItsInput(t *testing.T) {
	m := NewInterchange().AddMessage(Key{})
	comps := make([]string, 1, 4)
	comps[0] = "A"
	s := m.AddComposite("TST", comps)

	s.Set(1, 2, "B")
	if got := comps[:2][1]; got != "" {
		t.Errorf("Set wrote %q into the caller's array", got)
	}
	comps[0] = "Z"
	if got := s.Element(1); got != "A" {
		t.Errorf("editing the caller's slice reached the segment: %q", got)
	}
}

func tagsOf(segs []*Segment) string {
	var out []string
	for _, s := range segs {
		out = append(out, s.Tag+"+"+s.Qualifier())
	}
	return strings.Join(out, " ")
}

func TestSegmentGroupAddBuildsGroupsFrontToBack(t *testing.T) {
	m := NewInterchange().AddMessage(Key{})
	m.Add("BGM", "")
	u1 := m.AddGroup("SG2")
	u1.Add("LOC", "147")
	u1.AddGroup("SG3").Add("EQD", "CN")
	u2 := m.AddGroup("SG2")
	u2.Add("LOC", "147")
	u1.Add("MEA", "VGM")

	if got := tagsOf(m.Segments()); got != "BGM+ LOC+147 EQD+CN MEA+VGM LOC+147" {
		t.Errorf("message order = %s", got)
	}
	if got := tagsOf(u1.Segments()); got != "LOC+147 EQD+CN MEA+VGM" {
		t.Errorf("unit 1 = %s", got)
	}
	if got := tagsOf(u2.Segments()); got != "LOC+147" {
		t.Errorf("unit 2 = %s", got)
	}
}

func TestZeroValuesAreUsable(t *testing.T) {
	var ix Interchange
	ix.SetCharset(UNOA)
	ix.SetSyntaxVersion(2)
	ix.SetSender("S", "")
	ix.SetRecipient("R", "")
	ix.SetDate("260101")
	ix.SetTime("1200")
	ix.SetControlRef("C1")
	m := ix.AddMessage(Key{Type: Is("TSTMSG"), Version: Is("D"), Release: Is("95B"), Agency: Is("UN")})
	m.SetRef("M1")
	m.Add("BGM", "", "REF1")

	var sb strings.Builder
	if err := Write(&sb, &ix); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sb.String(), "UNA:+.? 'UNB+UNOA:2+S+R+260101:1200+C1'") {
		t.Errorf("output: %q", sb.String())
	}

	var g FunctionalGroup
	g.SetControlRef("G1")
	if g.ControlRef() != "G1" || g.Header().Tag != "UNG" {
		t.Errorf("functional group: %q %q", g.ControlRef(), g.Header().Tag)
	}
	var msg Message
	msg.SetRef("M2")
	if msg.Ref() != "M2" || msg.Header().Tag != "UNH" {
		t.Errorf("message: %q %q", msg.Ref(), msg.Header().Tag)
	}
}

func TestMessageAndFindGroup(t *testing.T) {
	ix := NewInterchange()
	m := ix.AddMessage(Key{})
	a := m.AddGroup("SG2")
	a.Add("LOC", "147", "A")
	b := m.AddGroup("SG2")
	b.Add("LOC", "147", "B")
	nested := b.AddGroup("SG3")
	nested.Add("EQD", "CN")

	if ix.Message(1) != m || ix.Message(0) != nil || ix.Message(2) != nil {
		t.Error("Message does not count from 1")
	}
	if m.FindGroup("SG2") != a || len(m.FindAllGroups("SG2")) != 2 || m.FindGroup("SG9") != nil {
		t.Error("Message.FindGroup or FindAllGroups")
	}
	if b.FindGroup("SG3") != nested || len(b.FindAllGroups("SG3")) != 1 || a.FindGroup("SG3") != nil {
		t.Error("SegmentGroup.FindGroup or FindAllGroups")
	}

	// A chain through anything missing gives empty results rather than a panic.
	var none *Interchange
	if got := ix.Message(5).FindGroup("SG2").FindGroup("SG3").Find("EQD", "CN").Element(2); got != "" {
		t.Errorf("chain through a missing message = %q", got)
	}
	if none.Message(1) != nil {
		t.Error("nil interchange returned a message")
	}
	var nm *Message
	var ng *SegmentGroup
	if nm.FindAll("X", "") != nil || nm.Segments() != nil || nm.Groups() != nil || nm.FindAllGroups("X") != nil ||
		ng.FindAll("X", "") != nil || ng.Segments() != nil || ng.Groups() != nil || ng.FindAllGroups("X") != nil {
		t.Error("nil receivers returned something")
	}
}

func TestEarlierSegmentGroupFilledLater(t *testing.T) {
	m := NewInterchange().AddMessage(Key{})
	m.Add("BGM", "")
	g1 := m.AddGroup("SG2")
	g2 := m.AddGroup("SG2")
	g2.Add("LOC", "147", "B")
	g1.Add("LOC", "147", "A")
	if got := tagsOf(m.Segments()); got != "BGM+ LOC+147 LOC+147" || m.Segments()[1].Element(2) != "A" {
		t.Errorf("message order = %s, second segment %q", got, m.Segments()[1].Element(2))
	}

	unit := m.AddGroup("SG2")
	unit.Add("LOC", "147", "C")
	n1 := unit.AddGroup("SG3")
	n2 := unit.AddGroup("SG3")
	n2.Add("EQD", "CN", "Y")
	n1.Add("EQD", "CN", "X")
	var got []string
	for _, s := range unit.Segments() {
		got = append(got, s.Element(2))
	}
	if strings.Join(got, " ") != "C X Y" {
		t.Errorf("unit order = %v, want C X Y", got)
	}
	if n1.Segments()[0].Element(2) != "X" || n2.Segments()[0].Element(2) != "Y" {
		t.Error("nested views hold the wrong segments")
	}
}

func TestNilMessageAndFunctionalGroupReads(t *testing.T) {
	var m *Message
	var g *FunctionalGroup
	if m.Ref() != "" || m.FunctionalGroup() != nil || m.Header() != nil || m.Key().Type != nil {
		t.Error("nil message reads")
	}
	if g.MessageType() != "" || g.Sender() != "" || g.Recipient() != "" || g.ControlRef() != "" ||
		g.Agency() != "" || g.Version() != "" || g.Release() != "" || g.Date() != "" || g.Time() != "" ||
		g.Header() != nil {
		t.Error("nil functional group reads")
	}
	ix := NewInterchange()
	if ix.Message(9).Ref() != "" || ix.Message(9).FunctionalGroup().ControlRef() != "" {
		t.Error("chain through a missing message")
	}
}
