# Changelog

## v0.4.1 - 2026-09-23

- `StreamWriter.Grow`/`Reserve` reject negative sizes even after a sticky
  error, and `Reserve(0)` no longer flushes pending bytes.
- Truncated varints panic with `bt: truncated ULEB128/SLEB128 at offset N`
  instead of a misleading `need 1 bytes ... have 2` message.
- Benchmarks: `BenchmarkWriteRecords` reports `records/op` correctly and
  `BenchmarkWriteFixedRecords` reports throughput for every variant.
- Documentation fixes: `StreamWriter.Buffered` in the sections recipe, reserve
  and patch lifetime warnings, accurate fuzz-seed description, updated panic
  reference, cleaner performance tables.

## v0.4.0 - 2026-09-22

- Varint decoding is unrolled: `ULEB128` and `SLEB128` resolve three- to
  ten-byte values in a flat fast path (10-byte varints ~2.5x faster, no
  regression for one- and two-byte values).
- Stateless append helpers: `AppendU24LE`, `AppendU24BE`, `AppendULEB128`,
  `AppendSLEB128`, plus `ULEB128Size`/`SLEB128Size` for exact buffer sizing.
- `Cursor.Align` skips and `Writer.Align` writes padding up to a size multiple,
  relative to the start of the buffer.

## v0.3.0 - 2026-09-22

- `Writer`: append-style encoding for scalars, varints and strings, with
  `io.Writer`/`io.ByteWriter`/`io.StringWriter`/`io.WriterTo` interop plus
  `Grow`, `Truncate`, `Reset` and `Len`.
- Length-prefixed sections: `Reserve` with `PatchU8`/`PatchU16LE`/`PatchU16BE`/
  `PatchU32LE`/`PatchU32BE`/`PatchU64LE`/`PatchU64BE`, and the closure helpers
  `LenU8`, `LenU16LE`, `LenU16BE`, `LenU32LE`, `LenU32BE`.
- `StreamWriter`: buffered writes to an `io.Writer` with sticky errors,
  automatic flushing (`WithFlushThreshold`), explicit `Flush` and `Grow`.
  `Reserve` suspends automatic flushing while a reserve is open, and
  `Write`/`WriteByte`/`WriteString` return the sticky flush error.
- Writing panics on values that do not fit their encoding (a 24-bit write above
  `0xFFFFFF`), on `CStr` values containing NUL, and on patches outside the
  buffer.
- New round-trip fuzz targets and writer benchmarks.
- Usage scenarios in the README plus runnable godoc examples for records,
  chunks, streaming and framed writing; the test suite now builds on 32-bit
  platforms.

## v0.2.0 - 2026-09-21

- `NewCursorFromReader` plus `Read`/`ReadByte`, so `Cursor` implements
  `io.Reader` and `io.ByteReader`.
- Iterators: `Records`, `IndexedRecords` and `Chunks` (`iter.Seq`/`iter.Seq2`,
  zero allocations per iteration). Requires Go 1.23.
- `Stream`: incremental parsing from an `io.Reader` with `Fill`, `Cursor`,
  `Advance`, `Discard`, `Buffered`, `Err`, `WithMaxBuffer` and
  `ErrBufferLimit`. Existing cursor hot paths are unchanged.

## v0.1.0 - 2026-09-21

Initial release.

- `Cursor` readers: `U8`, `U16`/`U32`/`U64` with `binary.ByteOrder` plus
  `LE`/`BE` wrappers, `I8`-`I64`, `U24`/`I24`, `F32`/`F64`.
- Navigation: `Pos`, `BytesLeft`, `CanRead`, `Ensure`, `Skip`, `Bytes`, `Peek`,
  `Sub`.
- Strings: `CStr`, `CStrOrRest`, `StrOrRest`, `RawStr`, `StrUnsafe` and
  `ErrNoNul`.
- Varints: `ULEB128` and `SLEB128` with offset rollback on malformed input.
- Panic-based bounds contract with zero-allocation numeric reads.
- Benchmarks, fuzz targets, godoc examples and the `cmd/btdebug` playground.
