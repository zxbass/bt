// Package bt provides fast helpers for reading and writing trusted binary
// data.
//
// # Reading
//
// Cursor reads scalars, strings and varints from a byte slice. Records,
// IndexedRecords and Chunks iterate fixed-size frames, and Sub carves out
// length-delimited records. Stream adapts an io.Reader for incremental
// parsing, and NewCursorFromReader buffers a whole reader.
//
// # Writing
//
// Writer appends the same encodings to a byte slice, and StreamWriter buffers
// them into an io.Writer with sticky errors. Reserve with Patch, or the
// LenU8/LenU16/LenU32 helpers, build length-prefixed sections.
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
// Writer panics when a value does not fit its encoding, for example:
//
//	bt: value 0x1000000 does not fit in 24 bits
//
// Callers are expected to validate sizes up front with CanRead/Ensure so that a
// panic signals a programming error rather than malformed input.
//
// If a read fails, the cursor offset is left unchanged, including ULEB128.
//
// Cursor aliases the underlying buffer: it does not copy. Mutating the buffer
// after NewCursor changes what the cursor reads, and slices returned by
// Bytes/Peek alias the buffer as well. The buffer must outlive the cursor.
// Writer.Bytes aliases the writer's buffer and is invalidated by the next
// write that grows it.
//
// No type is safe for concurrent use.
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
	panic(c.needMessage(n))
}

// needMessage formats the read-bounds panic, so callers whose fallthrough path
// ends in a panic can keep it as a terminating statement.
func (c *Cursor) needMessage(n int) string {
	if n < 0 {
		return fmt.Sprintf("bt: negative size %d", n)
	}
	return fmt.Sprintf("bt: need %d bytes at offset %d, have %d",
		n, c.off, len(c.b)-c.off)
}

// truncatedMessage formats the panic for a varint that continues past the end
// of the buffer. Unlike a plain read, the missing byte count is not known, so
// the message names the encoding instead of a size.
func (c *Cursor) truncatedMessage(kind string) string {
	return fmt.Sprintf("bt: truncated %s at offset %d", kind, c.off)
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

// Align advances the cursor to the next multiple of size and returns the number
// of skipped padding bytes. The offset is aligned relative to the start of the
// buffer.
func (c *Cursor) Align(size int) int {
	if size <= 0 {
		panic(fmt.Sprintf("bt: bad align size %d", size))
	}
	pad := (size - c.off%size) % size
	c.need(pad)
	c.off += pad
	return pad
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

	panic(c.truncatedMessage("ULEB128"))
}

// uleb128Rest decodes a three- to ten-byte varint whose first two bytes carry
// continuation bits. It moves the offset only on success.
func (c *Cursor) uleb128Rest() uint64 {
	b := c.b[c.off:]
	if len(b) < 3 {
		panic(c.truncatedMessage("ULEB128"))
	}
	v := uint64(b[0]&0x7F) | uint64(b[1]&0x7F)<<7
	if b[2] < 0x80 {
		c.off += 3
		return v | uint64(b[2])<<14
	}
	v |= uint64(b[2]&0x7F) << 14
	if len(b) < 4 {
		panic(c.truncatedMessage("ULEB128"))
	}
	if b[3] < 0x80 {
		c.off += 4
		return v | uint64(b[3])<<21
	}
	v |= uint64(b[3]&0x7F) << 21
	if len(b) < 5 {
		panic(c.truncatedMessage("ULEB128"))
	}
	if b[4] < 0x80 {
		c.off += 5
		return v | uint64(b[4])<<28
	}
	v |= uint64(b[4]&0x7F) << 28
	if len(b) < 6 {
		panic(c.truncatedMessage("ULEB128"))
	}
	if b[5] < 0x80 {
		c.off += 6
		return v | uint64(b[5])<<35
	}
	v |= uint64(b[5]&0x7F) << 35
	if len(b) < 7 {
		panic(c.truncatedMessage("ULEB128"))
	}
	if b[6] < 0x80 {
		c.off += 7
		return v | uint64(b[6])<<42
	}
	v |= uint64(b[6]&0x7F) << 42
	if len(b) < 8 {
		panic(c.truncatedMessage("ULEB128"))
	}
	if b[7] < 0x80 {
		c.off += 8
		return v | uint64(b[7])<<49
	}
	v |= uint64(b[7]&0x7F) << 49
	if len(b) < 9 {
		panic(c.truncatedMessage("ULEB128"))
	}
	if b[8] < 0x80 {
		c.off += 9
		return v | uint64(b[8])<<56
	}
	v |= uint64(b[8]&0x7F) << 56
	if len(b) < 10 {
		panic(c.truncatedMessage("ULEB128"))
	}
	if b[9] > 1 {
		panic("bt: ULEB128 overflow")
	}
	c.off += 10
	return v | uint64(b[9])<<63
}

// SLEB128 reads a signed LEB128 varint (DWARF-style, sign-extended). Like
// ULEB128, malformed input panics and leaves the cursor offset unchanged.
func (c *Cursor) SLEB128() int64 {
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
			b1 := c.b[c.off+1]
			if b1 < 0x80 {
				c.off += 2
				v := int64(b0&0x7F) | int64(b1&0x7F)<<7
				if b1&0x40 != 0 {
					v -= 1 << 14
				}
				return v
			}
			return c.sleb128Rest()
		}
	}

	panic(c.truncatedMessage("SLEB128"))
}

