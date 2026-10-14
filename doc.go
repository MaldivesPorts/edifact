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
// [ParseTime] works on a plain string. These codes from code list 2379 are
// supported:
//
//	2    DDMMYY                301  YYMMDDHHMMZZZ
//	3    MMDDYY                401  HHMM
//	101  YYMMDD                402  HHMMSS
//	102  CCYYMMDD              404  HHMMSSZZZ
//	201  YYMMDDHHMM            602  CCYY
//	202  YYMMDDHHMMSS          610  CCYYMM
//	203  CCYYMMDDHHMM          718  CCYYMMDD-CCYYMMDD
//	204  CCYYMMDDHHMMSS        719  CCYYMMDDHHMM-CCYYMMDDHHMM
//	205  CCYYMMDDHHMMZHHMM
//
// ZZZ is a time zone written as letters. Time reads GMT and UTC as UTC, and
// returns [ErrDateZone] for any other zone. A time from a format without a zone
// keeps its digits as written and comes back in [UnknownTimezone], so a caller
// can attach the right zone. [Segment.TimeRange] and [ParseTimeRange] read the
// range formats 718 and 719.
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
//   - every problem a specification finds
//
// Each [Diagnostic] gives its [Severity], the message it concerns and the
// segment.
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
// [NewInterchange] returns an interchange with syntax UNOA version 2. Set the
// UNB values with its setters, then add messages with [Interchange.AddMessage].
// [Message.Add] builds a segment from simple elements and
// [Message.AddComposite] from a slice of components per element.
// [Segment.Set], [Segment.SetInt], [Segment.SetFloat] and [Segment.SetTime]
// write one component of an existing segment, extending it as needed.
//
// For formats 301 and 404, whose zone is written as letters, SetTime converts
// the time to UTC and writes UTC. For other formats it writes the time's own
// clock, and for 205 its own offset as well.
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
//   - drops trailing empty components and elements and trailing spaces, as
//     ISO 9735 requires
//   - returns [ErrIncompleteHeader] for a UNB, UNG or UNH missing an element
//     ISO 9735 makes mandatory, [ErrNoContent] for an interchange or functional
//     group with no messages or a message with no segments, and
//     [ErrGroupMismatch] below syntax version 4 for a message that differs from
//     its functional group
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
// The first component of UNB names the syntax level, mostly a character set.
// Read reads any level when the file starts with a UNA. Without one, it
// understands the level A separators ' + : ?, plus * as the repetition
// separator in syntax version 4, or the separators given with
// [DefaultDelimiters]. A file without a UNA that uses the level B separators
// otherwise gives [ErrUnsupportedSeparators].
//
// # Repetitions
//
// From syntax version 4, one element can hold several values, separated by the
// repetition separator. [Segment.Repetitions] counts them and
// [Segment.Repetition] reads one. Element, Component, Components and Set work
// on the first repetition, so code that ignores repetitions keeps working.
// [Segment.SetRepetition] writes into one.
//
// To write repetitions, call [Interchange.SetSyntax] with version 4. Syntax
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
