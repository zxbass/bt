# 3. Writing with `Writer`

`Writer` is the write-side mirror of `Cursor`: it appends encoded values to a
growing byte slice. It never returns errors, it panics on values that do not
fit, and it allocates only when the buffer has to grow.

## 3.1 The append model

```go
w := bt.NewWriter()

w.U16LE(0x1234)
w.CStr("dev")
w.SLEB128(-5)

data := w.Bytes() // aliases the writer buffer
```

State helpers:

| Method | Effect |
| --- | --- |
| `Len()` | number of buffered bytes |
| `Bytes()` | the buffer, aliased |
| `Reset()` | drop bytes, **keep capacity** |
| `Grow(n)` | ensure room for `n` more bytes without growing |
| `Truncate(n)` | drop everything past the first `n` bytes |
| `Align(size)` | append zeros to the next size multiple, return count |

`Grow` is the allocation switch: call it once for the typical record size and
the hot loop stays allocation-free.

```go
w := bt.NewWriter()
w.Grow(64)

for _, rec := range records {
	w.Reset() // capacity survives; reuse for the next record
	encode(w, rec)
	handle(w.Bytes())
}
```

`Reset` after `Grow` means one allocation for a whole batch. `Truncate` is for
rolling back a partially written record without losing the capacity:

```go
start := w.Len()
if err := tryEncode(w, rec); err != nil {
	w.Truncate(start) // roll back, keep capacity
}
```

`Align` is the write-side counterpart of `Cursor.Align`, relative to the start
of the buffer:

```go
w := bt.NewWriter()
w.U8(0xAA)
w.Align(4)       // appends 3 zeros, returns 3
w.U32LE(0xDEAD)  // starts at offset 4
```

## 3.2 Scalars

The numeric surface mirrors `Cursor`: `U8`, `U16(order)`, `U16LE/BE`,
`U32`, `U64`, signed `I*`, floats `F32/F64`, 24-bit `U24LE/BE` and `I24LE/BE`.

Values must fit their encoding:

```go
w.U24LE(0xFFFFFF) // fine
w.U24LE(0x1000000)
// panic: bt: value 0x1000000 does not fit in 24 bits
```

The same rules apply to signed 24-bit writes (`I24LE` accepts
`[-0x800000, 0x7FFFFF]`) and to `CStr`.

## 3.3 Strings: `RawStr` vs `CStr`

```go
w.RawStr("a\x00b") // writes a\0b, NUL bytes allowed
w.CStr("a\x00b")
// panic: bt: CStr contains NUL byte
```

`CStr` appends a terminating NUL and refuses to write a string that already
contains one. The reason is round-trip safety: `Cursor.CStr`/`StrOrRest` stop
at the first NUL, so an embedded NUL would silently truncate the value on read.
Panicking at write time moves the bug to where it is cheap to find.

`WriteString` is the `io.StringWriter` alias for `RawStr`; use whichever reads
better, the bytes are identical.

## 3.4 `io` interop and `WriteTo`

`Writer` implements `io.Writer`, `io.ByteWriter`, `io.StringWriter` and
`io.WriterTo`. The first three always succeed:

```go
n, err := w.Write([]byte("raw")) // n == 4, err == nil
err = w.WriteByte(0x00)          // nil
```

`WriteTo` follows `bytes.Buffer` semantics: it writes the buffered bytes to
`dst` and **drains** them, keeping the capacity.

```go
n, err := w.WriteTo(file)
// w.Len() == 0 on success; w.Bytes() may still be reused via Reset
```

Failure behaviour:

- partial write + error: written bytes are removed, the unwritten tail stays in
  the buffer so the caller can retry;
- short write with no error: returns `io.ErrShortWrite`;
- a `Write` implementation that returns `n > len(p)` or `n < 0` panics with
  `bt: invalid Write count` (mirroring `bytes.Buffer`).

`WriteTo` does one `Write` call with the whole buffer, which is the fastest way
to hand a large encoded record to a file or socket. For per-field streaming
use `StreamWriter` (chapter 6).

## 3.5 Length-prefixed sections

Binary formats constantly need "write a length, then the body". `bt` supports
both explicit patching and closure helpers.

### Explicit: `Reserve` + `Patch*`

```go
w := bt.NewWriter()

nameOff := w.Reserve(2)        // two zero bytes, returns the offset
nameStart := w.Len()
w.RawStr(name)
w.PatchU16LE(nameOff, uint16(w.Len()-nameStart))
```

`Reserve(n)` appends `n` zero bytes and returns the offset. `PatchU8`,
`PatchU16LE/BE`, `PatchU32LE/BE`, `PatchU64LE/BE` overwrite at an offset:

```go
w.PatchU32BE(crcOff, crc)
```

Patching outside the buffer panics:

```
bt: patch at offset 5 out of range (len 4)
```

This is the tool for checksums, back-references and any field whose value is
known only after the body is written.

### Closure: `LenU8`, `LenU16LE/BE`, `LenU32LE/BE`

```go
w.LenU16LE(func(w *bt.Writer) {
	w.CStr("name")
	w.U32LE(42)
})
```

The helper reserves the prefix, runs the closure, then patches the payload
length. Sections nest naturally:

```go
w.LenU8(func(w *bt.Writer) {
	w.U8(1)
	w.LenU8(func(w *bt.Writer) { w.RawStr("inner") })
})
```

Length checks happen after the body is written: if the payload does not fit the
chosen prefix, the write panics instead of truncating.

```
bt: section length 256 does not fit in 8 bits
```

`sectionLen` takes a `uint64` on purpose. On 32-bit platforms an `int` length
cannot exceed `MaxInt32`, and constant tests like `1<<32` do not even compile
if the parameter is `int`; the wider type keeps the guard portable (see
chapter 8 for the CI job that caught this).

## 3.6 Stateless appends

`append.go` adds the encodings that `encoding/binary` does not cover, with the
same shape as `binary.LittleEndian.AppendUint16`:

```go
buf = bt.AppendU24LE(buf, v)
buf = bt.AppendU24BE(buf, v)
buf = bt.AppendULEB128(buf, v)
buf = bt.AppendSLEB128(buf, v)
```

Sizes are available for exact preallocation:

```go
buf = slices.Grow(buf, bt.ULEB128Size(v)+bt.SLEB128Size(d))
buf = bt.AppendULEB128(buf, v)
buf = bt.AppendSLEB128(buf, d)
```

Internally each helper wraps a stack `Writer`, so the encoding logic lives in
one place while `Writer.ULEB128` keeps its small inlinable body:

```go
func AppendULEB128(b []byte, v uint64) []byte {
	w := Writer{b: b}
	w.ULEB128(v)
	return w.b
}
```

The wrapper is itself inlinable (cost 39) and allocation-free when `b` has
capacity. `ULEB128Size` uses the `bits.Len64` formula described in chapter 4.

## 3.7 What `Writer` is not

- It does not report errors: if you need error-returning writes to an
  `io.Writer`, use `StreamWriter`.
- It is not safe for concurrent use.
- It does not own the underlying array: `Bytes()` aliases it, and appends may
  reallocate.
- It does not do alignment-aware padding automatically; call `Align` (or
  `Reserve`) explicitly.

For a complete encoder walkthrough, see the cookbook chapter, recipes 3-5.
