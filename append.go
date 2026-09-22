package bt

import "math/bits"

// AppendU24LE appends a little-endian unsigned 24-bit value to b and returns
// the extended buffer. It panics if v does not fit in 24 bits.
func AppendU24LE(b []byte, v uint32) []byte {
	w := Writer{b: b}
	w.U24LE(v)
	return w.b
}

// AppendU24BE appends a big-endian unsigned 24-bit value to b and returns the
// extended buffer. It panics if v does not fit in 24 bits.
func AppendU24BE(b []byte, v uint32) []byte {
	w := Writer{b: b}
	w.U24BE(v)
	return w.b
}

// AppendULEB128 appends an unsigned LEB128 varint to b and returns the
// extended buffer.
func AppendULEB128(b []byte, v uint64) []byte {
	w := Writer{b: b}
	w.ULEB128(v)
	return w.b
}

// AppendSLEB128 appends a signed LEB128 varint to b and returns the extended
// buffer.
func AppendSLEB128(b []byte, v int64) []byte {
	w := Writer{b: b}
	w.SLEB128(v)
	return w.b
}

// ULEB128Size returns the number of bytes AppendULEB128 appends for v.
func ULEB128Size(v uint64) int {
	return (bits.Len64(v|1) + 6) / 7
}

// SLEB128Size returns the number of bytes AppendSLEB128 appends for v.
func SLEB128Size(v int64) int {
	return (bits.Len64((uint64(v)^uint64(v>>63))|1) + 7) / 7
}
