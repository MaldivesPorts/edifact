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

// Diagnostic locates a problem within an interchange: a structural violation
// found by Read or validation, or a value [Fields] could not convert.
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
	// segment group is at fault. It is empty when no segment is named.
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
	// Err is the error behind a value [Fields] could not convert, and nil otherwise.
	Err error
}

// String renders the diagnostic on one line, such as
// "error: message 1 (M1), segment 4, DTM+137: required segment is missing".
func (d Diagnostic) String() string {
	var at []string
	if d.MessageIdx > 0 {
		at = append(at, fmt.Sprintf("message %d (%s)", d.MessageIdx, d.MessageRef))
	}
	if d.SegmentIdx > 0 {
		at = append(at, fmt.Sprintf("segment %d", d.SegmentIdx))
	}
	if d.Tag != "" {
		what := d.Tag
		if d.Path != "" {
			what = d.Path + "/" + what
		}
		if d.Qualifier != "" {
			what += "+" + d.Qualifier
		}
		at = append(at, what)
	}

	var sb strings.Builder
	sb.WriteString(d.Severity.String())
	sb.WriteString(": ")
	if len(at) > 0 {
		sb.WriteString(strings.Join(at, ", "))
		sb.WriteString(": ")
	}
	sb.WriteString(d.Message)
	return sb.String()
}
