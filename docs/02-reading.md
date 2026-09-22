# 2. Reading with `Cursor`

`Cursor` is a sequential reader over a byte slice. It is three words wide
(slice header plus offset) and every read is a bounds check plus a load. This
chapter walks through the API, and explains the implementation details that
leak into its behaviour.

## 2.1 Creating a cursor and tracking position

```go
c := bt.NewCursor(data)

c.Pos()       // current offset
c.BytesLeft() // len(data) - Pos()
```

Cursors start at offset 0, and there is no way to move backwards. If you need
random access, keep the offsets you care about and use `Sub`/`Peek` or a second
cursor.

## 2.2 Validation: `CanRead`, `Ensure`, and the panic boundary

```go
if !c.CanRead(7) {
	return io.ErrUnexpectedEOF
}

id := c.U16LE()
flags := c.U8()
price := c.F32LE()
```

- `CanRead(n)` returns `false` for `n < 0` and for `n > BytesLeft()`.
- `Ensure(n)` is `CanRead` in panic form and is intended for invariant
  checks inside a parser.

`need` is deliberately a single comparison (`uint(n) > uint(len(b)-off)`), so
even validation stays cheap. Measured cost of the panic contract on the reader
side is about 1 ns per numeric read (see chapter 7).

## 2.3 Navigation

| Method | Effect | Notes |
| --- | --- | --- |
| `Skip(n)` | advance by `n` | panics if not enough bytes |
| `Bytes(n)` | return next `n` bytes, advance | result aliases, cap = `n` |
| `Peek(n)` | return next `n` bytes | does not advance, cap = `n` |
| `Sub(n)` | `*Cursor` over next `n` bytes, advance parent | aliases, allocates |
| `Align(size)` | advance to next multiple of `size` | returns padding bytes |

`Bytes` and `Peek` cap the result so it cannot be resliced past its window:

```go
raw := c.Peek(4) // len(raw) == 4, cap(raw) == 4

// raw[0:8] panics at runtime instead of reading neighbouring memory,
// which turns a reslice bug into an immediate, local failure.
```

`Align` is relative to the **start of the buffer**, not to the cursor's
current position:

```go
c := bt.NewCursor([]byte{0xAA, 0, 0, 0, 0xBB})
c.U8()            // offset 1
pad := c.Align(4) // 3, offset 4
_ = c.U8()        // 0xBB
```

A non-positive size panics with `bt: bad align size N`; if the padding extends
past the end, `Align` panics like any other read.

## 2.4 Scalars and byte order

Reads come in three shapes:

```go
c.U16(btOrder) // 16-bit in an arbitrary binary.ByteOrder
c.U16LE()      // little-endian
c.U16BE()      // big-endian
```

Everything unsigned has a signed counterpart (`I8`, `I16LE`, ...), and every
`I*` is a reinterpretation of the unsigned read (`int16(c.U16LE())`), which is
well-defined in Go for two's complement.

Floats reinterpret the bits:

```go
bits := c.U32LE()
f := math.Float32frombits(bits) // what F32LE does internally
```

`F32(order)`, `F32LE/BE`, `F64(order)`, `F64LE/BE` are available.

### 24-bit reads

`U24LE`/`U24BE` return `uint32` and are implemented with a fast path:

```go
func (c *Cursor) U24LE() uint32 {
	c.need(3)
	if c.off+4 <= len(c.b) {
		v := binary.LittleEndian.Uint32(c.b[c.off:]) & 0xFFFFFF
		c.off += 3
		return v
	}
	// fall back to three byte loads for the last three bytes of a slice
	...
}
```

When a fourth byte exists, one unaligned 32-bit load plus a mask beats three
separate loads; the offset still advances by only 3. `I24` sign-extends:

```go
func signExtend24(v uint32) int32 {
	if v&0x800000 != 0 {
		return int32(v | 0xFF000000)
	}
	return int32(v)
}
```

### Why `LE`/`BE` wrappers duplicate the generic bodies

