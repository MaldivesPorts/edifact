package edifact_test

import (
	"fmt"
	"os"
	"time"

	"github.com/MaldivesPorts/edifact"
)

// outHeader is the document number and date of a message, from BGM and DTM+137.
type outHeader struct {
	Number string
	Date   time.Time
}

// MarshalEDIFACT writes h, with its date as CCYYMMDDHHMM.
func (h outHeader) MarshalEDIFACT(m *edifact.Message) error {
	m.Add("BGM", "", h.Number, "9")
	return m.Add("DTM", "137").SetTime(1, 2, h.Date, edifact.FormatCCYYMMDDHHMM)
}

// Marshal writes a type into a message.
func ExampleMarshal() {
	ix := edifact.NewInterchange()
	ix.SetSender("SENDER", "")
	ix.SetRecipient("RECIPIENT", "")
	ix.SetDateTime(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	ix.SetControlRef("REF1")
	m := ix.AddMessage(edifact.Key{
		Type:    edifact.Is("BAPLIE"),
		Version: edifact.Is("D"),
		Release: edifact.Is("95B"),
		Agency:  edifact.Is("UN"),
	})
	m.SetRef("M1")

	h := outHeader{Number: "BAY1", Date: time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)}
	if err := edifact.Marshal(m, h); err != nil {
		fmt.Println(err)
		return
	}
	if err := edifact.Write(os.Stdout, ix, edifact.SegmentSuffixLF()); err != nil {
		fmt.Println(err)
	}
	// Output:
	// UNA:+.? '
	// UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'
	// UNH+M1+BAPLIE:D:95B:UN'
	// BGM++BAY1+9'
	// DTM+137:202601021504:203'
	// UNT+4+M1'
	// UNZ+1+REF1'
}
