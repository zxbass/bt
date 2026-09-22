# 1. The contract

`bt` has one central idea: **the input is trusted, so mistakes are programmer
errors, not data errors.** That single decision shapes every API in the
package. This chapter states the three error models, the aliasing rules and the
lifetime rules you must respect.

## 1.1 Trusted input, panics, and `CanRead`

A classic parser looks like this:

```go
id, err := readU16(r)
if err != nil {
	return err
}
flags, err := readU8(r)
if err != nil {
	return err
}
```

The `bt` version looks like this:

```go
if !c.CanRead(3) {
	return fmt.Errorf("truncated header")
}

id := c.U16LE()
flags := c.U8()
```

Reads past the end of the buffer **panic**:

```
bt: need 4 bytes at offset 2, have 1
```

The intended workflow is:

1. Validate the size of a region once, with `CanRead`/`Ensure`.
2. Parse inside it without error checks.
3. Treat any panic as a bug in the parser: either the validation was wrong, or
   the data was not actually trusted.

This only pays off if you do step 1. If you skip it, one malformed input turns
into a process-wide panic. For untrusted data, keep validation at the boundary
(see section 1.6).

### Why panics are cheap

`Cursor.need` is a single unsigned comparison:

```go
func (c *Cursor) need(n int) {
	if uint(n) > uint(len(c.b)-c.off) {
		c.needPanic(n)
	}
}
```

The `uint` conversion folds two checks into one: a negative `n` becomes a huge
unsigned number, so `n = -1` fails the same comparison as an out-of-bounds
positive size. `needPanic` is marked `//go:noinline`, so the cold path never
pollutes the inlining decision of the hot path.

## 1.2 Three error models

| Situation | Model | Surface |
| --- | --- | --- |
| Read past the end, negative size | panic (string value) | `Cursor` methods |
| Value does not fit its encoding, bad patch, NUL in `CStr` | panic (string value) | `Writer` methods |
| I/O failure while streaming | sticky `error`, no panic | `Stream.Fill`, `StreamWriter.Flush` |

Panic values are always strings with a `bt: ` prefix; there are no custom panic
types to match on. Tests assert prefixes, and `cmd/btdebug` recovers them to
print diagnostics.

Streaming keeps the error model from `io`:

- `Stream.Fill` returns `io.ErrUnexpectedEOF` when the stream ends early, and
  the underlying error otherwise. The error is **sticky**: `Fill` keeps
  returning it, and `Err()` exposes the cause (`io.EOF` on a clean end).
- `StreamWriter` types record the first write error and turn every later write
  into a no-op; `Write`/`WriteByte`/`WriteString` return that error, while the
  typed methods surface it through `Err()`/`Flush()`.

## 1.3 Failed reads do not move the cursor

Every read either succeeds and advances the offset, or panics and leaves the
offset exactly where it was. This includes varints, whose multi-byte decoding
is where a naive implementation would leak a partial advance:

```go
c := bt.NewCursor([]byte{0x2A, 0x80})

_ = c.U8() // offset 1
// c.ULEB128() panics: byte 0x80 continues into nothing
// c.Pos() is still 1
```

The rule makes error recovery possible: after a panic from a read, the cursor
still points at the bytes that failed, so a caller that recovers can inspect
them. The same property is verified by fuzz tests.

## 1.4 Aliasing and lifetimes

`bt` never copies data unless the method name says so. That is the source of
its speed and the source of its sharpest edges.

| Value | Aliases? | Valid until |
| --- | --- | --- |
| `Cursor.Bytes(n)`, `Peek(n)`, `Sub(n)` | yes, the source buffer | the buffer is modified |
| `Chunks` slices | yes, the source buffer | the buffer is modified |
| `Writer.Bytes()` | yes, the writer buffer | the next write that grows it |
| `Cursor.RawStr` | no, copies | forever |
| `Cursor.StrOrRest`, `CStr`, `CStrOrRest` | no, copy | forever |
| `Cursor.StrUnsafe` | **yes**, aliases via `unsafe.String` | buffer alive and unmodified |

`Bytes`/`Peek` cap the returned slice at `n`:

```go
raw := c.Bytes(4)  // len 4, cap 4
// raw[:8] is not possible; a careless reslice cannot read past the window
```

`Writer.Bytes()` is the writable equivalent: keep it only until the next append
that reallocates:

```go
w := bt.NewWriter()
w.U32LE(1)
snapshot := w.Bytes() // aliases w's buffer
w.Grow(1 << 20)       // may reallocate; snapshot may be stale
```

`StrUnsafe` is deliberately named. The string it returns points into the
buffer; the compiler cannot check that for you.

## 1.5 Concurrency

No type in `bt` is safe for concurrent use. Two goroutines must not share a
`Cursor`, `Writer`, `Stream` or `StreamWriter`. The usual pattern is one parser
per goroutine over a read-only buffer; read-only access to the *same* byte
slice from multiple cursors is fine:

```go
data := load()
go func() { c := bt.NewCursor(data); parseIndex(c) }()
go func() { c := bt.NewCursor(data); parseTables(c) }()
```

`Cursor` has no internal cache or shared state, so separate cursors over one
buffer are independent.

## 1.6 When not to use `bt`

- **Untrusted input with no validation layer.** The panic contract converts
  data errors into crashes. Validate first, or use an error-returning parser.
- **Schema-heavy formats.** If you define the format in a `.proto`, use
  generated code: `bt` has no code generation and no schema.
- **Text formats.** JSON/CSV/XML have their own parsers.
- **Memory-mapped, lazy access.** `bt` parses slices you already have; it does
  not map files (see the cookbook for reading files into memory).

## 1.7 Panic message reference

| Message | Produced by |
| --- | --- |
| `bt: need N bytes at offset O, have H` | every read past the end, `Align` |
| `bt: negative size N` | negative sizes, `Grow`, `Truncate`, `Reserve` |
| `bt: value 0x... does not fit in 24 bits` | `U24LE/BE`, `I24LE/BE` (unsigned form) |
| `bt: value N does not fit in 24 bits` | `I24LE/BE` (signed form) |
| `bt: CStr contains NUL byte` | `Writer.CStr` |
| `bt: patch at offset N out of range (len M)` | `Patch*` |
| `bt: section length N does not fit in B bits` | `LenU8/LenU16*/LenU32*` |
| `bt: truncate size N out of range (len M)` | `Truncate` |
| `bt: bad align size N` | `Align` (size <= 0) |
| `bt: bad chunk size N` | `Records`/`IndexedRecords`/`Chunks` (size <= 0) |
| `bt: N trailing bytes do not fit M-byte chunks` | iterators with a partial tail |
| `bt: ULEB128 overflow` / `bt: SLEB128 overflow` | 10th varint byte out of range |
| `bt: invalid Write count` | `Writer.WriteTo` with a misbehaving `io.Writer` |
| `bt: Flush with open Reserve` | `StreamWriter.Flush` while a reserve is open |

The exact strings are part of the documented contract; tests assert them.
