package bt

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
)

// Writer appends encoded values to a byte slice. It is the write-side
// counterpart of Cursor: writes do not return errors, and invalid input
// panics.
//
// Writer grows its buffer by appending, so it allocates only when a write
// outgrows the current capacity. Call Grow up front to keep hot paths
// allocation-free.
//
// Bytes aliases the writer's buffer and is invalidated by the next write that
// grows it. Writer is not safe for concurrent use.
type Writer struct {
	b []byte
}

// NewWriter returns an empty writer.
func NewWriter() *Writer {
	return &Writer{}
}

// Len returns the number of buffered bytes.
func (w *Writer) Len() int {
	return len(w.b)
}

// Bytes returns the buffered bytes. The slice aliases the writer's buffer and
// is invalidated by the next write that grows it.
func (w *Writer) Bytes() []byte {
	return w.b
}

// Reset drops all buffered bytes, keeping the allocated capacity.
func (w *Writer) Reset() {
	w.b = w.b[:0]
}

// Grow ensures room for at least n more bytes so that subsequent writes do not
// reallocate.
func (w *Writer) Grow(n int) {
	if n < 0 {
		panic(fmt.Sprintf("bt: negative size %d", n))
	}
	if n <= cap(w.b)-len(w.b) {
		return
	}
	buf := make([]byte, len(w.b), len(w.b)+n)
	copy(buf, w.b)
	w.b = buf
}

// Truncate drops all bytes beyond the first n, keeping the capacity.
func (w *Writer) Truncate(n int) {
	if n < 0 {
		panic(fmt.Sprintf("bt: negative size %d", n))
	}
	if n > len(w.b) {
		panic(fmt.Sprintf("bt: truncate size %d out of range (len %d)", n, len(w.b)))
	}
	w.b = w.b[:n]
}

// Write implements io.Writer by appending p. It never fails.
func (w *Writer) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

// WriteByte implements io.ByteWriter by appending b. It never fails.
func (w *Writer) WriteByte(b byte) error {
	w.b = append(w.b, b)
	return nil
}

// WriteString implements io.StringWriter by appending s. It never fails.
func (w *Writer) WriteString(s string) (int, error) {
	w.b = append(w.b, s...)
	return len(s), nil
}

// WriteTo implements io.WriterTo: it writes the buffered bytes to dst and
// drains them, keeping the capacity, mirroring bytes.Buffer.
func (w *Writer) WriteTo(dst io.Writer) (int64, error) {
	total := len(w.b)
	if total == 0 {
		return 0, nil
	}

	n, err := dst.Write(w.b)
	if n < 0 || n > total {
		panic("bt: invalid Write count")
	}
	if n > 0 {
		w.b = w.b[:copy(w.b, w.b[n:])]
	}
	if err != nil {
		return int64(n), err
	}
	if n != total {
		return int64(n), io.ErrShortWrite
	}
	return int64(n), nil
}

// U8 appends an unsigned 8-bit value.
func (w *Writer) U8(v byte) {
	w.b = append(w.b, v)
}

// U16 appends an unsigned 16-bit value in the given byte order.
func (w *Writer) U16(order binary.ByteOrder, v uint16) {
	var tmp [2]byte
	order.PutUint16(tmp[:], v)
	w.b = append(w.b, tmp[:]...)
}

// U16LE appends a little-endian unsigned 16-bit value.
func (w *Writer) U16LE(v uint16) {
	w.b = binary.LittleEndian.AppendUint16(w.b, v)
}

// U16BE appends a big-endian unsigned 16-bit value.
func (w *Writer) U16BE(v uint16) {
	w.b = binary.BigEndian.AppendUint16(w.b, v)
}

// U32 appends an unsigned 32-bit value in the given byte order.
func (w *Writer) U32(order binary.ByteOrder, v uint32) {
	var tmp [4]byte
	order.PutUint32(tmp[:], v)
	w.b = append(w.b, tmp[:]...)
}

// U32LE appends a little-endian unsigned 32-bit value.
func (w *Writer) U32LE(v uint32) {
	w.b = binary.LittleEndian.AppendUint32(w.b, v)
}

// U32BE appends a big-endian unsigned 32-bit value.
func (w *Writer) U32BE(v uint32) {
	w.b = binary.BigEndian.AppendUint32(w.b, v)
}

// U64 appends an unsigned 64-bit value in the given byte order.
func (w *Writer) U64(order binary.ByteOrder, v uint64) {
	var tmp [8]byte
	order.PutUint64(tmp[:], v)
	w.b = append(w.b, tmp[:]...)
}

// U64LE appends a little-endian unsigned 64-bit value.
func (w *Writer) U64LE(v uint64) {
	w.b = binary.LittleEndian.AppendUint64(w.b, v)
}

// U64BE appends a big-endian unsigned 64-bit value.
func (w *Writer) U64BE(v uint64) {
	w.b = binary.BigEndian.AppendUint64(w.b, v)
}

