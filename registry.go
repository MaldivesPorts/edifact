package edifact

import "fmt"

// Registry resolves a Key to a Spec. Each Registry is independent of every
// other, and the package holds none of its own.
//
// Register and Load are not safe for concurrent use. Once a Registry is built,
// any number of goroutines may read, validate and write through it.
type Registry struct {
	specs []Spec
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register validates s and stores a deep copy, so later edits to s do not reach
// the Registry. It refuses a malformed spec, such as one with no message type,
// a node naming both or neither of a tag and a segment group, or a segment
// declared twice in a row.
//
// Registering a Key already present overrides the earlier spec for it, since
// [Registry.Lookup] prefers the later registration.
func (r *Registry) Register(s Spec) error {
	if err := s.validate(); err != nil {
		return err
	}
	r.specs = append(r.specs, s.Clone())
	return nil
}

// Load registers each spec in turn, stopping at the first that fails.
func (r *Registry) Load(specs []Spec) error {
	for i, s := range specs {
		if err := r.Register(s); err != nil {
			return fmt.Errorf("edifact: spec %d of %d: %w", i+1, len(specs), err)
		}
	}
	return nil
}

// Lookup returns a copy of the spec matching k. Where several match, the one
// constraining the most fields wins, and the latest registration breaks a tie.
func (r *Registry) Lookup(k Key) (*Spec, bool) {
	s, ok := r.lookup(k)
	if !ok {
		return nil, false
	}
	c := s.Clone()
	return &c, true
}

// lookup is Lookup without the copy, returning the registry's own spec.
func (r *Registry) lookup(k Key) (*Spec, bool) {
	best := -1
	bestScore := -1
	for i := range r.specs {
		if !r.specs[i].Key.matches(k) {
			continue
		}
		if score := r.specs[i].Key.specificity(); score >= bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return nil, false
	}
	return &r.specs[best], true
}

// Len reports how many specs are registered.
func (r *Registry) Len() int { return len(r.specs) }
