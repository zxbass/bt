// Package bt provides fast helpers for reading trusted binary data.
//
// # Contract
//
// Reading past the end of the buffer panics. The panic value is a string:
//
//	bt: need 4 bytes at offset 2, have 1
//
// Negative sizes panic with:
//
//	bt: negative size -1
//
// Callers are expected to validate sizes up front with CanRead/Ensure so that a
// panic signals a programming error rather than malformed input.
//
// If a read fails, the cursor offset is left unchanged, including ULEB128.
//
// Cursor aliases the underlying buffer: it does not copy. Mutating the buffer
// after NewCursor changes what the cursor reads, and slices returned by
// Bytes/Peek alias the buffer as well. The buffer must outlive the cursor.
//
// Cursor is not safe for concurrent use.
package bt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"unsafe"
)

// ErrNoNul is returned by CStr when the buffer has no null terminator.
var ErrNoNul = errors.New("bt: null terminator not found")

// CStr returns the bytes up to the first null terminator. It returns ErrNoNul
// if the buffer contains no null byte.
func CStr(buf []byte) (string, error) {
	nulIdx := bytes.IndexByte(buf, 0)

	if nulIdx == -1 {
		return "", ErrNoNul
	}

	return string(buf[:nulIdx]), nil
}

// CStrOrRest is like CStr but returns the whole buffer when it contains no
// null terminator.
func CStrOrRest(buf []byte) string {
	nulIdx := bytes.IndexByte(buf, 0)

	if nulIdx == -1 {
		return string(buf)
	}

	return string(buf[:nulIdx])
}

// Cursor is a sequential reader over a byte slice.
type Cursor struct {
	b   []byte
	off int
}

// NewCursor returns a cursor positioned at the start of b.
func NewCursor(b []byte) *Cursor {
	return &Cursor{b: b, off: 0}
}

// uint conversion folds the negative-size check into the bounds check so that
// the hot path stays a single comparison and need is inlinable.
func (c *Cursor) need(n int) {
	if uint(n) > uint(len(c.b)-c.off) {
		c.needPanic(n)
	}
}

//go:noinline
func (c *Cursor) needPanic(n int) {
	if n < 0 {
		panic(fmt.Sprintf("bt: negative size %d", n))
	}
	panic(fmt.Sprintf("bt: need %d bytes at offset %d, have %d",
		n, c.off, len(c.b)-c.off))
}

// CanRead reports whether n bytes can be read from the current offset.
func (c *Cursor) CanRead(n int) bool {
	return n >= 0 && n <= len(c.b)-c.off
}

// Ensure panics unless n bytes can be read from the current offset.
func (c *Cursor) Ensure(n int) {
	c.need(n)
}

// BytesLeft returns the number of unread bytes.
func (c *Cursor) BytesLeft() int {
	return len(c.b) - c.off
}

// Pos returns the current offset.
func (c *Cursor) Pos() int {
	return c.off
}

// Skip advances the cursor by n bytes.
func (c *Cursor) Skip(n int) {
	c.need(n)
	c.off += n
}

// Bytes returns the next n bytes and advances the cursor. The returned slice
// aliases the cursor's buffer and is capped at n, so it cannot be resliced past
// the requested window.
func (c *Cursor) Bytes(n int) []byte {
	c.need(n)
	raw := c.b[c.off : c.off+n : c.off+n]
	c.off += n
	return raw
}

// Peek returns the next n bytes without advancing the cursor. The returned
// slice aliases the cursor's buffer and is capped at n.
func (c *Cursor) Peek(n int) []byte {
	c.need(n)
	return c.b[c.off : c.off+n : c.off+n]
}

// Sub returns an independent cursor over the next n bytes and advances the
// parent cursor by n. The sub-cursor aliases the same buffer; use it to parse
// a length-delimited record without letting its reads affect the parent.
func (c *Cursor) Sub(n int) *Cursor {
	c.need(n)
	sub := NewCursor(c.b[c.off : c.off+n : c.off+n])
	c.off += n
	return sub
}

// U8 reads an unsigned 8-bit value.
func (c *Cursor) U8() byte {
	c.need(1)
	v := c.b[c.off]
	c.off += 1
	return v
}

// U16 reads an unsigned 16-bit value in the given byte order.
func (c *Cursor) U16(byteOrder binary.ByteOrder) uint16 {
	c.need(2)
	v := byteOrder.Uint16(c.b[c.off:])
	c.off += 2
	return v
}

// U16LE reads a little-endian unsigned 16-bit value.
//
// The LE/BE wrappers duplicate the generic bodies on purpose: hard-coding the
// concrete byte order lets the compiler inline the whole read into the caller
// instead of paying for an interface dispatch (see BenchmarkU16LE vs
// BenchmarkU16Order).
func (c *Cursor) U16LE() uint16 {
	c.need(2)
	v := binary.LittleEndian.Uint16(c.b[c.off:])
	c.off += 2
	return v
}

