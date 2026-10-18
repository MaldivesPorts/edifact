# edifact

A Go library for reading and writing UN/EDIFACT interchanges, the message format ISO 9735 defines.

```
go get github.com/MaldivesPorts/edifact
```

It needs Go 1.22 or later and has no dependencies outside the standard library.

## Quick start

### Read a file

`bayplan.edi` holds the short bayplan from [EDIFACT in brief](docs/edifact.md#the-format).

```go
f, err := os.Open("bayplan.edi")
if err != nil {
	log.Fatal(err)
}
defer f.Close()

ix, err := edifact.Read(f)
if err != nil {
	log.Fatal(err)
}

fmt.Println("from", ix.Sender(), "to", ix.Recipient())
for _, m := range ix.Messages() {
	fmt.Println("message", m.Key())
	for _, eqd := range m.FindAll("EQD", "CN") {
		fmt.Println("container", eqd.Element(2), "size and type", eqd.Element(3))
	}
}
```

```
from SENDER to RECIPIENT
message BAPLIE:D:95B:UN:-
container XXXU1234567 size and type 22G1
container XXXU7654321 size and type 45G1
```

### Write a file

```go
ix := edifact.NewInterchange()
ix.SetSender("SENDER", "")
ix.SetRecipient("RECIPIENT", "")
ix.SetDate("260101")
ix.SetTime("1200")
ix.SetControlRef("REF1")

m := ix.AddMessage(edifact.Key{
	Type:    edifact.Is("BAPLIE"),
	Version: edifact.Is("D"),
	Release: edifact.Is("95B"),
	Agency:  edifact.Is("UN"),
})
m.SetRef("M1")
m.Add("BGM", "", "BAY1", "9")
m.AddComposite("DTM", []string{"137", "202601011200", "203"})
m.AddComposite("LOC", []string{"147"}, []string{"0010204", "", "5"})
m.Add("EQD", "CN", "XXXU1234567", "22G1")

if err := edifact.Write(os.Stdout, ix, edifact.SegmentSuffixLF()); err != nil {
	log.Fatal(err)
}
```

```
UNA:+.? '
UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'
UNH+M1+BAPLIE:D:95B:UN'
BGM++BAY1+9'
DTM+137:202601011200:203'
LOC+147+0010204::5'
EQD+CN+XXXU1234567+22G1'
UNT+6+M1'
UNZ+1+REF1'
```

`Write` fills in the `UNA`, `UNT` and `UNZ` lines, including their counts.

## Documentation

- [EDIFACT in brief](docs/edifact.md) introduces the format, as well as the terms used by this package.
- The [package documentation](https://pkg.go.dev/github.com/MaldivesPorts/edifact) covers more options (including defining specs and validating against them), with runnable examples.
- [CHANGELOG.md](CHANGELOG.md) lists what changed in each version.

## Known limitations/room for improvement

- **Batch EDIFACT only.** Interactive EDIFACT (`UIB` to `UIZ`) and the syntax version 4 package segments (`UNO`, `UNP`) are not read or written.
- **Character sets.** Only `UNOA`, `UNOB`, `UNOC` and `UNOW` are checked and converted. Other syntax levels, such as `UNOD` to `UNOK` (other parts of ISO 8859) and `UNOX` and `UNOY` (ISO 2022 code extension), are read with a warning and written with their values as they are.
- **Level B separators without a `UNA`.** A file with no `UNA` that uses the level B default separators (IS3 between elements, IS1 between components, IS4 after each segment) is not detected, and needs `DefaultDelimiters`. Detecting them automatically would fix this.
- **Values are not checked against the directory.** A specification describes segment order, repetition and qualifiers. It does not check element lengths, formats, code list values, or which data elements a segment requires.
- **Header checks are limited.** `Write` refuses empty mandatory header elements and a date of preparation of the wrong length for the syntax version, but checks nothing else about their format, such as a month of 13.
- **Explicit segment indices go unchecked.** `DDD:1:2` survives a read and a write, but validation does not compare the numbers with the nesting a specification describes.
- **Date formats.** Only the codes listed under [Numbers and dates](https://pkg.go.dev/github.com/MaldivesPorts/edifact#hdr-Numbers_and_dates) are supported. Time zones written as letters must be `GMT` or `UTC`.
- **Whole interchanges in memory.** `Read` builds the complete interchange before returning, and `Write` renders all of it before writing. A streaming reader/writer would fix this.
- **Round trips change some bytes.** See [Reading and writing back](https://pkg.go.dev/github.com/MaldivesPorts/edifact#hdr-Reading_and_writing_back).

## License

MIT. See [LICENSE](LICENSE).
