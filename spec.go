package edifact

import (
	"errors"
	"fmt"
	"strings"
)

// Unbounded lets a Node repeat without limit.
const Unbounded = -1

// Key identifies a message specification. It is the composite carried by UNH,
// element 2.
//
// A nil field matches any value. A non-nil field must equal it exactly, so
// Absent matches only an empty component. A Key read off the wire never holds
// nil.
type Key struct {
	Type        *string // 0065 message type
	Version     *string // 0052 message version number
	Release     *string // 0054 message release number
	Agency      *string // 0051 controlling agency
	Association *string // 0057 association assigned code
}

// Is returns a Key field matching exactly v.
func Is(v string) *string { return &v }

// Absent returns a Key field matching only an empty component, as distinct from
// a nil field, which matches anything.
func Absent() *string { return Is("") }

// fields returns the five in UNH order, for the operations that treat them
// alike.
func (k Key) fields() [5]*string {
	return [5]*string{k.Type, k.Version, k.Release, k.Agency, k.Association}
}

// Equal reports whether two Keys constrain the same things. Key holds pointers,
// so == compares addresses rather than values.
func (k Key) Equal(other Key) bool {
	a, b := k.fields(), other.fields()
	for i := range a {
		switch {
		case a[i] == nil && b[i] == nil:
		case a[i] == nil || b[i] == nil:
			return false
		case *a[i] != *b[i]:
			return false
		}
	}
	return true
}

// String renders the Key colon-separated, with * for a field matching anything
// and - for one matching only an empty component.
func (k Key) String() string {
	parts := make([]string, 0, 5)
	for _, f := range k.fields() {
		switch {
		case f == nil:
			parts = append(parts, "*")
		case *f == "":
			parts = append(parts, "-")
		default:
			parts = append(parts, *f)
		}
	}
	return strings.Join(parts, ":")
}

// Node is one entry in a message specification, either a segment or a segment
// group, never both.
type Node struct {
	// Tag is the segment tag, for a segment node.
	Tag string
	// Group is the segment group's label, such as SG2, for a segment group
	// node.
	Group string
	// Qualifier is the value element 1 of a segment must have. Empty accepts
	// any.
	Qualifier string
	// Qualifiers lists several values element 1 may have, any of which is
	// accepted, for a segment that may carry any of them in any order. Set
	// either Qualifier or Qualifiers, not both.
	Qualifiers []string
	// Required means the segment or segment group must appear at least once.
	// For a node with Qualifiers, any one of them is enough.
	Required bool
	// Max is how many times it may repeat. Zero means once, and Unbounded
	// means without limit.
	Max int
	// Nodes is a segment group's contents, in order. A new repetition of the
	// segment group starts with any of its nodes up to and including the
	// first required one.
	Nodes []Node
}

// Spec describes the segments between UNH and UNT for one message dialect.
type Spec struct {
	// Key selects the messages this spec applies to.
	Key Key
	// Nodes are the message's segments and segment groups, in order.
	Nodes []Node
}

// Clone returns a deep copy. Key holds pointers and Nodes is a slice, so a
// shallow copy would let an edit reach the original.
func (s Spec) Clone() Spec {
	s.Key = s.Key.clone()
	s.Nodes = cloneNodes(s.Nodes)
	return s
}

// clone returns a Key whose fields point at strings of its own.
func (k Key) clone() Key {
	return Key{
		Type:        cloneField(k.Type),
		Version:     cloneField(k.Version),
		Release:     cloneField(k.Release),
		Agency:      cloneField(k.Agency),
		Association: cloneField(k.Association),
	}
}

func cloneField(f *string) *string {
	if f == nil {
		return nil
	}
	return Is(*f)
}

func cloneNodes(ns []Node) []Node {
	if ns == nil {
		return nil
	}
	out := make([]Node, len(ns))
	for i, n := range ns {
		n.Nodes = cloneNodes(n.Nodes)
		if n.Qualifiers != nil {
			n.Qualifiers = append([]string(nil), n.Qualifiers...)
		}
		out[i] = n
	}
	return out
}

// specificity counts the fields a Key constrains. A Key matching on more fields
// wins over one matching on fewer.
func (k Key) specificity() int {
	n := 0
	for _, f := range k.fields() {
		if f != nil {
			n++
		}
	}
	return n
}

// matches reports whether k, which may hold wildcards, accepts other.
func (k Key) matches(other Key) bool {
	a, b := k.fields(), other.fields()
	for i := range a {
		if a[i] == nil {
			continue
		}
		if b[i] == nil || *a[i] != *b[i] {
			return false
		}
	}
	return true
}

// IsSegment reports whether n names a segment rather than a segment group.
func (n Node) IsSegment() bool { return n.Tag != "" }

// repeats returns the effective upper bound, resolving the zero value to 1.
func (n Node) repeats() int {
	if n.Max == 0 {
		return 1
	}
	return n.Max
}

// validate checks a Spec's shape. It does not consult any message.
func (s Spec) validate() error {
	if s.Key.Type == nil || *s.Key.Type == "" {
		return errors.New("edifact: spec has no message type")
	}
	return validateNodes(s.Nodes, *s.Key.Type)
}

func validateNodes(ns []Node, path string) error {
	previous := ""
	for _, n := range ns {
		switch {
		case n.Tag != "" && n.Group != "":
			return fmt.Errorf("edifact: %s: node names both tag %q and segment group %q", path, n.Tag, n.Group)
		case n.Tag == "" && n.Group == "":
			return fmt.Errorf("edifact: %s: node names neither a tag nor a segment group", path)
		}

		here := path + "/" + n.Tag + n.Group
		if n.Max < Unbounded {
			return fmt.Errorf("edifact: %s: Max is %d", here, n.Max)
		}

		if n.Qualifier != "" && len(n.Qualifiers) > 0 {
			return fmt.Errorf("edifact: %s: node sets both Qualifier and Qualifiers", here)
		}
		for _, q := range n.Qualifiers {
			if q == "" {
				return fmt.Errorf("edifact: %s: Qualifiers holds an empty value", here)
			}
		}

		if n.IsSegment() {
			if len(n.Nodes) > 0 {
				return fmt.Errorf("edifact: %s: a segment cannot hold nodes", here)
			}
		} else {
			if n.Qualifier != "" || len(n.Qualifiers) > 0 {
				return fmt.Errorf("edifact: %s: a segment group cannot hold a qualifier", here)
			}
			if len(n.Nodes) == 0 {
				return fmt.Errorf("edifact: %s: a segment group holds no nodes", here)
			}
			if err := validateNodes(n.Nodes, here); err != nil {
				return err
			}
		}

		id := n.Tag + "+" + n.qualifierText() + "/" + n.Group
		if id == previous {
			return fmt.Errorf("edifact: %s: declared twice in a row, which Max covers", here)
		}
		previous = id
	}
	return nil
}

// accepts reports whether q satisfies the node's qualifier. A node with
// neither Qualifier nor Qualifiers accepts any value.
func (n Node) accepts(q string) bool {
	if len(n.Qualifiers) > 0 {
		for _, want := range n.Qualifiers {
			if q == want {
				return true
			}
		}
		return false
	}
	return n.Qualifier == "" || q == n.Qualifier
}

// qualifierText renders the qualifiers a node expects, joined by "/" when it
// lists several.
func (n Node) qualifierText() string {
	if len(n.Qualifiers) > 0 {
		return strings.Join(n.Qualifiers, "/")
	}
	return n.Qualifier
}
