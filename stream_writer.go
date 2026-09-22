package bt

import (
	"encoding/binary"
	"io"
)

const streamWriterFlushThreshold = 64 << 10

// StreamWriterOption configures a StreamWriter.
type StreamWriterOption func(*StreamWriter)

// WithFlushThreshold flushes automatically once the buffered bytes reach n.
// Values <= 0 disable automatic flushing; call Flush explicitly.
func WithFlushThreshold(n int) StreamWriterOption {
	return func(sw *StreamWriter) { sw.threshold = n }
}

// StreamWriter buffers writes in memory and forwards them to an io.Writer.
// It is the write-side counterpart of Stream: the typed write methods do not
// return errors, failures are sticky and surface through Err and Flush, and
// invalid values panic like Writer. Write, WriteByte and WriteString report
// the sticky error, including a flush failure caused by the call itself.
//
// After the first write error every write method becomes a no-op and Flush
// returns that error until Reset is called. The underlying writer is never
// closed. StreamWriter is not safe for concurrent use.
type StreamWriter struct {
	dst       io.Writer
	buf       Writer
	threshold int
	reserved  []reservation
	err       error
}

type reservation struct {
	start int
	end   int
}

// NewStreamWriter returns a writer that buffers into dst. Once the buffered
// bytes reach the flush threshold (64 KiB by default) they are written to dst
// automatically; Flush writes whatever is left.
func NewStreamWriter(dst io.Writer, opts ...StreamWriterOption) *StreamWriter {
	sw := &StreamWriter{dst: dst, threshold: streamWriterFlushThreshold}
	for _, opt := range opts {
		opt(sw)
	}
	return sw
}

// Buffered returns the number of bytes waiting to be flushed.
func (sw *StreamWriter) Buffered() int {
	return sw.buf.Len()
}

// Err returns the sticky write error, or nil.
func (sw *StreamWriter) Err() error {
	return sw.err
}

// Reset drops buffered bytes and clears the sticky error.
func (sw *StreamWriter) Reset() {
	sw.err = nil
	sw.reserved = sw.reserved[:0]
	sw.buf.Reset()
}

// Flush writes buffered bytes to the underlying writer. Bytes that were already
// handed to a failing writer are lost; the error is sticky. Flushing with an
// open Reserve panics, because the reserved positions would be lost.
func (sw *StreamWriter) Flush() error {
	if sw.err != nil {
		return sw.err
	}
	if len(sw.reserved) > 0 {
		panic("bt: Flush with open Reserve")
	}
	return sw.flush()
}

func (sw *StreamWriter) flush() error {
	if sw.buf.Len() == 0 {
		return nil
	}
	if _, err := sw.buf.WriteTo(sw.dst); err != nil {
		sw.err = err
		sw.buf.Reset()
		return err
	}
	return nil
}

func (sw *StreamWriter) afterWrite() error {
	if len(sw.reserved) == 0 && sw.threshold > 0 && sw.buf.Len() >= sw.threshold {
		return sw.flush()
	}
	return nil
}

// release ends the reserve that contains pos, if any.
func (sw *StreamWriter) release(pos int) {
	for i, r := range sw.reserved {
		if pos >= r.start && pos < r.end {
			sw.reserved = append(sw.reserved[:i], sw.reserved[i+1:]...)
			return
		}
	}
}

// Write implements io.Writer: it buffers p and reports the sticky error,
// including a flush failure triggered by this call.
func (sw *StreamWriter) Write(p []byte) (int, error) {
	if sw.err != nil {
		return 0, sw.err
	}
	sw.buf.Write(p)
	return len(p), sw.afterWrite()
}

// WriteByte implements io.ByteWriter: it buffers b and reports the sticky
// error, including a flush failure triggered by this call.
func (sw *StreamWriter) WriteByte(b byte) error {
	if sw.err != nil {
		return sw.err
	}
	sw.buf.WriteByte(b)
	return sw.afterWrite()
}