// I8 appends a signed 8-bit value.
func (w *Writer) I8(v int8) {
	w.b = append(w.b, byte(v))
}

// I16 appends a signed 16-bit value in the given byte order.
func (w *Writer) I16(order binary.ByteOrder, v int16) {
	w.U16(order, uint16(v))
}

// I16LE appends a little-endian signed 16-bit value.
func (w *Writer) I16LE(v int16) {
	w.U16LE(uint16(v))
}

// I16BE appends a big-endian signed 16-bit value.
func (w *Writer) I16BE(v int16) {
	w.U16BE(uint16(v))
}

// I32 appends a signed 32-bit value in the given byte order.
func (w *Writer) I32(order binary.ByteOrder, v int32) {
	w.U32(order, uint32(v))
}

// I32LE appends a little-endian signed 32-bit value.
func (w *Writer) I32LE(v int32) {
	w.U32LE(uint32(v))
}

// I32BE appends a big-endian signed 32-bit value.
func (w *Writer) I32BE(v int32) {
	w.U32BE(uint32(v))
}

// I64 appends a signed 64-bit value in the given byte order.
func (w *Writer) I64(order binary.ByteOrder, v int64) {
	w.U64(order, uint64(v))
}

// I64LE appends a little-endian signed 64-bit value.
func (w *Writer) I64LE(v int64) {
	w.U64LE(uint64(v))
}

// I64BE appends a big-endian signed 64-bit value.
func (w *Writer) I64BE(v int64) {
	w.U64BE(uint64(v))
}

// U24LE appends a little-endian unsigned 24-bit value. It panics if v does not
// fit in 24 bits.
func (w *Writer) U24LE(v uint32) {
	if v > 0xFFFFFF {
		panic(fmt.Sprintf("bt: value %#x does not fit in 24 bits", v))
	}
	w.b = append(w.b, byte(v), byte(v>>8), byte(v>>16))
}

// U24BE appends a big-endian unsigned 24-bit value. It panics if v does not
// fit in 24 bits.
func (w *Writer) U24BE(v uint32) {
	if v > 0xFFFFFF {
		panic(fmt.Sprintf("bt: value %#x does not fit in 24 bits", v))
	}
	w.b = append(w.b, byte(v>>16), byte(v>>8), byte(v))
}

// I24LE appends a little-endian signed 24-bit value. It panics if v does not
// fit in 24 bits.
func (w *Writer) I24LE(v int32) {
	if v < -0x800000 || v > 0x7FFFFF {
		panic(fmt.Sprintf("bt: value %d does not fit in 24 bits", v))
	}
	w.U24LE(uint32(v) & 0xFFFFFF)
}

// I24BE appends a big-endian signed 24-bit value. It panics if v does not fit
// in 24 bits.
func (w *Writer) I24BE(v int32) {
	if v < -0x800000 || v > 0x7FFFFF {
		panic(fmt.Sprintf("bt: value %d does not fit in 24 bits", v))
	}
	w.U24BE(uint32(v) & 0xFFFFFF)
}

// F32 appends an IEEE 754 32-bit float in the given byte order.
func (w *Writer) F32(order binary.ByteOrder, v float32) {
	w.U32(order, math.Float32bits(v))
}

// F32LE appends a little-endian IEEE 754 32-bit float.
func (w *Writer) F32LE(v float32) {
	w.U32LE(math.Float32bits(v))
}

// F32BE appends a big-endian IEEE 754 32-bit float.
func (w *Writer) F32BE(v float32) {
	w.U32BE(math.Float32bits(v))
}

// F64 appends an IEEE 754 64-bit float in the given byte order.
func (w *Writer) F64(order binary.ByteOrder, v float64) {
	w.U64(order, math.Float64bits(v))
}

// F64LE appends a little-endian IEEE 754 64-bit float.
func (w *Writer) F64LE(v float64) {
	w.U64LE(math.Float64bits(v))
}

// F64BE appends a big-endian IEEE 754 64-bit float.
func (w *Writer) F64BE(v float64) {
	w.U64BE(math.Float64bits(v))
}

// ULEB128 appends an unsigned LEB128 varint.
func (w *Writer) ULEB128(v uint64) {
	for v >= 0x80 {
		w.b = append(w.b, byte(v)|0x80)
		v >>= 7
	}
	w.b = append(w.b, byte(v))
}

// SLEB128 appends a signed LEB128 varint.
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

// RawStr appends s unchanged. Null bytes are kept.
func (w *Writer) RawStr(s string) {
	w.b = append(w.b, s...)
}

// CStr appends s followed by a null terminator. It panics if s contains a null
// byte, because the terminator would be lost on a round trip.
func (w *Writer) CStr(s string) {
	if strings.IndexByte(s, 0) >= 0 {
		panic("bt: CStr contains NUL byte")
	}
	w.b = append(w.b, s...)
	w.b = append(w.b, 0)
}

// zeroPad supplies zero bytes without allocating on every Reserve.
var zeroPad [256]byte

