# bt

[![CI](https://github.com/zxbass/bt/actions/workflows/ci.yml/badge.svg)](https://github.com/zxbass/bt/actions/workflows/ci.yml)

Fast helpers for reading trusted binary data from byte slices.

`bt` is built for parsers that already know their input is well-formed: reads
past the end of the buffer **panic** instead of returning an error, so there is
no `if err != nil` noise on every field. You validate sizes up front with
`CanRead`/`Ensure`, and a panic then means a bug in the parser, not malformed
input.

```go
c := bt.NewCursor(data)

if !c.CanRead(7) {
	return fmt.Errorf("truncated header")
}

id := c.U16LE()
flags := c.U8()
price := c.F32LE()
```

## Install

```
go get github.com/zxbass/bt
```

Requires Go 1.23+ (iterators).

## API

| Group | Methods |
| --- | --- |
| Navigation | `Pos`, `BytesLeft`, `CanRead`, `Ensure`, `Skip`, `Bytes`, `Peek`, `Sub` |
| Unsigned | `U8`, `U16`/`U32`/`U64` (`order` + `LE`/`BE`), `U24LE`, `U24BE` |
| Signed | `I8`, `I16`/`I32`/`I64` (`order` + `LE`/`BE`), `I24LE`, `I24BE` |
| Floats | `F32`/`F64` (`order` + `LE`/`BE`) |
| Varints | `ULEB128`, `SLEB128` |
| Strings | `CStr`, `CStrOrRest`, `StrOrRest`, `RawStr`, `StrUnsafe` |
| Iterators | `Records`, `IndexedRecords`, `Chunks` |
| Streams | `Stream`: `Fill`, `Cursor`, `Advance`, `Discard`, `Buffered`, `Err`, `WithMaxBuffer` |
| `io` interop | `NewCursorFromReader`, `Read`, `ReadByte` |
| Errors | `ErrNoNul` (used by `CStr`), `ErrBufferLimit` (used by `Stream`) |

`Sub(n)` returns an independent cursor over the next `n` bytes and advances the
parent. Use it to parse a length-delimited record without letting its reads
affect the parent offset:

```go
record := c.Sub(int(c.U16LE()))
kind := record.U8()
name := record.StrOrRest(record.BytesLeft())
```

## Iterators

Fixed-size records and zero-copy chunks, with no allocations per iteration:

```go
for rec := range c.Records(recSize) {
	id := rec.U16LE()
}

for i, rec := range c.IndexedRecords(recSize) {
	_ = i
	_ = rec
}

for chunk := range c.Chunks(1 << 20) {
	h.Write(chunk)
}
```

Iteration consumes the parent cursor. A size that is not positive or a trailing
partial record panics.

## Streaming

`Stream` adapts an `io.Reader`: `Fill` guarantees a window of bytes, and the
usual `Cursor` parses inside it. I/O errors are returned by `Fill` (sticky,
`Err()` exposes `io.EOF`); parsing errors are still panics.

```go
st := bt.NewStream(r)
for {
	if err := st.Fill(4); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) && st.Buffered() == 0 {
			break
		}
		return err
	}
	c := st.Cursor()
	kind := c.U8()
	size := int(c.U16LE())
	st.Advance(3)

	if err := st.Fill(size); err != nil {
		return err
	}
	body := st.Cursor().Bytes(size)
	st.Advance(size)
	_ = kind
	_ = body
}
```

Cursors from `Cursor()` are invalidated by the next `Fill`/`Advance` because the
window may move or compact. `WithMaxBuffer(n)` caps memory and makes `Fill`
return `ErrBufferLimit` instead of growing further.

For one-shot use, `NewCursorFromReader(r)` reads the whole stream and returns a
regular `Cursor`. `Cursor` also implements `io.Reader` and `io.ByteReader`
(`Read`/`ReadByte` return `io.EOF` instead of panicking).

## Contract

- Out-of-bounds reads panic: `bt: need 4 bytes at offset 2, have 1`.
- Negative sizes panic: `bt: negative size -1`.
- A failed read leaves the cursor offset unchanged, including `ULEB128`/`SLEB128`.
- `Cursor` aliases the buffer: it does not copy, the buffer must outlive the
  cursor, and `Bytes`/`Peek`/`Sub` results alias it too. `Bytes`/`Peek` cap the
  result at `n`, so it cannot be resliced past the requested window.
- `RawStr` copies, `StrUnsafe` does not (the string is only valid while the
  buffer is alive and unmodified).
- `Cursor` is not safe for concurrent use.

## Performance

`go test -bench=. -benchmem`, amd64 (i3-10100, Go 1.26). Absolute numbers are
machine-dependent; the comparison columns are from the same run.

| operation | bt | `encoding/binary` |
| --- | --- | --- |
| `U16LE` | 4.2 ns, 0 allocs | 3.0 ns direct, 35 ns + 1 alloc via `binary.Read` |
| `U32LE` | 4.0 ns, 0 allocs | 3.0 ns direct, 56 ns + 1 alloc |
| `U64LE` | 4.1 ns, 0 allocs | 3.0 ns direct, 60 ns + 1 alloc |
| `ULEB128` (1/2/10 bytes) | 2.7 / 3.2 / 18.7 ns | — |
| `SLEB128` (1/2 bytes) | 2.9 / 3.3 ns | — |
| `RawStr(4)` / `StrUnsafe(4)` | 30 ns / 13 ns, 1 / 0 allocs | — |
| `ParseRecords` (mixed fields + name strings) | 461 MB/s, ~38 ns/record | — |
| `Records(16)` iteration (64 KiB) | ~6 GB/s, 0 allocs | — |
| `Chunks(16)` iteration (64 KiB) | ~11.7 GB/s, 0 allocs | — |
| `Sub(16)` loop (same data) | ~0.3 GB/s, 1 alloc/record | — |
| `Stream` over `bytes.Reader` (16-byte records) | ~1 GB/s, ~15 ns/record | — |
| `UnsafeCast` (no bounds check, native endian) | 0.54 ns | — |

The panic-on-out-of-bounds contract costs about 1 ns per numeric read versus a
bare `binary.LittleEndian` call. The dynamic `U16(order)`/`U32(order)`/... forms
cost ~1.5 ns more than the `LE`/`BE` wrappers because of interface dispatch.
`BenchmarkUnsafeCast` (amd64/arm64 only) is the theoretical floor: a raw
native-endian load with no bounds check.

## Debug playground

```
go run ./cmd/btdebug -demo
go run ./cmd/btdebug -hex "42 02 01 01 02" -seq "u8,u16le,u16be"
printf '\x01\x02\x03\x04' | go run ./cmd/btdebug -seq "u32le,sleb"
```

The `-seq` flag accepts `u8`, `u16le`/`u16be`, `u32le`/`u32be`, `u64le`/`u64be`,
`i8`, `i16le`/`i16be`, `i24le`/`i24be`, `u24le`/`u24be`, `f32le`/`f32be`,
`f64le`/`f64be`, `uleb`, `sleb`, `str:N`, `rawstr:N`, `strunsafe:N`, `cstr:N`,
`bytes:N`, `peek:N`, `skip:N`, `sub:N`, `ensure:N`, `canread:N`, `pos` and
`hex`.

## Development

```
go test -race -cover ./...
go test -run=^$ -fuzz=FuzzULEB128RoundTrip -fuzztime=30s .
go test -run=^$ -bench=. -benchmem ./...
```

Compare benchmark changes with `benchstat`:

```
go test -run=^$ -bench=. -count=10 . | tee /tmp/old.txt
# apply changes
go test -run=^$ -bench=. -count=10 . | tee /tmp/new.txt
go run golang.org/x/perf/cmd/benchstat@latest /tmp/old.txt /tmp/new.txt
```

## License

MIT