// U16BE reads a big-endian unsigned 16-bit value.
func (c *Cursor) U16BE() uint16 {
	c.need(2)
	v := binary.BigEndian.Uint16(c.b[c.off:])
	c.off += 2
	return v
}

// U32 reads an unsigned 32-bit value in the given byte order.
func (c *Cursor) U32(byteOrder binary.ByteOrder) uint32 {
	c.need(4)
	v := byteOrder.Uint32(c.b[c.off:])
	c.off += 4
	return v
}

// U32LE reads a little-endian unsigned 32-bit value.
func (c *Cursor) U32LE() uint32 {
	c.need(4)
	v := binary.LittleEndian.Uint32(c.b[c.off:])
	c.off += 4
	return v
}

// U32BE reads a big-endian unsigned 32-bit value.
func (c *Cursor) U32BE() uint32 {
	c.need(4)
	v := binary.BigEndian.Uint32(c.b[c.off:])
	c.off += 4
	return v
}

// U64 reads an unsigned 64-bit value in the given byte order.
func (c *Cursor) U64(byteOrder binary.ByteOrder) uint64 {
	c.need(8)
	v := byteOrder.Uint64(c.b[c.off:])
	c.off += 8
	return v
}

// U64LE reads a little-endian unsigned 64-bit value.
func (c *Cursor) U64LE() uint64 {
	c.need(8)
	v := binary.LittleEndian.Uint64(c.b[c.off:])
	c.off += 8
	return v
}

// U64BE reads a big-endian unsigned 64-bit value.
func (c *Cursor) U64BE() uint64 {
	c.need(8)
	v := binary.BigEndian.Uint64(c.b[c.off:])
	c.off += 8
	return v
}

// I8 reads a signed 8-bit value.
func (c *Cursor) I8() int8 {
	return int8(c.U8())
}

// I16 reads a signed 16-bit value in the given byte order.
func (c *Cursor) I16(byteOrder binary.ByteOrder) int16 {
	return int16(c.U16(byteOrder))
}

// I16LE reads a little-endian signed 16-bit value.
func (c *Cursor) I16LE() int16 { return int16(c.U16LE()) }

// I16BE reads a big-endian signed 16-bit value.
func (c *Cursor) I16BE() int16 { return int16(c.U16BE()) }

// I32 reads a signed 32-bit value in the given byte order.
func (c *Cursor) I32(byteOrder binary.ByteOrder) int32 {
	return int32(c.U32(byteOrder))
}

// I32LE reads a little-endian signed 32-bit value.
func (c *Cursor) I32LE() int32 { return int32(c.U32LE()) }

// I32BE reads a big-endian signed 32-bit value.
func (c *Cursor) I32BE() int32 { return int32(c.U32BE()) }

// I64 reads a signed 64-bit value in the given byte order.
func (c *Cursor) I64(byteOrder binary.ByteOrder) int64 {
	return int64(c.U64(byteOrder))
}

// I64LE reads a little-endian signed 64-bit value.
func (c *Cursor) I64LE() int64 { return int64(c.U64LE()) }

// I64BE reads a big-endian signed 64-bit value.
func (c *Cursor) I64BE() int64 { return int64(c.U64BE()) }

// U24LE reads a little-endian unsigned 24-bit value.
func (c *Cursor) U24LE() uint32 {
	c.need(3)
	if c.off+4 <= len(c.b) {
		v := binary.LittleEndian.Uint32(c.b[c.off:]) & 0xFFFFFF
		c.off += 3
		return v
	}
	b := c.b[c.off:]
	v := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
	c.off += 3
	return v
}

// U24BE reads a big-endian unsigned 24-bit value.
func (c *Cursor) U24BE() uint32 {
	c.need(3)
	if c.off+4 <= len(c.b) {
		v := binary.BigEndian.Uint32(c.b[c.off:]) >> 8
		c.off += 3
		return v
	}
	b := c.b[c.off:]
	v := uint32(b[2]) | uint32(b[1])<<8 | uint32(b[0])<<16
	c.off += 3
	return v
}

func signExtend24(v uint32) int32 {
	if v&0x800000 != 0 {
		return int32(v | 0xFF000000)
	}
	return int32(v)
}

// I24LE reads a little-endian signed 24-bit value.
func (c *Cursor) I24LE() int32 { return signExtend24(c.U24LE()) }

// I24BE reads a big-endian signed 24-bit value.
func (c *Cursor) I24BE() int32 { return signExtend24(c.U24BE()) }

