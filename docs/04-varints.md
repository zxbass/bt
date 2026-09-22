# 4. Varints in depth

LEB128 ("little endian base 128") stores an integer in 7-bit groups, each byte
carrying a continuation flag in bit 7. Small values stay one byte, large values
grow to ten. It is the encoding behind protobuf field numbers, DWARF offsets
and WebAssembly indices.

## 4.1 The encoding in one table

| Value | `ULEB128` | `SLEB128` (DWARF) |
| --- | --- | --- |
| `0` | `00` | `00` |
| `1` | `01` | `01` |
| `63` | `3F` | `3F` |
| `64` | `40` | `C0 00` |
| `127` | `7F` | `FF 00` |
| `128` | `80 01` | `80 01` |
| `300` | `AC 02` | `AC 02` |
| `-1` | — | `7F` |
| `-64` | — | `40` |
| `-65` | — | `BF 7F` |
| `-300` | — | `D4 7D` |
| `MaxUint64` | `FF FF FF FF FF FF FF FF FF 01` | — |
| `MaxInt64` | — | `FF FF FF FF FF FF FF FF FF 00` |
| `MinInt64` | — | `80 80 80 80 80 80 80 80 80 7F` |

The unsigned rule is simple: while the remaining value does not fit in 7 bits,
emit `value | 0x80` and shift right by 7.

The signed rule adds sign extension: the encoding stops when the remaining
bits are all zero and bit 6 of the emitted byte is clear, or when the remaining
bits are all ones and bit 6 is set.

## 4.2 `SLEB128` is not zigzag

Go's `encoding/binary` uses **zigzag** for signed varints:

```go
binary.PutVarint(nil, -1) // 01
binary.PutVarint(nil, 1)  // 02
```

`bt.Cursor.SLEB128`/`Writer.SLEB128` implement the **DWARF** variant, which
preserves the sign bit convention described above:

```go
w := bt.NewWriter()
w.SLEB128(-1)             // 7F
```

The two are not interchangeable. If a format specifies zigzag, use
`binary.AppendVarint`/`binary.Varint`; if it specifies DWARF/protobuf
`int32`/`int64`, use `bt`. This is the single most common varint mistake; the
round-trip tests compare `ULEB128` against `binary.AppendUvarint` but test
`SLEB128` against an independent encoder, exactly because the standard library
would disagree.

## 4.3 Decoding contract

`Cursor.ULEB128`/`SLEB128` follow the package contract:

- a successful read advances the offset by 1-10 bytes and returns the value;
- a truncated varint panics with the usual bounds message and **does not move
  the offset**;
- a malformed tenth byte panics with `bt: ULEB128 overflow` /
  `bt: SLEB128 overflow`, leaving the offset unchanged.

```go
c := bt.NewCursor([]byte{0x2A, 0x80})
_ = c.U8() // offset 1
// c.ULEB128() panics
// c.Pos() == 1, the 0x80 is still there
```

## 4.4 How the fast paths are built

The most common varints are one or two bytes, so those live directly in the
public method:

```go
func (c *Cursor) ULEB128() (result uint64) {
	if c.off < len(c.b) {
		b0 := c.b[c.off]
		if b0 < 0x80 {
			c.off++
			return uint64(b0)
		}
		if c.off+1 < len(c.b) {
			b1 := c.b[c.off+1]
			if b1 < 0x80 {
				c.off += 2
				return uint64(b0&0x7F) | uint64(b1)<<7
			}
			return c.uleb128Rest()
		}
	}
	panic(c.needMessage(1))
}
```

Everything from three to ten bytes goes to a flat, fully unrolled helper
(`uleb128Rest`, `sleb128Rest`) instead of a loop:

```go
b := c.b[c.off:]
if len(b) < 3 {
	panic(c.needMessage(1))
}
v := uint64(b[0]&0x7F) | uint64(b[1]&0x7F)<<7
if b[2] < 0x80 {
	c.off += 3
	return v | uint64(b[2])<<14
}
v |= uint64(b[2]&0x7F) << 14
if len(b) < 4 {
	panic(c.needMessage(1))
}
...
```

Design notes:

- **Why a separate helper**: `ULEB128` is 292 inline-cost units (budget 80) and
  `SLEB128` is 391, so neither can be inlined anyway. Keeping the 1-2 byte
  path in the small public function and the tail in a cold helper keeps
  register pressure and code size on the hot path minimal.
