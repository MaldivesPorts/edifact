package edifact

import (
	"strings"
	"testing"
)

func fullKey(assoc string) Key {
	return Key{Type: Is("TSTMSG"), Version: Is("D"), Release: Is("95B"), Agency: Is("UN"), Association: Is(assoc)}
}

func baseSpec() Spec {
	return Spec{
		Key: Key{Type: Is("TSTMSG"), Version: Is("D"), Release: Is("95B"), Agency: Is("UN"), Association: Is("AAA01")},
		Nodes: []Node{
			{Tag: "BGM", Required: true},
			{Tag: "DTM", Qualifier: "137", Required: true},
			{Group: "SG1", Max: Unbounded, Nodes: []Node{
				{Tag: "LOC", Qualifier: "9", Required: true},
				{Tag: "LOC", Qualifier: "11"},
			}},
		},
	}
}

func TestRegisterClonesDefensively(t *testing.T) {
	r := NewRegistry()
	s := baseSpec()
	if err := r.Register(s); err != nil {
		t.Fatal(err)
	}

	// Mutating the caller's copy, including through the shared backing array.
	s.Nodes[0].Required = false
	s.Nodes[2].Nodes[0].Qualifier = "CHANGED"

	got, ok := r.Lookup(fullKey("AAA01"))
	if !ok {
		t.Fatal("not found")
	}
	if !got.Nodes[0].Required {
		t.Error("registry saw a mutation of the caller's copy")
	}
	if got.Nodes[2].Nodes[0].Qualifier != "9" {
		t.Errorf("nested node mutated: %q", got.Nodes[2].Nodes[0].Qualifier)
	}
}

func TestRekeyDoesNotDisturbTheOriginal(t *testing.T) {
	r := NewRegistry()
	base := baseSpec()

	for _, assoc := range []string{"AAA01", "AAA02"} {
		s := base.Clone()
		s.Key.Association = Is(assoc)
		s.Nodes = append(s.Nodes, Node{Tag: "FTX", Qualifier: "AAY"})
		if err := r.Register(s); err != nil {
			t.Fatal(err)
		}
	}

	if len(base.Nodes) != 3 {
		t.Errorf("base spec grew to %d nodes", len(base.Nodes))
	}
	for _, assoc := range []string{"AAA01", "AAA02"} {
		got, ok := r.Lookup(fullKey(assoc))
		if !ok {
			t.Fatalf("%s not found", assoc)
		}
		if len(got.Nodes) != 4 {
			t.Errorf("%s has %d nodes, want 4", assoc, len(got.Nodes))
		}
	}
}

func TestLookupPrefersSpecificity(t *testing.T) {
	r := NewRegistry()
	generic := baseSpec()
	generic.Key = Key{Type: Is("TSTMSG")}
	specific := baseSpec()

	if err := r.Load([]Spec{generic, specific}); err != nil {
		t.Fatal(err)
	}

	full := fullKey("AAA01")
	got, ok := r.Lookup(full)
	if !ok || *got.Key.Association != "AAA01" {
		t.Errorf("got %s, want the specific spec", got.Key)
	}

	other := fullKey("ZZZ99")
	got, ok = r.Lookup(other)
	if !ok || got.Key.Association != nil {
		t.Errorf("got %s, want the catch-all", got.Key)
	}

	if _, ok := r.Lookup(Key{Type: Is("OTHER")}); ok {
		t.Error("matched an unregistered message type")
	}
}

func TestLookupLaterRegistrationOverrides(t *testing.T) {
	r := NewRegistry()
	first := baseSpec()
	second := baseSpec()
	second.Nodes[0].Tag = "BGM"
	second.Nodes = append(second.Nodes, Node{Tag: "RFF", Qualifier: "ZZZ"})

	if err := r.Load([]Spec{first, second}); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Lookup(first.Key)
	if len(got.Nodes) != 4 {
		t.Errorf("got %d nodes, want the later registration's 4", len(got.Nodes))
	}
}

func TestSpecValidate(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		want string
	}{
		{"no type", Spec{Nodes: []Node{{Tag: "BGM"}}}, "no message type"},
		{"tag and group", Spec{Key: Key{Type: Is("T")}, Nodes: []Node{{Tag: "BGM", Group: "SG1"}}}, "both"},
		{"neither", Spec{Key: Key{Type: Is("T")}, Nodes: []Node{{Required: true}}}, "neither"},
		{"segment with children", Spec{Key: Key{Type: Is("T")}, Nodes: []Node{
			{Tag: "BGM", Nodes: []Node{{Tag: "DTM"}}}}}, "cannot hold nodes"},
		{"empty group", Spec{Key: Key{Type: Is("T")}, Nodes: []Node{{Group: "SG1"}}}, "holds no nodes"},
		{"group with qualifier", Spec{Key: Key{Type: Is("T")}, Nodes: []Node{
			{Group: "SG1", Qualifier: "9", Nodes: []Node{{Tag: "LOC"}}}}}, "cannot hold a qualifier"},
		{"duplicate siblings", Spec{Key: Key{Type: Is("T")}, Nodes: []Node{
			{Tag: "LOC", Qualifier: "9"}, {Tag: "LOC", Qualifier: "9"}}}, "declared twice in a row"},
		{"bad max", Spec{Key: Key{Type: Is("T")}, Nodes: []Node{{Tag: "BGM", Max: -7}}}, "Max is -7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.spec.validate()
			if err == nil {
				t.Fatalf("no error, want one mentioning %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %q, want it to mention %q", err, c.want)
			}
		})
	}

	if err := baseSpec().validate(); err != nil {
		t.Errorf("valid spec rejected: %v", err)
	}
	if err := NewRegistry().Register(Spec{Key: Key{Type: Is("T")}, Nodes: []Node{{Group: "SG1"}}}); err == nil {
		t.Error("Register accepted an invalid spec")
	}
}

