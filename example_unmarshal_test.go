package edifact_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/MaldivesPorts/edifact"
)

// inHeader is the document number and date of a message, from BGM and DTM+137.
type inHeader struct {
	Number string
	Date   time.Time
}

// UnmarshalEDIFACT reads h, recording a date that will not convert on f.
func (h *inHeader) UnmarshalEDIFACT(m *edifact.Message, f *edifact.Fields) error {
	h.Number = m.Find("BGM", "").Element(2)
	h.Date = f.Time(m.Find("DTM", "137"), 1, 2, "document date")
	return nil
}

// Unmarshal reads a message into a type and returns the diagnostics the type
// recorded.
func ExampleUnmarshal() {
	const plan = `UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'
UNH+M1+BAPLIE:D:95B:UN'
BGM++BAY1+9'
DTM+137:2026010112AB:203'
UNT+4+M1'
UNZ+1+REF1'`

	ix, err := edifact.Read(strings.NewReader(plan))
	if err != nil {
		fmt.Println(err)
		return
	}
	var h inHeader
	diags, err := edifact.Unmarshal(ix.Message(1), &h)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(h.Number, h.Date.IsZero())
	for _, d := range diags {
		fmt.Println(d)
	}
	// Output:
	// BAY1 true
	// warning: message 1 (M1), segment 3, DTM+137: document date: element 1 component 2: format 203: value "2026010112AB" is not all digits
}