- **Why flat and unrolled**: an unrolled chain of `if b[k] < 0x80` branches
  avoids the loop's per-iteration bookkeeping (bounds compare, shift counter,
  overflow test) and lets the CPU speculate across bytes. It is the same trick
  `google.golang.org/protobuf/encoding/protowire` uses in `ConsumeVarint`.
- **Why `panic(c.needMessage(1))` instead of `c.needPanic(1)`**:
  `needPanic` is a function call, and Go's terminating-statement analysis does
  not know it panics, so a function ending in `needPanic` would need an
  unreachable `return` that would show up as uncovered code. `needMessage`
  returns the formatted string and the `panic(...)` builtin is a terminating
  statement, so the helper ends cleanly and coverage stays at 100%.

Sign handling is the only difference for `sleb128Rest`: at each terminal byte,
if bit 6 is set, the high bits are sign-extended by subtracting
`1 << (7*(k+1))`; at the ninth byte the extension is `v |= -1 << 63`:

```go
if b[3] < 0x80 {
	if b[3]&0x40 != 0 {
		v -= 1 << 28
	}
	c.off += 4
	return v | int64(b[3])<<21
}
```

## 4.5 Encoding

Encoding is a loop because appending is inherently sequential and long varints
are rare:

```go
func (w *Writer) ULEB128(v uint64) {
	for v >= 0x80 {
		w.b = append(w.b, byte(v)|0x80)
		v >>= 7
	}
	w.b = append(w.b, byte(v))
}

func (w *Writer) SLEB128(v int64) {
	for {
		b := byte(v) & 0x7F
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			w.b = append(w.b, b)
			return
		}
		w.b = append(w.b, b|0x80)
	}
}
```

`Writer.ULEB128` is small enough to inline (cost 25), which is why the
`Append*` wrappers can reuse it for free (chapter 3).

## 4.6 Sizes without encoding

`ULEB128Size` and `SLEB128Size` compute the encoded length directly, using
`bits.Len64` (which compiles to the `LZCNT` instruction on modern CPUs):

```go
func ULEB128Size(v uint64) int {
	return (bits.Len64(v|1) + 6) / 7
}

func SLEB128Size(v int64) int {
	return (bits.Len64((uint64(v)^uint64(v>>63))|1) + 7) / 7
}
```

- `v|1` avoids `Len64(0) == 0` and lets the compiler drop the zero check.
- For signed values, `v ^ (v>>63)` maps `v` to the number of value bits while
  `|1` keeps the result positive; the extra `+7` accounts for the sign bit.
- Boundary checks: `SLEB128Size(63) == 1`, `SLEB128Size(64) == 2`,
  `SLEB128Size(-64) == 1`, `SLEB128Size(-65) == 2`, `SLEB128Size(MinInt64) == 10`.

These formulas come from protobuf-go's `protowire.SizeVarint`, generalized to
the signed variant.

## 4.7 Benchmarks

`benchstat` over ten runs, Ryzen 5 5600 (Go 1.27):

| Case | loop version | unrolled | delta |
| --- | --- | --- | --- |
| `ULEB128` 1 byte | 2.27 ns | 2.21 ns | −2.6% |
| `ULEB128` 2 bytes | 2.51 ns | 2.44 ns | −3.0% |
| `ULEB128` 3 bytes | 6.91 ns | 4.10 ns | −40.7% |
| `ULEB128` 5 bytes | 9.34 ns | 4.97 ns | −46.9% |
| `ULEB128` 10 bytes | 16.17 ns | 6.37 ns | −60.6% |
| `SLEB128` 1 byte | 2.38 ns | 2.31 ns | −3.3% |
| `SLEB128` 3 bytes | 7.11 ns | 4.65 ns | −34.6% |
| `SLEB128` 5 bytes | 9.95 ns | 5.51 ns | −44.7% |
| `SLEB128` 10 bytes | 16.21 ns | 6.74 ns | −58.4% |

All cases are allocation-free. For reference, `github.com/dennwc/varint`
reports 2.3-5.5 ns for 1-9 byte varints on its own hardware, and
`binary.AppendUvarint` (unsigned only) sits at ~3.0 ns here. The unrolled tail
brings `bt` into the same class while keeping the panic/rollback contract.

## 4.8 Pitfalls

- **Negative integers cost 10 bytes.** In two's complement, bit 63 is always
  set, so the encoder always emits the maximum length. If size matters more
  than compatibility, use zigzag (`binary.AppendVarint`).
- **Truncated varints are data errors, not panics you should recover from.**
  Validate the region first with `CanRead(10)` if the source is untrusted.
- **`SLEB128` is DWARF, not zigzag.** See section 4.2.