// WriteString implements io.StringWriter: it buffers s and reports the sticky
// error, including a flush failure triggered by this call.
func (sw *StreamWriter) WriteString(s string) (int, error) {
	if sw.err != nil {
		return 0, sw.err
	}
	sw.buf.WriteString(s)
	return len(s), sw.afterWrite()
}

// U8 buffers an unsigned 8-bit value.
func (sw *StreamWriter) U8(v byte) {
	if sw.err != nil {
		return
	}
	sw.buf.U8(v)
	sw.afterWrite()
}

// U16 buffers an unsigned 16-bit value in the given byte order.
func (sw *StreamWriter) U16(order binary.ByteOrder, v uint16) {
	if sw.err != nil {
		return
	}
	sw.buf.U16(order, v)
	sw.afterWrite()
}

// U16LE buffers a little-endian unsigned 16-bit value.
func (sw *StreamWriter) U16LE(v uint16) {
	if sw.err != nil {
		return
	}
	sw.buf.U16LE(v)
	sw.afterWrite()
}

// U16BE buffers a big-endian unsigned 16-bit value.
func (sw *StreamWriter) U16BE(v uint16) {
	if sw.err != nil {
		return
	}
	sw.buf.U16BE(v)
	sw.afterWrite()
}

// U32 buffers an unsigned 32-bit value in the given byte order.
func (sw *StreamWriter) U32(order binary.ByteOrder, v uint32) {
	if sw.err != nil {
		return
	}
	sw.buf.U32(order, v)
	sw.afterWrite()
}

// U32LE buffers a little-endian unsigned 32-bit value.
func (sw *StreamWriter) U32LE(v uint32) {
	if sw.err != nil {
		return
	}
	sw.buf.U32LE(v)
	sw.afterWrite()
}

// U32BE buffers a big-endian unsigned 32-bit value.
func (sw *StreamWriter) U32BE(v uint32) {
	if sw.err != nil {
		return
	}
	sw.buf.U32BE(v)
	sw.afterWrite()
}

// U64 buffers an unsigned 64-bit value in the given byte order.
func (sw *StreamWriter) U64(order binary.ByteOrder, v uint64) {
	if sw.err != nil {
		return
	}
	sw.buf.U64(order, v)
	sw.afterWrite()
}

// U64LE buffers a little-endian unsigned 64-bit value.
func (sw *StreamWriter) U64LE(v uint64) {
	if sw.err != nil {
		return
	}
	sw.buf.U64LE(v)
	sw.afterWrite()
}

// U64BE buffers a big-endian unsigned 64-bit value.
func (sw *StreamWriter) U64BE(v uint64) {
	if sw.err != nil {
		return
	}
	sw.buf.U64BE(v)
	sw.afterWrite()
}

// I8 buffers a signed 8-bit value.
func (sw *StreamWriter) I8(v int8) {
	if sw.err != nil {
		return
	}
	sw.buf.I8(v)
	sw.afterWrite()
}

// I16 buffers a signed 16-bit value in the given byte order.
func (sw *StreamWriter) I16(order binary.ByteOrder, v int16) {
	if sw.err != nil {
		return
	}
	sw.buf.I16(order, v)
	sw.afterWrite()
}

// I16LE buffers a little-endian signed 16-bit value.
func (sw *StreamWriter) I16LE(v int16) {
	if sw.err != nil {
		return
	}
	sw.buf.I16LE(v)
	sw.afterWrite()
}

// I16BE buffers a big-endian signed 16-bit value.
func (sw *StreamWriter) I16BE(v int16) {
	if sw.err != nil {
		return
	}
	sw.buf.I16BE(v)
	sw.afterWrite()
}

// I32 buffers a signed 32-bit value in the given byte order.
func (sw *StreamWriter) I32(order binary.ByteOrder, v int32) {
	if sw.err != nil {
		return
	}
	sw.buf.I32(order, v)
	sw.afterWrite()
}

// I32LE buffers a little-endian signed 32-bit value.
func (sw *StreamWriter) I32LE(v int32) {
	if sw.err != nil {
		return
	}
	sw.buf.I32LE(v)
	sw.afterWrite()
}