func TestNodeDefaults(t *testing.T) {
	if got := (Node{Tag: "BGM"}).repeats(); got != 1 {
		t.Errorf("zero Max repeats %d, want 1", got)
	}
	if got := (Node{Tag: "LOC", Max: Unbounded}).repeats(); got != Unbounded {
		t.Errorf("Unbounded repeats %d", got)
	}
	if (Node{Group: "SG1"}).IsSegment() {
		t.Error("a group reported itself a segment")
	}
}

func TestAbsentDistinguishedFromWildcard(t *testing.T) {
	// The case a wildcard blank got wrong: a spec meant for messages carrying no
	// association code, competing with one that pins the association.
	r := NewRegistry()

	noAssoc := baseSpec()
	noAssoc.Key = Key{Type: Is("TSTMSG"), Version: Is("D"), Release: Is("95B"), Agency: Is("UN"), Association: Absent()}
	byAssoc := baseSpec()
	byAssoc.Key = Key{Type: Is("TSTMSG"), Association: Is("AAA02")}

	if err := r.Load([]Spec{noAssoc, byAssoc}); err != nil {
		t.Fatal(err)
	}

	got, ok := r.Lookup(fullKey("AAA02"))
	if !ok || got.Key.Association == nil || *got.Key.Association != "AAA02" {
		t.Errorf("populated association resolved to %s, want the AAA02 spec", got.Key)
	}

	blank := fullKey("")
	got, ok = r.Lookup(blank)
	if !ok || got.Key.Agency == nil {
		t.Errorf("blank association resolved to %s, want the Absent spec", got.Key)
	}
}

func TestWildcardStillMatchesAnything(t *testing.T) {
	r := NewRegistry()
	wild := baseSpec()
	wild.Key = Key{Type: Is("TSTMSG")} // every other field nil
	if err := r.Register(wild); err != nil {
		t.Fatal(err)
	}
	for _, assoc := range []string{"AAA01", "ZZZ99", ""} {
		if _, ok := r.Lookup(fullKey(assoc)); !ok {
			t.Errorf("wildcard did not match association %q", assoc)
		}
	}
}

func TestKeyEqualAndString(t *testing.T) {
	a := fullKey("AAA01")
	b := fullKey("AAA01")
	if !a.Equal(b) {
		t.Error("equal keys reported unequal")
	}
	if a == b {
		t.Error("== compared pointers as equal, which would hide the bug Equal exists for")
	}
	if !a.Equal(a) {
		t.Error("a key is not equal to itself")
	}

	wild := Key{Type: Is("TSTMSG")}
	if wild.Equal(a) {
		t.Error("a wildcard key reported equal to a populated one")
	}
	if got := wild.String(); got != "TSTMSG:*:*:*:*" {
		t.Errorf("String() = %q", got)
	}
	if got := fullKey("").String(); got != "TSTMSG:D:95B:UN:-" {
		t.Errorf("String() = %q", got)
	}
}

func TestCloneCopiesKey(t *testing.T) {
	s := baseSpec()
	c := s.Clone()
	*c.Key.Type = "CHANGED"
	if *s.Key.Type != "TSTMSG" {
		t.Errorf("editing the clone's key reached the original: %q", *s.Key.Type)
	}

	// A nil field stays nil, so a wildcard stays a wildcard.
	w := Spec{Key: Key{Type: Is("TSTMSG")}}.Clone()
	if w.Key.Version != nil || w.Key.Association != nil {
		t.Errorf("a wildcard field was filled in: %s", w.Key)
	}
}

func TestRegisterIsolatesKey(t *testing.T) {
	r := NewRegistry()
	s := baseSpec()
	if err := r.Register(s); err != nil {
		t.Fatal(err)
	}
	*s.Key.Type = "OTHER" // edit the caller's copy after registering

	if _, ok := r.Lookup(fullKey("AAA01")); !ok {
		t.Error("the registered spec was changed through the caller's key")
	}
	if _, ok := r.Lookup(Key{Type: Is("OTHER")}); ok {
		t.Error("the caller's edit is visible in the registry")
	}
}

func TestLookupReturnsACopy(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(baseSpec()); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Lookup(fullKey("AAA01"))
	*got.Key.Type = "OTHER"
	got.Nodes[0].Required = false
	got.Nodes[2].Nodes[0].Tag = "XXX"

	again, ok := r.Lookup(fullKey("AAA01"))
	if !ok {
		t.Fatal("editing the result changed which keys the registry matches")
	}
	if !again.Nodes[0].Required || again.Nodes[2].Nodes[0].Tag != "LOC" {
		t.Error("editing the result reached the registry's nodes")
	}
}

func TestQualifiersRules(t *testing.T) {
	for name, n := range map[string]Node{
		"both":          {Tag: "LOC", Qualifier: "9", Qualifiers: []string{"11"}},
		"empty value":   {Tag: "LOC", Qualifiers: []string{"9", ""}},
		"segment group": {Group: "SG1", Qualifiers: []string{"9"}, Nodes: []Node{{Tag: "LOC"}}},
	} {
		s := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{n}}
		if err := NewRegistry().Register(s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	s := Spec{Key: Key{Type: Is("TSTMSG")}, Nodes: []Node{{Tag: "LOC", Qualifiers: []string{"9", "11"}}}}
	c := s.Clone()
	c.Nodes[0].Qualifiers[0] = "X"
	if s.Nodes[0].Qualifiers[0] != "9" {
		t.Error("Clone shares Qualifiers")
	}
}
