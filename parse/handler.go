package parse

import "github.com/MaldivesPorts/edifact/spec"

// Handler parse handler interface
type Handler interface {
	Handle(*spec.Node, spec.Segment) error
}