// I32BE buffers a big-endian signed 32-bit value.
func (sw *StreamWriter) I32BE(v int32) {
	if sw.err != nil {
		return
	}
	sw.buf.I32BE(v)
	sw.afterWrite()
}

// I64 buffers a signed 64-bit value in the given byte order.
func (sw *StreamWriter) I64(order binary.ByteOrder, v int64) {
	if sw.err != nil {
		return
	}
	sw.buf.I64(order, v)
	sw.afterWrite()
}

// I64LE buffers a little-endian signed 64-bit value.
func (sw *StreamWriter) I64LE(v int64) {
	if sw.err != nil {
		return
	}
	sw.buf.I64LE(v)
	sw.afterWrite()
}

// I64BE buffers a big-endian signed 64-bit value.
func (sw *StreamWriter) I64BE(v int64) {
	if sw.err != nil {
		return
	}
	sw.buf.I64BE(v)
	sw.afterWrite()
}

// U24LE buffers a little-endian unsigned 24-bit value.
func (sw *StreamWriter) U24LE(v uint32) {
	if sw.err != nil {
		return
	}
	sw.buf.U24LE(v)
	sw.afterWrite()
}

// U24BE buffers a big-endian unsigned 24-bit value.
func (sw *StreamWriter) U24BE(v uint32) {
	if sw.err != nil {
		return
	}
	sw.buf.U24BE(v)
	sw.afterWrite()
}

// I24LE buffers a little-endian signed 24-bit value.
func (sw *StreamWriter) I24LE(v int32) {
	if sw.err != nil {
		return
	}
	sw.buf.I24LE(v)
	sw.afterWrite()
}

// I24BE buffers a big-endian signed 24-bit value.
func (sw *StreamWriter) I24BE(v int32) {
	if sw.err != nil {
		return
	}
	sw.buf.I24BE(v)
	sw.afterWrite()
}

// F32 buffers an IEEE 754 32-bit float in the given byte order.
func (sw *StreamWriter) F32(order binary.ByteOrder, v float32) {
	if sw.err != nil {
		return
	}
	sw.buf.F32(order, v)
	sw.afterWrite()
}

// F32LE buffers a little-endian IEEE 754 32-bit float.
func (sw *StreamWriter) F32LE(v float32) {
	if sw.err != nil {
		return
	}
	sw.buf.F32LE(v)
	sw.afterWrite()
}

// F32BE buffers a big-endian IEEE 754 32-bit float.
func (sw *StreamWriter) F32BE(v float32) {
	if sw.err != nil {
		return
	}
	sw.buf.F32BE(v)
	sw.afterWrite()
}

// F64 buffers an IEEE 754 64-bit float in the given byte order.
func (sw *StreamWriter) F64(order binary.ByteOrder, v float64) {
	if sw.err != nil {
		return
	}
	sw.buf.F64(order, v)
	sw.afterWrite()
}

// F64LE buffers a little-endian IEEE 754 64-bit float.
func (sw *StreamWriter) F64LE(v float64) {
	if sw.err != nil {
		return
	}
	sw.buf.F64LE(v)
	sw.afterWrite()
}

// F64BE buffers a big-endian IEEE 754 64-bit float.
func (sw *StreamWriter) F64BE(v float64) {
	if sw.err != nil {
		return
	}
	sw.buf.F64BE(v)
	sw.afterWrite()
}

// ULEB128 buffers an unsigned LEB128 varint.
func (sw *StreamWriter) ULEB128(v uint64) {
	if sw.err != nil {
		return
	}
	sw.buf.ULEB128(v)
	sw.afterWrite()
}

// SLEB128 buffers a signed LEB128 varint.
func (sw *StreamWriter) SLEB128(v int64) {
	if sw.err != nil {
		return
	}
	sw.buf.SLEB128(v)
	sw.afterWrite()
}

// RawStr buffers s unchanged.
func (sw *StreamWriter) RawStr(s string) {
	if sw.err != nil {
		return
	}
	sw.buf.RawStr(s)
	sw.afterWrite()
}

// CStr buffers s followed by a null terminator.
func (sw *StreamWriter) CStr(s string) {
	if sw.err != nil {
		return
	}
	sw.buf.CStr(s)
	sw.afterWrite()
}