// sleb128Rest decodes a three- to ten-byte varint whose first two bytes carry
// continuation bits. It moves the offset only on success.
func (c *Cursor) sleb128Rest() int64 {
	b := c.b[c.off:]
	if len(b) < 3 {
		panic(c.truncatedMessage("SLEB128"))
	}
	v := int64(b[0]&0x7F) | int64(b[1]&0x7F)<<7
	if b[2] < 0x80 {
		if b[2]&0x40 != 0 {
			v -= 1 << 21
		}
		c.off += 3
		return v | int64(b[2])<<14
	}
	v |= int64(b[2]&0x7F) << 14
	if len(b) < 4 {
		panic(c.truncatedMessage("SLEB128"))
	}
	if b[3] < 0x80 {
		if b[3]&0x40 != 0 {
			v -= 1 << 28
		}
		c.off += 4
		return v | int64(b[3])<<21
	}
	v |= int64(b[3]&0x7F) << 21
	if len(b) < 5 {
		panic(c.truncatedMessage("SLEB128"))
	}
	if b[4] < 0x80 {
		if b[4]&0x40 != 0 {
			v -= 1 << 35
		}
		c.off += 5
		return v | int64(b[4])<<28
	}
	v |= int64(b[4]&0x7F) << 28
	if len(b) < 6 {
		panic(c.truncatedMessage("SLEB128"))
	}
	if b[5] < 0x80 {
		if b[5]&0x40 != 0 {
			v -= 1 << 42
		}
		c.off += 6
		return v | int64(b[5])<<35
	}
	v |= int64(b[5]&0x7F) << 35
	if len(b) < 7 {
		panic(c.truncatedMessage("SLEB128"))
	}
	if b[6] < 0x80 {
		if b[6]&0x40 != 0 {
			v -= 1 << 49
		}
		c.off += 7
		return v | int64(b[6])<<42
	}
	v |= int64(b[6]&0x7F) << 42
	if len(b) < 8 {
		panic(c.truncatedMessage("SLEB128"))
	}
	if b[7] < 0x80 {
		if b[7]&0x40 != 0 {
			v -= 1 << 56
		}
		c.off += 8
		return v | int64(b[7])<<49
	}
	v |= int64(b[7]&0x7F) << 49
	if len(b) < 9 {
		panic(c.truncatedMessage("SLEB128"))
	}
	if b[8] < 0x80 {
		if b[8]&0x40 != 0 {
			v |= -1 << 63
		}
		c.off += 9
		return v | int64(b[8])<<56
	}
	v |= int64(b[8]&0x7F) << 56
	if len(b) < 10 {
		panic(c.truncatedMessage("SLEB128"))
	}
	if b[9] != 0x00 && b[9] != 0x7F {
		panic("bt: SLEB128 overflow")
	}
	c.off += 10
	return v | int64(b[9]&0x7F)<<63
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