// F32 reads an IEEE 754 32-bit float in the given byte order.
func (c *Cursor) F32(byteOrder binary.ByteOrder) float32 {
	c.need(4)
	v := math.Float32frombits(byteOrder.Uint32(c.b[c.off:]))
	c.off += 4
	return v
}

// F32LE reads a little-endian IEEE 754 32-bit float.
func (c *Cursor) F32LE() float32 {
	c.need(4)
	v := math.Float32frombits(binary.LittleEndian.Uint32(c.b[c.off:]))
	c.off += 4
	return v
}

// F32BE reads a big-endian IEEE 754 32-bit float.
func (c *Cursor) F32BE() float32 {
	c.need(4)
	v := math.Float32frombits(binary.BigEndian.Uint32(c.b[c.off:]))
	c.off += 4
	return v
}

// F64 reads an IEEE 754 64-bit float in the given byte order.
func (c *Cursor) F64(byteOrder binary.ByteOrder) float64 {
	c.need(8)
	v := math.Float64frombits(byteOrder.Uint64(c.b[c.off:]))
	c.off += 8
	return v
}

// F64LE reads a little-endian IEEE 754 64-bit float.
func (c *Cursor) F64LE() float64 {
	c.need(8)
	v := math.Float64frombits(binary.LittleEndian.Uint64(c.b[c.off:]))
	c.off += 8
	return v
}

// F64BE reads a big-endian IEEE 754 64-bit float.
func (c *Cursor) F64BE() float64 {
	c.need(8)
	v := math.Float64frombits(binary.BigEndian.Uint64(c.b[c.off:]))
	c.off += 8
	return v
}

// ULEB128 reads an unsigned LEB128 varint. Malformed input (truncated or
// overflowing uint64) panics and leaves the cursor offset unchanged.
func (c *Cursor) ULEB128() (result uint64) {
	start := c.off

	if c.off < len(c.b) {
		b0 := c.b[c.off]
		if b0 < 0x80 {
			c.off++
			return uint64(b0)
		}
		if c.off+1 < len(c.b) {
			if b1 := c.b[c.off+1]; b1 < 0x80 {
				c.off += 2
				return uint64(b0&0x7F) | uint64(b1)<<7
			}
		}
	}

	for shift := uint(0); ; shift += 7 {
		if c.off == len(c.b) {
			c.off = start
			c.needPanic(1)
		}

		b := c.b[c.off]
		c.off++

		if shift == 63 && b > 1 {
			c.off = start
			panic("bt: ULEB128 overflow")
		}
		result |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return result
		}
	}
}

// SLEB128 reads a signed LEB128 varint (DWARF-style, sign-extended). Like
// ULEB128, malformed input panics and leaves the cursor offset unchanged.
func (c *Cursor) SLEB128() int64 {
	start := c.off

	if c.off < len(c.b) {
		b0 := c.b[c.off]
		if b0 < 0x80 {
			c.off++
			v := int64(b0 & 0x3F)
			if b0&0x40 != 0 {
				v -= 0x40
			}
			return v
		}
		if c.off+1 < len(c.b) {
			if b1 := c.b[c.off+1]; b1 < 0x80 {
				c.off += 2
				v := int64(b0&0x7F) | int64(b1&0x7F)<<7
				if b1&0x40 != 0 {
					v -= 1 << 14
				}
				return v
			}
		}
	}

	var result int64

	for i := 0; ; i++ {
		if c.off == len(c.b) {
			c.off = start
			c.needPanic(1)
		}

		b := c.b[c.off]
		c.off++

		if i == 9 && b != 0x00 && b != 0x7F {
			c.off = start
			panic("bt: SLEB128 overflow")
		}

		result |= int64(b&0x7F) << (7 * i)

		if b&0x80 == 0 {
			if shift := uint(7 * (i + 1)); shift < 64 && b&0x40 != 0 {
				result |= -1 << shift
			}
			return result
		}
	}
}

// StrOrRest reads exactly sz bytes and returns the bytes up to the first null
// terminator. If the window has no null byte, the whole window is returned.
func (c *Cursor) StrOrRest(sz int) string {
	c.need(sz)
	v := CStrOrRest(c.b[c.off : c.off+sz])
	c.off += sz
	return v
}

// RawStr reads exactly sz bytes and returns them as a string, copying the data.
// Null bytes are kept.
func (c *Cursor) RawStr(sz int) string {
	c.need(sz)
	v := string(c.b[c.off : c.off+sz])
	c.off += sz
	return v
}

// StrUnsafe is like RawStr but does not copy: the returned string aliases the
// cursor's buffer and is only valid while that buffer is alive and unmodified.
func (c *Cursor) StrUnsafe(sz int) string {
	c.need(sz)
	if sz == 0 {
		return ""
	}
	v := unsafe.String(unsafe.SliceData(c.b[c.off:c.off+sz]), sz)
	c.off += sz
	return v
}