// Reserve buffers n zero bytes and returns the offset of the first one.
// Automatic flushing is suspended while any reserve is open, so the reserved
// bytes and the body written before the patch stay in the buffer. A Patch call
// whose position falls inside the reserved range releases it; flushing resumes
// on the next write or an explicit Flush. Flushing with an open Reserve panics.
func (sw *StreamWriter) Reserve(n int) int {
	if sw.err != nil {
		return 0
	}
	pos := sw.buf.Reserve(n)
	if n > 0 {
		sw.reserved = append(sw.reserved, reservation{start: pos, end: pos + n})
	}
	sw.afterWrite()
	return pos
}

// PatchU8 overwrites the buffered byte at pos.
func (sw *StreamWriter) PatchU8(pos int, v byte) {
	if sw.err != nil {
		return
	}
	sw.buf.PatchU8(pos, v)
	sw.release(pos)
}

// PatchU16LE overwrites a little-endian unsigned 16-bit value at pos.
func (sw *StreamWriter) PatchU16LE(pos int, v uint16) {
	if sw.err != nil {
		return
	}
	sw.buf.PatchU16LE(pos, v)
	sw.release(pos)
}

// PatchU16BE overwrites a big-endian unsigned 16-bit value at pos.
func (sw *StreamWriter) PatchU16BE(pos int, v uint16) {
	if sw.err != nil {
		return
	}
	sw.buf.PatchU16BE(pos, v)
	sw.release(pos)
}

// PatchU32LE overwrites a little-endian unsigned 32-bit value at pos.
func (sw *StreamWriter) PatchU32LE(pos int, v uint32) {
	if sw.err != nil {
		return
	}
	sw.buf.PatchU32LE(pos, v)
	sw.release(pos)
}

// PatchU32BE overwrites a big-endian unsigned 32-bit value at pos.
func (sw *StreamWriter) PatchU32BE(pos int, v uint32) {
	if sw.err != nil {
		return
	}
	sw.buf.PatchU32BE(pos, v)
	sw.release(pos)
}

// PatchU64LE overwrites a little-endian unsigned 64-bit value at pos.
func (sw *StreamWriter) PatchU64LE(pos int, v uint64) {
	if sw.err != nil {
		return
	}
	sw.buf.PatchU64LE(pos, v)
	sw.release(pos)
}

// PatchU64BE overwrites a big-endian unsigned 64-bit value at pos.
func (sw *StreamWriter) PatchU64BE(pos int, v uint64) {
	if sw.err != nil {
		return
	}
	sw.buf.PatchU64BE(pos, v)
	sw.release(pos)
}

// LenU8 runs f and prefixes the buffered bytes with their length as u8.
func (sw *StreamWriter) LenU8(f func(*Writer)) {
	if sw.err != nil {
		return
	}
	sw.buf.LenU8(f)
	sw.afterWrite()
}

// LenU16LE runs f and prefixes the buffered bytes with their length as
// little-endian u16.
func (sw *StreamWriter) LenU16LE(f func(*Writer)) {
	if sw.err != nil {
		return
	}
	sw.buf.LenU16LE(f)
	sw.afterWrite()
}

// LenU16BE runs f and prefixes the buffered bytes with their length as
// big-endian u16.
func (sw *StreamWriter) LenU16BE(f func(*Writer)) {
	if sw.err != nil {
		return
	}
	sw.buf.LenU16BE(f)
	sw.afterWrite()
}

// LenU32LE runs f and prefixes the buffered bytes with their length as
// little-endian u32.
func (sw *StreamWriter) LenU32LE(f func(*Writer)) {
	if sw.err != nil {
		return
	}
	sw.buf.LenU32LE(f)
	sw.afterWrite()
}

// LenU32BE runs f and prefixes the buffered bytes with their length as
// big-endian u32.
func (sw *StreamWriter) LenU32BE(f func(*Writer)) {
	if sw.err != nil {
		return
	}
	sw.buf.LenU32BE(f)
	sw.afterWrite()
}
