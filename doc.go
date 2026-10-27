// Package edifact reads and writes UN/EDIFACT interchanges, as defined by
// ISO 9735.
//
// An [Interchange] holds messages, and each [Message] holds segments. Positions
// in a [Segment] count from 1, the way the published segment tables number
// them. For readers new to the format, [EDIFACT in brief] walks through a
// short file and defines the terms used here.
//
//	ix, err := edifact.Read(r)
//	if err != nil {
//		log.Fatal(err)
//	}
//	for _, eqd := range ix.Message(1).FindAll("EQD", "CN") {
//		fmt.Println(eqd.Element(2))
//	}
//
// # Reading
//
// [Read] reads the envelope and segments without consulting any
// specification. Envelope values have named accessors, such as
// [Interchange.Sender] and [Message.Key]. To read a position that has no named
// accessor, [Interchange.Header], [FunctionalGroup.Header] and
// [Message.Header] return the UNB, UNG and UNH segments themselves.
//
// [Message.Find] returns the first segment with a tag and qualifier, and
// [Message.FindAll] every one. An empty qualifier matches any.
// [Segment.Element], [Segment.Component] and [Segment.Components] read
// positions. An absent position, or any position of a missing segment, reads
// as "", so a chain such as m.Find("RFF", "BM").Element(2) never panics.
//
// # Numbers and dates
//
// [Segment.Float] accepts a dot or a comma as the decimal mark, a leading
// decimal mark (.5) and an exponent (1E3). [Segment.Int] accepts an optional
// minus sign and digits. Both return [ErrNumeric] for anything else, such as
// +5, 2., 1,000.5 or NaN, and [ErrEmpty] for an empty or absent position.
//
// [Segment.Time] reads a date in the format named by the component after it,
// the layout DTM uses. [Segment.TimeIn] takes the format as an argument, and
// [ParseTime] works on a plain string. These formats are supported, each a
// [DateFormat] constant:
//
//	2    FormatDDMMYY               301  FormatYYMMDDHHMMZZZ
//	3    FormatMMDDYY               401  FormatHHMM
//	101  FormatYYMMDD               402  FormatHHMMSS
//	102  FormatCCYYMMDD             404  FormatHHMMSSZZZ
//	201  FormatYYMMDDHHMM           602  FormatCCYY
//	202  FormatYYMMDDHHMMSS         610  FormatCCYYMM
//	203  FormatCCYYMMDDHHMM         718  FormatCCYYMMDDPeriod
//	204  FormatCCYYMMDDHHMMSS       719  FormatCCYYMMDDHHMMPeriod
//	205  FormatCCYYMMDDHHMMZHHMM
//
// A qualifier read from a message converts with DateFormat(code). Any other
// gives [ErrDateFormat].
//
// ZZZ is a time zone written as letters. Time reads GMT and UTC as UTC, and
// returns [ErrDateZone] for any other zone. A time from a format without a zone
// keeps its digits as written and comes back in [UnknownTimezone], so a caller
// can attach the right zone. [Segment.TimeRange] and [ParseTimeRange] read the
// formats that name a period.
//
// # Reading many values
//
// [Fields] reads values the way the typed accessors do, but records a warning
// for each value that will not convert instead of returning an error. A caller
// can read every value it needs and handle the problems once:
//
//	f := edifact.NewFields(m)
//	date := f.Time(m.Find("DTM", "137"), 1, 2, "document date")
//	for _, cell := range m.FindAllGroups("SG2") {
//		weight := f.Float(cell.Find("MEA", "WT"), 3, 2, "weight")
//		// ...
//	}
//	for _, d := range f.Diagnostics {
//		log.Println(d)
//	}
//
// An absent segment or an empty position gives the zero value and records
// nothing. Each warning is a [Diagnostic] naming the message, the segment and
// its segment groups, followed by the caller's label for the value. Its Err
// holds the conversion error, so errors can be compared with errors.Is later.
// [Fields.Addf] records a problem only the caller can see, such as one value
// missing where another makes it necessary.
//
// # Reading into and writing from types
//
// A type that implements [Unmarshaler] reads itself from a message, and one
// that implements [Marshaler] writes itself into one. A type may implement
// either or both. Its methods choose every segment, code and format, so the
// mapping stays in the caller's code:
//
//	func (h *Header) UnmarshalEDIFACT(m *edifact.Message, f *edifact.Fields) error {
//		h.Number = m.Find("BGM", "").Element(2)
//		h.Date = f.Time(m.Find("DTM", "137"), 1, 2, "document date")
//		return nil
//	}
//
//	func (h Header) MarshalEDIFACT(m *edifact.Message) error {
//		m.Add("BGM", "", h.Number, "9")
//		return m.Add("DTM", "137").SetTime(1, 2, h.Date, edifact.FormatCCYYMMDDHHMM)
//	}
//
// [Unmarshal] calls UnmarshalEDIFACT with a [Fields] located in the message
// and returns the diagnostics it collected. [Marshal] calls MarshalEDIFACT.
// Write with a registry then checks the result against its spec.
//
// # Filler
//
// Read skips whitespace, control characters such as STX and ETX, and a UTF-8
// byte order mark before the first segment or after the last. It also skips
// line breaks between segments, so files with one segment per line read the
// same as files with none.
//
// # Errors and diagnostics
//
// Read returns an error only when it cannot carry on:
//
//   - no UNB at the start, or a UNA with missing or repeated separators
//   - a service segment out of place, such as UNH inside a message
//   - a message or functional group still open at the end
//   - a segment with no terminator, or one longer than [MaxSegmentSize]
//   - anything after UNZ
//
// By default, Read collects everything else on Interchange.Diagnostics and
// succeeds:
//
//   - a UNT, UNE or UNZ count or reference that does not match
//   - a header missing mandatory elements
//   - no UNZ
//   - some messages in functional groups and some not
//   - an interchange or functional group with no messages, or a message with
//     no segments
//   - below syntax version 4, a message whose type, version, release or
//     agency differs from its functional group's
//   - a value holding a character outside the interchange's charset, once per
//     segment, and a charset this package does not support
//   - every problem a specification finds
//
// Each [Diagnostic] gives its [Severity], the message it concerns and the
// segment. [Fields] produces diagnostics of the same type for values that will
// not convert.
//
// # Read options
//
// [Read], [ReadWithSpec] and [Registry.Read] take options after their other
// arguments:
//
//   - [RejectReadOn] returns the first diagnostic at or above a severity as an
//     error. Default: none.
//   - [MaxSegmentSize] limits the length of one segment. Default: 16 MiB.
//   - [DefaultDelimiters] gives the separators for a file with no UNA.
//     Default: the level A separators.
//
// # Specifications and segment groups
//
// Without a specification a message is a flat list of segments. A [Spec]
// describes one message type's segment table, so reading can sort the segments
// into [SegmentGroup] views and report what does not fit. The library ships no
// specs. Callers write them for the messages they handle.
//
// A segment group is a view over the message's own segments, so a segment
// found through one is the same segment [Message.Segments] holds.
// [Message.FindGroup] returns the first repetition of a segment group and
// [Message.FindAllGroups] every one. Segment groups nest, and the same methods
// work at every level. On a missing message or segment group they return nil,
// so a chain ending in Element gives "" rather than panicking.
//
// A [Node] is either a segment, named by Tag, or a segment group, named by
// Group and holding Nodes. Qualifier is the value element 1 must have, and
// Qualifiers lists several accepted in any order. Required means at least once,
// and for a node with Qualifiers any one of them is enough. Max is how often it
// may appear: 0 means once and [Unbounded] means without limit.
//
// A spec may declare the same tag in several nodes with different qualifiers,
// and the same segment at two positions with other nodes between them.
// [Registry.Register] refuses a segment declared twice in a row, since Max
// covers that. Validation reports a warning for a segment that matches nothing
// where it appears, and for one repeated more often than its Max allows. Either
// stays in the view of the segment group it falls inside. Validation reports an
// error for an absent required segment or segment group.
//
// [Spec.Clone] copies a spec deeply, to derive a variant without touching the
// original.
//
// # Keys and the registry
//
// A [Key] selects a spec from a message's UNH. Its fields are the data elements
// 0065 message type, 0052 version, 0054 release, 0051 controlling agency and
// 0057 association assigned code. [Is] matches a value exactly, a nil field
// matches anything, and [Absent] matches only an empty component. When several
// specs match a message, the one constraining the most fields wins, and a later
// registration beats an earlier one with the same fields.
//
// A [Registry] holds specs. [Registry.Register] adds one and [Registry.Load] a
// slice. [Registry.Validate] checks an interchange built in code and fills in
// its segment groups, as [Registry.Read] does.
//
// # Choosing which specs apply
//
// The library keeps no registry between calls. Every read or write names the
// specs it uses:
//
//	Read                        Write                            Specs used
//	edifact.Read(r)             edifact.Write(w, ix)             none
//	reg.Read(r)                 reg.Write(w, ix)                 those in reg
//	edifact.ReadWithSpec(r, s)  edifact.WriteWithSpec(w, ix, s)  s alone
//
// Nothing is shared between registries, so separate parts of a program, or
// separate tests, can each use their own. Build a registry fully before reading
// with it. After that, any number of goroutines can read and write through it.
//
// In a program that reads many files, build the registry once in a package of
// its own and call Read through a small function there:
//
//	// Package messages reads the EDIFACT messages this program handles.
//	package messages
//
//	var registry = sync.OnceValues(func() (*edifact.Registry, error) {
//		reg := edifact.NewRegistry()
//		return reg, reg.Load(specs)
//	})
//
//	func Read(r io.Reader, opts ...edifact.ReadOption) (*edifact.Interchange, error) {
//		reg, err := registry()
//		if err != nil {
//			return nil, err
//		}
//		return reg.Read(r, opts...)
//	}
//
// # Writing
//
// [NewInterchange] returns an interchange in [UNOA], syntax version 2. Set the
// UNB values with its setters, then add messages with [Interchange.AddMessage].
// [Interchange.SetDateTime] writes the date and time of preparation from a
// time.Time, in the format the syntax version requires: CCYYMMDD in version 4
// and YYMMDD below it. [Interchange.SetDate] and [Interchange.SetTime] write
// them as text.
// [Message.Add] builds a segment from simple elements and
// [Message.AddComposite] from a slice of components per element.
// [Segment.Set], [Segment.SetInt], [Segment.SetFloat] and [Segment.SetTime]
// write one component of an existing segment, extending it as needed.
//
// SetTime writes the time's own clock, and its offset when the format carries
// one. A format whose zone is written as letters gets the time in UTC.
//
// [Interchange.AddMessageTo] puts a message in a functional group from
// [Interchange.AddFunctionalGroup]. Write writes functional groups in the order
// they were added, each with its messages, and UNZ then counts functional
// groups rather than messages.
//
// [Message.Add] appends to the end of the message. [SegmentGroup.Add] and
// [SegmentGroup.AddGroup] place each segment where its segment group belongs,
// even after later segment groups exist.
//
// [Write]:
//
//   - computes the UNT, UNE and UNZ counts and references
//   - always writes a UNA declaring Interchange.Delimiters, with a comma as the
//     decimal mark if Delimiters.Decimal is one and a dot otherwise. SetFloat
//     values use that mark. Values set as text keep theirs.
//   - escapes separators inside values with the release character
//   - checks every value against the interchange's charset, and encodes UNOC
//     values as ISO 8859-1. A character outside the charset gives
//     [ErrCharset], naming its position.
//   - drops trailing empty components and elements and trailing spaces, as
//     ISO 9735 requires
//   - returns [ErrIncompleteHeader] for a UNB, UNG or UNH missing an element
//     ISO 9735 makes mandatory, [ErrNoContent] for an interchange or functional
//     group with no messages or a message with no segments, and
//     [ErrGroupMismatch] below syntax version 4 for a message that differs from
//     its functional group
//   - returns [ErrHeaderDate] for a UNB or UNG date whose length does not
//     match the syntax version, and [ErrSyntaxVersion] for UNOW below
//     version 4
//   - writes nothing if any of this fails
//
// [Registry.Write] and [WriteWithSpec] validate every message first. They drop
// a segment with no data that the spec marks conditional, and write a mandatory
// one as its bare tag. Without a spec, Write writes every segment and validates
// nothing.
//
// # Write options
//
//   - [SegmentSuffixLF] and [SegmentSuffixCRLF] write a line break after every
//     segment. Default: none.
//   - [AllowIncompleteHeaders] writes headers missing mandatory elements.
//   - [RejectWriteOn] returns the first diagnostic at or above a severity as an
//     error. Default: [Error].
//   - [FitCharset] converts characters outside the charset where it can, rather
//     than refusing them. Default: off.
//
// # Reading and writing back
//
// Reading a file and writing it back gives the same data, though not always the
// same bytes. Write drops line breaks, trailing spaces, trailing empty elements,
// and segments with no data that the spec marks conditional. It adds a UNA if
// there was none, and recomputes the trailer counts.
//
// A file without a UNA may use a comma as the decimal mark, and Write declares
// a dot in the UNA it adds. The library cannot tell which values are numbers,
// so it leaves them as they are. Read the positions known to be numeric with
// Float and write them back with SetFloat.
//
// # Separators and syntax levels
//
// The UNA declares six characters, in order: component separator, element
// separator, decimal mark, release character, repetition separator and segment
// terminator. [Interchange] keeps what was read in its Delimiters field. A
// space in the release position means there is none, and writing a value that
// holds a separator then returns [ErrUnreleasable].
//
// The first component of UNB names the syntax level, which is mostly a
// character set, a [Charset]. Read reads any level when the file starts with a
// UNA. Without one, it
// understands the level A separators ' + : ?, plus * as the repetition
// separator in syntax version 4, or the separators given with
// [DefaultDelimiters]. A file without a UNA that uses the level B separators
// otherwise gives [ErrUnsupportedSeparators].
//
// # Charsets
//
// A [Charset] is one of the syntax levels this package reads and writes:
//
//	UNOA  level A: upper case A to Z, digits, space and . , - ( ) / = ' + : ?
//	      ! " % & * ; < >
//	UNOB  level B: level A and lower case a to z
//	UNOC  level C: ISO 8859-1
//	UNOW  level W: UTF-8, syntax version 4 only
//
// [Interchange.SetCharset] and [Interchange.Charset] write and read it. Values
// in a Segment are always Go strings in UTF-8. Read decodes UNOC from
// ISO 8859-1 and Write encodes it back. Write refuses a value holding a
// character outside the charset, and Read reports one as a warning. The level A
// service characters ' + : ? may appear in values, and Write releases them where
// they clash with the separators.
//
// With [FitCharset], Write first converts what it can: an accented Latin-1
// letter becomes its base letter, such as é to e, Æ to AE and ß to ss, and in
// UNOA lower case becomes upper case. A file naming another syntax level, such
// as UNOD, is read with a warning and written with its values as they are.
//
// # Repetitions
//
// From syntax version 4, one element can hold several values, separated by the
// repetition separator. [Segment.Repetitions] counts them and
// [Segment.Repetition] reads one. Element, Component, Components and Set work
// on the first repetition, so code that ignores repetitions keeps working.
// [Segment.SetRepetition] writes into one.
//
// To write repetitions, call [Interchange.SetSyntaxVersion] with 4. Syntax
// versions 1 to 3 have no repetitions. The fifth UNA position is a space, * is
// an ordinary character, and writing an element with several repetitions
// returns [ErrNoRepetitionSeparator].
//
// # Explicit segment indices
//
// A tag can carry its nesting level and repetition number as components, such
// as DDD:1:2. Read takes the tag as DDD, so specs match it, and
// [Segment.Index] returns the numbers. [Segment.SetIndex] sets them.
//
// [EDIFACT in brief]: https://github.com/MaldivesPorts/edifact/blob/main/docs/edifact.md
package edifact
