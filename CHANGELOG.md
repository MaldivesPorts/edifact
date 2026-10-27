# Changelog

## v0.3.0 (2026-10-27)

### Changed (breaking)

- `Segment.SetTime`, `Segment.TimeIn`, `ParseTime` and `ParseTimeRange` take the format as a `DateFormat` instead of a `string`. Use either the provided constants, or convert with `DateFormat(code)`.

### Added

- `DateFormat`, with a constant for each supported code list 2379 format, named after its layout, such as `FormatCCYYMMDDHHMM` for 203.
- `Fields`, which reads typed values from a message's segments and records a `Diagnostic` for each value that will not convert, with `Fields.Addf` for additional problems the caller finds.
- `Unmarshaler` (interface) and `Unmarshal`, for a struct that reads itself from a message.
- `Marshaler` (interface) and `Marshal`, for a struct that writes itself into a message.
- `ErrNoMessage`, from `Unmarshal` and `Marshal` given a nil message.
- `Diagnostic.Err`, the conversion error behind a diagnostic from `Fields`, and nil for every other diagnostic.
- `Diagnostic.String` leaves out the location when a diagnostic names no message, segment or tag.

## v0.2.1 (2026-10-20)

Documentation only.

## v0.2.0 (2026-10-18)

### Changed (breaking)

- `Interchange.SetSyntax(identifier, version)` is replaced by `SetCharset(c)` and `SetSyntaxVersion(v)`.
- `Interchange.SyntaxIdentifier()` is replaced by `Charset()`, which returns a `Charset` and false for a syntax level the package does not support. The identifier as written stays readable through `Header().Component(1, 1)`.
- `Interchange.SyntaxVersion()` returns an `int`, or 0 when `UNB` names none or an unknown one.
- `SetPreparedDatetime(date, time)` on `Interchange` and `FunctionalGroup` is replaced by `SetDate(date)` and `SetTime(time)`.
- `FunctionalGroup.SetVersionRelease(version, release)` is replaced by `SetVersion(version)` and `SetRelease(release)`.
- `Write` checks every value against the interchange's charset and returns `ErrCharset` for a character outside it. Lower case in a `UNOA` interchange is now refused.
- `Write` refuses a `UNB` or `UNG` date of preparation whose length does not match the syntax version (`ErrHeaderDate`), and `UNOW` below syntax version 4 (`ErrSyntaxVersion`).
- `Read` decodes `UNOC` values from ISO 8859-1, so non-ASCII text in a `UNOC` file reads as UTF-8 rather than raw bytes.

### Added

- `Charset`, with `UNOA`, `UNOB`, `UNOC` and `UNOW`.
- `FitCharset()`, a write option that converts characters outside the charset where it can: accented Latin-1 letters to their base letters, and lower case to upper case in `UNOA`.
- `DateTime()` and `SetDateTime(t)` on `Interchange` and `FunctionalGroup`, reading and writing the date and time of preparation as a `time.Time`.
- Read warnings for a character outside the charset, once per segment, and for a syntax level the package does not support.
- `ErrCharset`, `ErrHeaderDate` and `ErrSyntaxVersion`.

## v0.1.0 (2026-10-14)

Initial release.
