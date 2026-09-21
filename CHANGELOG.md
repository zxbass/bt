# Changelog

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
