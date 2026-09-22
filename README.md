# bt

[![CI](https://github.com/zxbass/bt/actions/workflows/ci.yml/badge.svg)](https://github.com/zxbass/bt/actions/workflows/ci.yml)

Fast helpers for reading and writing trusted binary data.

`bt` is built for parsers and encoders that already know their data is
well-formed: reads past the end of the buffer and writes that do not fit their
encoding **panic** instead of returning an error, so there is no `if err != nil`
noise on every field. You validate sizes up front with `CanRead`/`Ensure`, and a
panic then means a bug in the parser, not malformed input.

```go
c := bt.NewCursor(data)

if !c.CanRead(7) {
	return fmt.Errorf("truncated header")
}

id := c.U16LE()
flags := c.U8()
price := c.F32LE()
```

The same fields come back out with `Writer`:

```go
w := bt.NewWriter()

w.U16LE(id)
w.U8(flags)
w.F32LE(price)
```

## Install

```
go get github.com/zxbass/bt
```

Requires Go 1.23+ (iterators).

## API

| Group | Methods |
| --- | --- |
| Navigation | `Pos`, `BytesLeft`, `CanRead`, `Ensure`, `Skip`, `Align`, `Bytes`, `Peek`, `Sub` |
| Unsigned | `U8`, `U16`/`U32`/`U64` (`order` + `LE`/`BE`), `U24LE`, `U24BE` |
| Signed | `I8`, `I16`/`I32`/`I64` (`order` + `LE`/`BE`), `I24LE`, `I24BE` |
| Floats | `F32`/`F64` (`order` + `LE`/`BE`) |
| Varints | `ULEB128`, `SLEB128` |
| Strings | `CStr`, `CStrOrRest`, `StrOrRest`, `RawStr`, `StrUnsafe` |
| Iterators | `Records`, `IndexedRecords`, `Chunks` |
| Streams | `Stream`: `Fill`, `Cursor`, `Advance`, `Discard`, `Buffered`, `Err`, `WithMaxBuffer` |
| `io` interop | `NewCursorFromReader`, `Read`, `ReadByte` |
| Errors | `ErrNoNul` (used by `CStr`), `ErrBufferLimit` (used by `Stream`) |
| Writing | `Writer`: same numeric/varint methods as `Cursor`, plus `RawStr`, `CStr` |
| Writer state | `NewWriter`, `Len`, `Bytes`, `Reset`, `Grow`, `Truncate`, `Align`, `Write`, `WriteByte`, `WriteString`, `WriteTo` |
| Sections | `Reserve`, `PatchU8`, `PatchU16LE`/`PatchU16BE`, `PatchU32LE`/`PatchU32BE`, `PatchU64LE`/`PatchU64BE`, `LenU8`, `LenU16LE`/`LenU16BE`, `LenU32LE`/`LenU32BE` |
| Append helpers | `AppendU24LE`, `AppendU24BE`, `AppendULEB128`, `AppendSLEB128`, `ULEB128Size`, `SLEB128Size` |
| Stream writing | `StreamWriter`: the same write methods, `Flush`, `Err`, `Buffered`, `Reset`, `Grow`, `WithFlushThreshold` |

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

## Writing

`Writer` appends encoded values to a byte slice. Invalid values panic and
nothing allocates while the capacity lasts; `Grow` reserves room up front:

```go
w := bt.NewWriter()
w.Grow(64)

w.U16LE(id)
w.CStr(name)
w.SLEB128(delta)
data := w.Bytes()
```

Length-delimited records use `Reserve` with a `Patch` method, or the
`LenU8`/`LenU16LE`/`LenU16BE`/`LenU32LE`/`LenU32BE` helpers, which run a closure
and patch the prefix for you:

```go
off := w.Reserve(2)
start := w.Len()
w.RawStr("payload")
w.PatchU16LE(off, uint16(w.Len()-start))

w.LenU16LE(func(w *bt.Writer) {
	w.CStr("name")
	w.U32LE(42)
})
```

`RawStr` writes a string as-is (NUL bytes included), while `CStr` appends a NUL
and panics on embedded NULs so that a round trip cannot silently lose data.
`Writer` also implements `io.Writer`, `io.ByteWriter`, `io.StringWriter` and
`io.WriterTo`; `WriteTo` drains the buffer like `bytes.Buffer` does.

Without a `Writer`, `AppendU24LE`/`AppendU24BE`/`AppendULEB128`/`AppendSLEB128`
append to an existing slice, and `ULEB128Size`/`SLEB128Size` report the encoded
length so you can grow the buffer exactly. For padded formats, `Cursor.Align`
skips and `Writer.Align` appends padding up to a size multiple, both relative
to the start of the buffer.

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

`StreamWriter` is the write-side counterpart: it buffers into an `io.Writer`
with the same error model. The typed write methods never return errors;
failures are sticky and surface through `Err`/`Flush`, while `Write`,
`WriteByte` and `WriteString` also return the sticky error, including a flush
failure caused by the call itself. After the first error every write is a no-op
until `Reset`. Buffered bytes are flushed automatically once they reach
`WithFlushThreshold(n)` (64 KiB by default); values `<= 0` switch to manual
`Flush` only.

```go
sw := bt.NewStreamWriter(out)
sw.U16LE(kind)
sw.RawStr(payload)
if err := sw.Flush(); err != nil {
	return err
}
```

`Reserve` suspends automatic flushing while any reserve is open, so the reserved
positions and the body between them stay in the buffer. A `Patch` inside the
reserved range releases it; flushing resumes on the next write or an explicit
`Flush`.

For one-shot use, `NewCursorFromReader(r)` reads the whole stream and returns a
regular `Cursor`. `Cursor` also implements `io.Reader` and `io.ByteReader`
(`Read`/`ReadByte` return `io.EOF` instead of panicking).

## Contract

- Out-of-bounds reads panic: `bt: need 4 bytes at offset 2, have 1`.
- Negative sizes panic: `bt: negative size -1`.
- A failed read leaves the cursor offset unchanged, including `ULEB128`/`SLEB128`.
- A value that does not fit its encoding panics, for example
  `bt: value 0x1000000 does not fit in 24 bits`. `CStr` panics on embedded NULs,
  patches outside the buffer panic, a non-positive alignment size panics, and a
  section longer than its prefix panics.
- `Cursor` aliases the buffer: it does not copy, the buffer must outlive the
  cursor, and `Bytes`/`Peek`/`Sub` results alias it too. `Bytes`/`Peek` cap the
  result at `n`, so it cannot be resliced past the requested window.
- `Writer.Bytes` aliases the writer's buffer and is invalidated by the next
  write that grows it.
- `RawStr` copies, `StrUnsafe` does not (the string is only valid while the
  buffer is alive and unmodified).
- `StreamWriter` drops the buffered bytes when a flush fails; the error is
  sticky and the io methods return it. `Flush` with an open `Reserve` panics,
  and a `Patch` only releases a reserve, it does not flush.
- No type is safe for concurrent use.

## Usage scenarios

Parse a length-delimited record: validate the header once, then work on a
sub-cursor so a bad record cannot move the parent offset.

```go
if !c.CanRead(3) {
	return io.ErrUnexpectedEOF
}
rec := c.Sub(int(c.U16LE()))
kind := rec.U8()
name := rec.StrOrRest(rec.BytesLeft())
```

Iterate fixed-size frames without allocations; `IndexedRecords` adds the frame
index, `Chunks` yields zero-copy slices for hashing or copying.

```go
for i, rec := range c.IndexedRecords(16) {
	_ = i
	_ = rec.U32LE()
}

for chunk := range c.Chunks(1 << 20) {
	h.Write(chunk)
}
```

Parse a stream incrementally: `Fill` guarantees a window, `Cursor` parses inside
it, `Advance` commits what was consumed. `Fill` reports truncation as
`io.ErrUnexpectedEOF`, and `WithMaxBuffer` caps memory.

```go
st := bt.NewStream(r)
for {
	if err := st.Fill(1); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) && st.Buffered() == 0 {
			return nil
		}
		return err
	}
	size := int(st.Cursor().U8())
	st.Advance(1)

	if err := st.Fill(size); err != nil {
		return err
	}
	body := st.Cursor().RawStr(size)
	st.Advance(size)
	handle(body)
}
```

Build a wire format with length-prefixed sections: `LenU16LE` runs a closure and
patches the prefix, while `Reserve`/`Patch` cover fields written after the body,
such as a trailing checksum.

```go
w := bt.NewWriter()
w.Grow(1 << 10)

w.LenU16LE(func(w *bt.Writer) {
	w.CStr(name)
	w.U32LE(id)
})

off := w.Reserve(4)
w.RawStr(body)
w.PatchU32LE(off, crc32.ChecksumIEEE(w.Bytes()[off+4:]))
```

Write large payloads to an `io.Writer` without checking errors on every field:
failures are sticky and surface once through `Flush`, and
`WithFlushThreshold(n)` batches the writes.

```go
sw := bt.NewStreamWriter(out, bt.WithFlushThreshold(1<<20))
for _, rec := range records {
	sw.U16LE(rec.id)
	sw.RawStr(rec.name)
}
if err := sw.Flush(); err != nil {
	return err
}
```

Round-trip tests: every `Writer` method has a matching `Cursor` reader, so a
table or fuzz test can write a value, read it back and treat panics as
failures.

```go
w := bt.NewWriter()
w.U24LE(v)
if got := bt.NewCursor(w.Bytes()).U24LE(); got != v {
	t.Fatalf("round trip %d = %d", v, got)
}
```

The runnable versions live in the package examples: `ExampleCursor_Sub`,
`ExampleCursor_IndexedRecords`, `ExampleCursor_Chunks`, `ExampleStream`,
`ExampleWithMaxBuffer`, `ExampleWriter_Reserve`, `ExampleWriter_LenU16LE`,
`ExampleStreamWriter`, `ExampleStreamWriter_LenU16BE` and
`ExampleWriter_WriteTo`.

## Performance

`go test -bench=. -benchmem`, amd64 (i3-10100, Go 1.26). Absolute numbers are
machine-dependent; the comparison columns are from the same run.

| operation | bt | `encoding/binary` |
| --- | --- | --- |
| `U16LE` | 4.2 ns, 0 allocs | 3.0 ns direct, 35 ns + 1 alloc via `binary.Read` |
| `U32LE` | 4.0 ns, 0 allocs | 3.0 ns direct, 56 ns + 1 alloc |
| `U64LE` | 4.1 ns, 0 allocs | 3.0 ns direct, 60 ns + 1 alloc |
| `ULEB128` (1/2/10 bytes) | 2.2 / 2.4 / 6.4 ns | — |
| `SLEB128` (1/2/10 bytes) | 2.3 / 2.8 / 6.7 ns | — |
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
native-endian load with no bounds check. The varint rows were re-measured on the
machine in the Writing table after the unrolled fast paths.

### Writing

`go test -run=^$ -bench=. -benchmem`, amd64 (Ryzen 5 5600, Go 1.27). These
numbers come from a different machine and run than the reader table above.

| operation | bt | stdlib |
| --- | --- | --- |
| `U16LE` | 2.4 ns, 0 allocs | 0.7 ns `binary.LittleEndian.AppendUint16` |
| `U16(order)` / `U32(order)` / `U64(order)` | 2.5 / 2.7 / 2.5 ns, 0 allocs | — |
| `U32BE` | 2.2 ns, 0 allocs | 0.7 ns `binary.BigEndian.AppendUint32` |
| `U64LE` | 2.5 ns, 0 allocs | 0.8 ns `binary.LittleEndian.AppendUint64` |
| `U24LE` | 3.4 ns, 0 allocs | — |
| `Writer.ULEB128` / `Writer.SLEB128` | 4.7 / 4.7 ns, 0 allocs | — |
| `AppendULEB128` | 3.7 ns, 0 allocs | 3.0 ns `binary.AppendUvarint` |
| `AppendU24LE` | 2.4 ns, 0 allocs | — |
| `RawStr(8)` / `CStr(8)` | 2.1 / 7.6 ns, 0 allocs | — |
| `LenU8` section with an 8-byte payload | 9.7 ns, 0 allocs | — |
| 1024 mixed records (`WriteRecords`) | 3.8 GB/s, ~5.7 ns/record, 0 allocs | — |
| 16-byte records via `StreamWriter` (4096 rec) | ~45 µs, 0 allocs | ~26 µs `bufio.Writer` |

The writer methods cost 1.5-2 ns more than a bare `AppendUint*` call because
every write updates the writer slice header through a pointer; the value checks
are branches, not allocations. The generic `U16(order)`/`U32(order)`/`U64(order)`
forms cost little extra when the byte order is a compile-time constant, because
the compiler devirtualizes the call. `StreamWriter` pays per-field method
overhead on top, so batching raw records through `bufio.Writer` is faster; it
buys the sticky-error, no-`if err != nil` API and the section helpers.
`StreamWriter.Grow` removes the few bytes of buffer-growth allocation.

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
go test -run=^$ -fuzz=FuzzWriterScalarsRoundTrip -fuzztime=30s .
go test -run=^$ -bench=. -benchmem ./...
```

The full fuzz matrix lives in `.github/workflows/ci.yml`.

Compare benchmark changes with `benchstat`:

```
go test -run=^$ -bench=. -count=10 . | tee /tmp/old.txt
# apply changes
go test -run=^$ -bench=. -count=10 . | tee /tmp/new.txt
go run golang.org/x/perf/cmd/benchstat@latest /tmp/old.txt /tmp/new.txt
```

## License

MIT