`U16(order)` goes through an interface, which costs an interface dispatch and
blocks some inlining. The `LE`/`BE` wrappers exist because the concrete byte
order lets the compiler inline the whole read into the caller:

```go
c.U16(binary.BigEndian) // dynamic-ish: ~1.5 ns more than LE/BE on the i3 run
c.U16BE()               // fully inlinable
```

Use the wrappers on hot paths and the `order` forms where the endianness is a
runtime parameter.

## 2.5 Strings

There are two package-level helpers and three methods; all of them work on a
window whose size you provide, except the package helpers.

```go
// Package level: scan for a NUL.
s, err := bt.CStr(buf)       // ErrNoNul if no terminator
s = bt.CStrOrRest(buf)       // whole buffer if no terminator

// Method level: window of known size.
c.RawStr(8)     // copies 8 bytes into a string, keeps NULs
c.StrOrRest(8)  // copies, stops at the first NUL inside the window
c.StrUnsafe(8)  // no copy, aliases the buffer, keeps NULs
```

Costs, measured on the i3 run in the README:

| Call | Time | Allocs |
| --- | --- | --- |
| `RawStr(4)` | ~30 ns | 1 |
| `StrUnsafe(4)` | ~13 ns | 0 |

`StrUnsafe` uses `unsafe.String(unsafe.SliceData(b), n)`; the result is only
valid while the underlying buffer is alive and unmodified. Convert it with
`strings.Clone` if it must outlive the buffer.

Use `CStrOrRest`/`StrOrRest` when reading fixed-size name fields that may or
may not be NUL-terminated (RIFF, ELF string tables, tar): one pass, no
surprises.

## 2.6 Sub-cursors and the allocation caveat

`Sub(n)` carves a window out of the parent and advances it:

```go
for c.BytesLeft() > 0 {
	record := c.Sub(int(c.U16LE()))
	kind := record.U8()
	name := record.StrOrRest(record.BytesLeft())
	_ = kind
	_ = name
}
```

Reads inside `record` cannot move the parent offset, and `record` cannot read
past its window even if its own size math is wrong.

The caveat: `Sub` returns `*Cursor` and is **not inlinable** (its inline cost
is 106 against a budget of 80), so it allocates a 32-byte cursor per call.
`BenchmarkSubLoop` on the Ryzen machine measures 0.73 GB/s and 1 allocation
per record. When that matters:

- use `Chunks`/`Records` instead (zero allocations, chapter 5),
- or parse in place with `Skip`/`Bytes`.

## 2.7 `io` interop

`Cursor` implements `io.Reader` and `io.ByteReader`:

```go
n, err := c.Read(buf)     // 0, io.EOF at the end; never panics
b, err := c.ReadByte()    // 0, io.EOF at the end; never panics
```

These are the only read methods that return errors instead of panicking; they
exist so a cursor can be plugged into `io.Copy`, `bufio.Scanner`, `gzip`, etc.
`NewCursorFromReader(r)` buffers a whole reader and returns a regular cursor
(use `Stream` for incremental parsing).

```go
c, err := bt.NewCursorFromReader(resp.Body)
if err != nil {
	return err
}
```

## 2.8 Putting it together: a header parser

```go
func parseHeader(data []byte) (Header, error) {
	c := bt.NewCursor(data)
	if !c.CanRead(12) {
		return Header{}, io.ErrUnexpectedEOF
	}

	var h Header
	h.Magic = c.U32BE()
	h.Version = c.U16BE()
	h.Flags = c.U16BE()
	h.Length = c.U32BE()

	if !c.CanRead(int(h.Length)) {
		return Header{}, fmt.Errorf("payload length %d exceeds buffer", h.Length)
	}
	h.Name = c.StrOrRest(8)
	_ = c.Align(4) // payload starts on a 4-byte boundary
	return h, nil
}
```

Note the shape: validate once per region, then read without error checks. Every
read is a load plus a comparison, and the only explicit error paths are the
ones that carry information the caller needs.
