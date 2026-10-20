package edifact

import (
	"fmt"
	"strings"
)

// Severity ranks a diagnostic.
type Severity int

const (
	// Warning marks a problem that leaves the interchange usable, such as a
	// segment a spec does not declare.
	Warning Severity = iota
	// Error marks a problem that breaks the standard or a spec, such as a
	// missing required segment. Write with a registry refuses a message whose
	// validation gives one.
	Error
)

// String returns "warning" or "error".
func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warning"
}

// Diagnostic locates a structural violation within an interchange.
type Diagnostic struct {
	// Severity is Warning or Error.
	Severity Severity
	// MessageIdx is the message's position in the interchange, counting from
	// 1, or 0 when the diagnostic concerns the envelope.
	MessageIdx int
	// MessageRef is the message's UNH reference, which need not be unique.
	MessageRef string
	// Path names the segment groups enclosing the problem, such as /SG2/SG3,
	// and is empty at the top level of a message or for the envelope.
	Path string
	// Tag is the segment tag, or the segment group's name when a whole
	// segment group is at fault.
	Tag string
	// Qualifier is element 1 of the segment, or the qualifier a spec expected.
	Qualifier string
	// SegmentIdx counts from 1, starting at UNH when MessageIdx is set and at
	// UNB otherwise. It is 0 when no single segment is at fault. Within a
	// message, segment n is Segments()[n-2], with UNH at 1 and UNT one past the
	// last of Segments().
	SegmentIdx int
	// Message describes the problem.
	Message string
}

// String renders the diagnostic on one line, such as
// "error: message 1 (M1), segment 4, DTM+137: required segment is missing".
func (d Diagnostic) String() string {
	var sb strings.Builder
	sb.WriteString(d.Severity.String())
	sb.WriteString(": ")
	if d.MessageIdx > 0 {
		fmt.Fprintf(&sb, "message %d (%s), ", d.MessageIdx, d.MessageRef)
	}
	if d.SegmentIdx > 0 {
		fmt.Fprintf(&sb, "segment %d, ", d.SegmentIdx)
	}
	if d.Path != "" {
		sb.WriteString(d.Path)
		sb.WriteByte('/')
	}
	sb.WriteString(d.Tag)
	if d.Qualifier != "" {
		sb.WriteByte('+')
		sb.WriteString(d.Qualifier)
	}
	sb.WriteString(": ")
	sb.WriteString(d.Message)
	return sb.String()
}
