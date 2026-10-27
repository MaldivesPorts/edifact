package edifact

import "errors"

// ErrNoMessage reports a nil message passed to [Unmarshal] or [Marshal].
var ErrNoMessage = errors.New("edifact: no message")

// Unmarshaler is a type that reads itself from a message.
type Unmarshaler interface {
	// UnmarshalEDIFACT reads m into the receiver. It records values that will
	// not convert, and problems it finds, on f. It returns an error only when
	// it cannot read m at all, such as a message of a type it does not handle.
	UnmarshalEDIFACT(m *Message, f *Fields) error
}

// Marshaler is a type that writes itself into a message.
type Marshaler interface {
	// MarshalEDIFACT adds the receiver's segments to m, choosing every value,
	// code and format written. It returns an error for a value it cannot
	// write.
	MarshalEDIFACT(m *Message) error
}

// Unmarshal reads m into v with a [Fields] located in m, and returns the
// diagnostics v recorded on it. They are returned even when v returns an
// error. A nil m gives [ErrNoMessage].
func Unmarshal(m *Message, v Unmarshaler) ([]Diagnostic, error) {
	if m == nil {
		return nil, ErrNoMessage
	}
	f := NewFields(m)
	err := v.UnmarshalEDIFACT(m, f)
	return f.Diagnostics, err
}

// Marshal writes v into m, usually a message just returned by
// [Interchange.AddMessage]. A nil m gives [ErrNoMessage].
func Marshal(m *Message, v Marshaler) error {
	if m == nil {
		return ErrNoMessage
	}
	return v.MarshalEDIFACT(m)
}