// Reserve appends n zero bytes and returns the offset of the first one, for
// later use with the Patch methods.
func (w *Writer) Reserve(n int) int {
	if n < 0 {
		panic(fmt.Sprintf("bt: negative size %d", n))
	}
	pos := len(w.b)
	w.appendZeros(n)
	return pos
}

// Align appends zero bytes until the length is a multiple of size and returns
// the number of appended bytes. The length is aligned relative to the start of
// the buffer.
func (w *Writer) Align(size int) int {
	if size <= 0 {
		panic(fmt.Sprintf("bt: bad align size %d", size))
	}
	pad := (size - len(w.b)%size) % size
	w.appendZeros(pad)
	return pad
}

func (w *Writer) appendZeros(n int) {
	for n > 0 {
		chunk := n
		if chunk > len(zeroPad) {
			chunk = len(zeroPad)
		}
		w.b = append(w.b, zeroPad[:chunk]...)
		n -= chunk
	}
}

func (w *Writer) patchCheck(pos, n int) {
	if pos < 0 || pos > len(w.b)-n {
		panic(fmt.Sprintf("bt: patch at offset %d out of range (len %d)", pos, len(w.b)))
	}
}

// PatchU8 overwrites the byte at pos.
func (w *Writer) PatchU8(pos int, v byte) {
	w.patchCheck(pos, 1)
	w.b[pos] = v
}

// PatchU16LE overwrites a little-endian unsigned 16-bit value at pos.
func (w *Writer) PatchU16LE(pos int, v uint16) {
	w.patchCheck(pos, 2)
	binary.LittleEndian.PutUint16(w.b[pos:], v)
}

// PatchU16BE overwrites a big-endian unsigned 16-bit value at pos.
func (w *Writer) PatchU16BE(pos int, v uint16) {
	w.patchCheck(pos, 2)
	binary.BigEndian.PutUint16(w.b[pos:], v)
}

// PatchU32LE overwrites a little-endian unsigned 32-bit value at pos.
func (w *Writer) PatchU32LE(pos int, v uint32) {
	w.patchCheck(pos, 4)
	binary.LittleEndian.PutUint32(w.b[pos:], v)
}

// PatchU32BE overwrites a big-endian unsigned 32-bit value at pos.
func (w *Writer) PatchU32BE(pos int, v uint32) {
	w.patchCheck(pos, 4)
	binary.BigEndian.PutUint32(w.b[pos:], v)
}

// PatchU64LE overwrites a little-endian unsigned 64-bit value at pos.
func (w *Writer) PatchU64LE(pos int, v uint64) {
	w.patchCheck(pos, 8)
	binary.LittleEndian.PutUint64(w.b[pos:], v)
}

// PatchU64BE overwrites a big-endian unsigned 64-bit value at pos.
func (w *Writer) PatchU64BE(pos int, v uint64) {
	w.patchCheck(pos, 8)
	binary.BigEndian.PutUint64(w.b[pos:], v)
}

// sectionLen panics unless n fits a prefix of the given bit width.
func sectionLen(n uint64, bits int) uint64 {
	if n > (uint64(1)<<uint(bits))-1 {
		panic(fmt.Sprintf("bt: section length %d does not fit in %d bits", n, bits))
	}
	return n
}

// LenU8 runs f and prefixes the bytes it writes with their length as u8.
func (w *Writer) LenU8(f func(*Writer)) {
	off := w.Reserve(1)
	start := len(w.b)
	f(w)
	w.PatchU8(off, byte(sectionLen(uint64(len(w.b)-start), 8)))
}

// LenU16LE runs f and prefixes the bytes it writes with their length as
// little-endian u16.
func (w *Writer) LenU16LE(f func(*Writer)) {
	off := w.Reserve(2)
	start := len(w.b)
	f(w)
	w.PatchU16LE(off, uint16(sectionLen(uint64(len(w.b)-start), 16)))
}

// LenU16BE runs f and prefixes the bytes it writes with their length as
// big-endian u16.
func (w *Writer) LenU16BE(f func(*Writer)) {
	off := w.Reserve(2)
	start := len(w.b)
	f(w)
	w.PatchU16BE(off, uint16(sectionLen(uint64(len(w.b)-start), 16)))
}

// LenU32LE runs f and prefixes the bytes it writes with their length as
// little-endian u32.
func (w *Writer) LenU32LE(f func(*Writer)) {
	off := w.Reserve(4)
	start := len(w.b)
	f(w)
	w.PatchU32LE(off, uint32(sectionLen(uint64(len(w.b)-start), 32)))
}

// LenU32BE runs f and prefixes the bytes it writes with their length as
// big-endian u32.
func (w *Writer) LenU32BE(f func(*Writer)) {
	off := w.Reserve(4)
	start := len(w.b)
	f(w)
	w.PatchU32BE(off, uint32(sectionLen(uint64(len(w.b)-start), 32)))
}
