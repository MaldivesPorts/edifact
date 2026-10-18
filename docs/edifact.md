# EDIFACT in brief

An introduction to the UN/EDIFACT format for readers new to it. The [package documentation](https://pkg.go.dev/github.com/MaldivesPorts/edifact) describes the library's API.

## The format

An EDIFACT file is plain text. This is a short bayplan, cut down from the BAPLIE message a ship's operator sends to say which container is in which slot on the vessel. Each line is one segment:

```
UNA:+.? '                          service string advice: declares the separators
UNB+UNOA:2+SENDER+RECIPIENT+260101:1200+REF1'
                                   interchange header: who, to whom, when
UNH+M1+BAPLIE:D:95B:UN'            message header: message M1 is a BAPLIE, directory D.95B
BGM++BAY1+9'                       beginning of message: document number BAY1, original (9)
DTM+137:202601011200:203'          message date, 2026-01-01 12:00, in format 203
LOC+147+0010204::5'                stowage cell 0010204
MEA+WT++KGM:12000'                 weight 12000 kg
LOC+9+XXAAA:139:6'                 port of loading XXAAA
LOC+11+XXBBB:139:6'                port of discharge XXBBB
EQD+CN+XXXU1234567+22G1'           container XXXU1234567, ISO size and type 22G1
LOC+147+0010206::5'                the next cell, and its container
MEA+WT++KGM:8500.5'
LOC+9+XXAAA:139:6'
LOC+11+XXCCC:139:6'
EQD+CN+XXXU7654321+45G1'
UNT+14+M1'                         message trailer: 14 segments from UNH to UNT
UNZ+1+REF1'                        interchange trailer: 1 message
```

Four characters give a segment its structure. The `UNA` at the top declares them, and without a `UNA` these defaults apply:

| Character | Role | Example |
|---|---|---|
| `'` | ends a segment | `BGM++BAY1+9'` |
| `+` | separates elements | `EQD` `+` `CN` `+` `XXXU1234567` |
| `:` | separates the components of one element | `137:202601011200:203` |
| `?` | releases the next character, so it counts as text | `O?'NEILL` is the text `O'NEILL` |

The first three letters of a segment are its tag. The segment's table in the UN directory fixes what each position means. For `DTM` in directory D.95B, element 1 has three components: what the date is (code 137, "Document/message date/time"), the date itself, and the format it is written in (code 203, CCYYMMDDHHMM).

Many segments, like `LOC` above, appear several times with different meanings. The first element tells them apart and is called the qualifier: `LOC+147` is a stowage cell, `LOC+9` a port of loading, `LOC+11` a port of discharge. The codes come from code lists the directory publishes.

The segments come in three layers of envelope:

```
UNA                     optional, declares the separators
UNB                     interchange: one transmission
  UNG                   functional group: optional, bundles messages
    UNH ... UNT         message: the business document
    UNH ... UNT
  UNE
UNZ
```

Inside a message, segments often form repeating segment groups. In the bayplan, each container has its own `LOC+147`, `MEA`, `LOC+9`, `LOC+11` and `EQD`. The file itself has no markers for where a segment group starts or ends. They follow from the message's segment table, which this library calls a specification, covered in the package documentation under [Specifications and segment groups](https://pkg.go.dev/github.com/MaldivesPorts/edifact#hdr-Specifications_and_segment_groups).

## Glossary

| Term | Meaning | In this library |
|---|---|---|
| Interchange | Everything from `UNB` to `UNZ`, one transmission between two parties. | `Interchange` |
| Functional group | An optional `UNG` to `UNE` bundle of messages inside an interchange, all of one type below syntax version 4. | `FunctionalGroup` |
| Message | One business document, from `UNH` to `UNT`. | `Message` |
| Segment | One tagged line ending in the segment terminator, such as `LOC+147+0010204::5'`. | `Segment` |
| Tag | The segment's three-letter code, such as `LOC`. | `Segment.Tag` |
| Element | One `+`-separated position in a segment. | `Segment.Element(i)` |
| Composite | An element made of several components. | `Segment.Components(i)` |
| Component | One `:`-separated position inside a composite. | `Segment.Component(i, j)` |
| Qualifier | The code in element 1 that says what a segment means, such as `147` in `LOC+147`. | `Segment.Qualifier()` |
| Repetition | One of several occurrences of the same element in one segment, allowed from syntax version 4. | `Segment.Repetition(i, r)` |
| Segment group | A repeating run of segments that the message's table treats as one unit, such as `SG2`. | `SegmentGroup` |
| Specification | The segment table for one message type: which segments and segment groups may appear, in what order, how often. | `Spec`, `Node` |
| Key | The message type, version, release, agency and association code from `UNH`, which selects a specification. | `Key` |
| Service segment | A segment that frames or controls the data rather than carrying it, such as `UNB` or `UNH`. Its tag starts with `U`. | header accessors |
| Syntax level | The character set an interchange uses, named in `UNB`, such as `UNOA` for upper case letters, digits and some punctuation. | `Charset` |
| Release character | The character that makes the next separator count as text, `?` by default. | `Delimiters.Release` |
| Diagnostic | A problem found while reading or validating that does not stop the read. | `Diagnostic` |
