package bt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
)

var errBoom = errors.New("boom")

var (
	_ io.Writer       = (*Writer)(nil)
	_ io.ByteWriter   = (*Writer)(nil)
	_ io.StringWriter = (*Writer)(nil)
	_ io.WriterTo     = (*Writer)(nil)
	_ io.Writer       = (*StreamWriter)(nil)
	_ io.ByteWriter   = (*StreamWriter)(nil)
	_ io.StringWriter = (*StreamWriter)(nil)
)

type countingWriter struct {
	calls int
	n     int
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	cw.calls++
	cw.n += len(p)
	return len(p), nil
}

type errLimitWriter struct {
	limit int
	err   error
	got   []byte
}

func (ew *errLimitWriter) Write(p []byte) (int, error) {
	if len(p) > ew.limit {
		ew.got = append(ew.got, p[:ew.limit]...)
		return ew.limit, ew.err
	}
	ew.got = append(ew.got, p...)
	return len(p), nil
}

type shortWriter struct {
	limit int
}

func (sw shortWriter) Write(p []byte) (int, error) {
	if len(p) > sw.limit {
		return sw.limit, nil
	}
	return len(p), nil
}

type badCountWriter struct{}

func (badCountWriter) Write(p []byte) (int, error) {
	return len(p) + 1, nil
}

func TestWriterExactBytes(t *testing.T) {
	tests := []struct {
		name  string
		write func(w *Writer)
		want  []byte
	}{
		{"U8", func(w *Writer) { w.U8(0xAB) }, []byte{0xAB}},
		{"U16 order LE", func(w *Writer) { w.U16(binary.LittleEndian, 0x1234) }, []byte{0x34, 0x12}},
		{"U16 order BE", func(w *Writer) { w.U16(binary.BigEndian, 0x1234) }, []byte{0x12, 0x34}},
		{"U16LE", func(w *Writer) { w.U16LE(0x1234) }, []byte{0x34, 0x12}},
		{"U16BE", func(w *Writer) { w.U16BE(0x1234) }, []byte{0x12, 0x34}},
		{"U32 order LE", func(w *Writer) { w.U32(binary.LittleEndian, 0x12345678) }, []byte{0x78, 0x56, 0x34, 0x12}},
		{"U32 order BE", func(w *Writer) { w.U32(binary.BigEndian, 0x12345678) }, []byte{0x12, 0x34, 0x56, 0x78}},
		{"U32LE", func(w *Writer) { w.U32LE(0x12345678) }, []byte{0x78, 0x56, 0x34, 0x12}},
		{"U32BE", func(w *Writer) { w.U32BE(0x12345678) }, []byte{0x12, 0x34, 0x56, 0x78}},
		{"U64 order LE", func(w *Writer) { w.U64(binary.LittleEndian, 0x0102030405060708) }, []byte{8, 7, 6, 5, 4, 3, 2, 1}},
		{"U64 order BE", func(w *Writer) { w.U64(binary.BigEndian, 0x0102030405060708) }, []byte{1, 2, 3, 4, 5, 6, 7, 8}},
		{"U64LE", func(w *Writer) { w.U64LE(0x0102030405060708) }, []byte{8, 7, 6, 5, 4, 3, 2, 1}},
		{"U64BE", func(w *Writer) { w.U64BE(0x0102030405060708) }, []byte{1, 2, 3, 4, 5, 6, 7, 8}},
		{"I8", func(w *Writer) { w.I8(-2) }, []byte{0xFE}},
		{"I16 order LE", func(w *Writer) { w.I16(binary.LittleEndian, -2) }, []byte{0xFE, 0xFF}},
		{"I16 order BE", func(w *Writer) { w.I16(binary.BigEndian, -2) }, []byte{0xFF, 0xFE}},
		{"I16LE", func(w *Writer) { w.I16LE(-2) }, []byte{0xFE, 0xFF}},
		{"I16BE", func(w *Writer) { w.I16BE(-2) }, []byte{0xFF, 0xFE}},
		{"I32 order LE", func(w *Writer) { w.I32(binary.LittleEndian, -2) }, []byte{0xFE, 0xFF, 0xFF, 0xFF}},
		{"I32 order BE", func(w *Writer) { w.I32(binary.BigEndian, -2) }, []byte{0xFF, 0xFF, 0xFF, 0xFE}},
		{"I32LE", func(w *Writer) { w.I32LE(-2) }, []byte{0xFE, 0xFF, 0xFF, 0xFF}},
		{"I32BE", func(w *Writer) { w.I32BE(-2) }, []byte{0xFF, 0xFF, 0xFF, 0xFE}},
		{"I64 order LE", func(w *Writer) { w.I64(binary.LittleEndian, -2) }, []byte{0xFE, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}},
		{"I64 order BE", func(w *Writer) { w.I64(binary.BigEndian, -2) }, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE}},
		{"I64LE", func(w *Writer) { w.I64LE(-2) }, []byte{0xFE, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}},
		{"I64BE", func(w *Writer) { w.I64BE(-2) }, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE}},
		{"U24LE", func(w *Writer) { w.U24LE(0x123456) }, []byte{0x56, 0x34, 0x12}},
		{"U24BE", func(w *Writer) { w.U24BE(0x123456) }, []byte{0x12, 0x34, 0x56}},
		{"I24LE negative", func(w *Writer) { w.I24LE(-2) }, []byte{0xFE, 0xFF, 0xFF}},
		{"I24BE negative", func(w *Writer) { w.I24BE(-2) }, []byte{0xFF, 0xFF, 0xFE}},
		{"I24LE positive", func(w *Writer) { w.I24LE(0x123456) }, []byte{0x56, 0x34, 0x12}},
		{"I24BE positive", func(w *Writer) { w.I24BE(0x123456) }, []byte{0x12, 0x34, 0x56}},
		{"F32 order LE", func(w *Writer) { w.F32(binary.LittleEndian, 1.5) }, []byte{0x00, 0x00, 0xC0, 0x3F}},
		{"F32 order BE", func(w *Writer) { w.F32(binary.BigEndian, 1.5) }, []byte{0x3F, 0xC0, 0x00, 0x00}},
		{"F32LE", func(w *Writer) { w.F32LE(1.5) }, []byte{0x00, 0x00, 0xC0, 0x3F}},
		{"F32BE", func(w *Writer) { w.F32BE(1.5) }, []byte{0x3F, 0xC0, 0x00, 0x00}},
		{"F64 order LE", func(w *Writer) { w.F64(binary.LittleEndian, 1.5) }, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF8, 0x3F}},
		{"F64 order BE", func(w *Writer) { w.F64(binary.BigEndian, 1.5) }, []byte{0x3F, 0xF8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{"F64LE", func(w *Writer) { w.F64LE(1.5) }, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF8, 0x3F}},
		{"F64BE", func(w *Writer) { w.F64BE(1.5) }, []byte{0x3F, 0xF8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{"ULEB128 one byte", func(w *Writer) { w.ULEB128(0x7F) }, []byte{0x7F}},
		{"ULEB128 two bytes", func(w *Writer) { w.ULEB128(300) }, []byte{0xAC, 0x02}},
		{"ULEB128 max", func(w *Writer) { w.ULEB128(math.MaxUint64) }, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01}},
		{"SLEB128 zero", func(w *Writer) { w.SLEB128(0) }, []byte{0x00}},
		{"SLEB128 minus one", func(w *Writer) { w.SLEB128(-1) }, []byte{0x7F}},
		{"SLEB128 63", func(w *Writer) { w.SLEB128(63) }, []byte{0x3F}},
		{"SLEB128 64", func(w *Writer) { w.SLEB128(64) }, []byte{0xC0, 0x00}},
		{"SLEB128 -64", func(w *Writer) { w.SLEB128(-64) }, []byte{0x40}},
		{"SLEB128 -65", func(w *Writer) { w.SLEB128(-65) }, []byte{0xBF, 0x7F}},
		{"SLEB128 300", func(w *Writer) { w.SLEB128(300) }, []byte{0xAC, 0x02}},
		{"SLEB128 min", func(w *Writer) { w.SLEB128(math.MinInt64) }, []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x7F}},
		{"RawStr", func(w *Writer) { w.RawStr("a\x00b") }, []byte{'a', 0x00, 'b'}},
		{"RawStr empty", func(w *Writer) { w.RawStr("") }, nil},
		{"CStr", func(w *Writer) { w.CStr("hi") }, []byte{'h', 'i', 0x00}},
		{"CStr empty", func(w *Writer) { w.CStr("") }, []byte{0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewWriter()
			tt.write(w)
			if !bytes.Equal(w.Bytes(), tt.want) {
				t.Fatalf("Bytes() = % x, want % x", w.Bytes(), tt.want)
			}
		})
	}
}

func TestWriterU24Ranges(t *testing.T) {
	w := NewWriter()
	w.U24LE(0xFFFFFF)
	w.U24BE(0xFFFFFF)
	w.I24LE(-0x800000)
	w.I24BE(-0x800000)
	w.I24LE(0x7FFFFF)
	w.I24BE(0x7FFFFF)
	if got, want := len(w.Bytes()), 18; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}

	mustPanic(t, "bt: value 0x1000000 does not fit in 24 bits", func() { w.U24LE(0x1000000) })
	mustPanic(t, "bt: value 0x1000000 does not fit in 24 bits", func() { w.U24BE(0x1000000) })
	mustPanic(t, "bt: value -8388609 does not fit in 24 bits", func() { w.I24LE(-0x800001) })
	mustPanic(t, "bt: value -8388609 does not fit in 24 bits", func() { w.I24BE(-0x800001) })
	mustPanic(t, "bt: value 8388608 does not fit in 24 bits", func() { w.I24LE(0x800000) })
	mustPanic(t, "bt: value 8388608 does not fit in 24 bits", func() { w.I24BE(0x800000) })
}

func TestWriterCStrNulPanics(t *testing.T) {
	mustPanic(t, "bt: CStr contains NUL byte", func() { NewWriter().CStr("a\x00b") })
}

func TestWriterVarintRoundTrip(t *testing.T) {
	unsigned := []uint64{0, 1, 0x7F, 0x80, 0x81, 300, 0x3FFF, 0x4000, 1 << 21, 1 << 32, math.MaxUint64}
	for _, v := range unsigned {
		w := NewWriter()
		w.ULEB128(v)
		if got := binary.AppendUvarint(nil, v); !bytes.Equal(w.Bytes(), got) {
			t.Fatalf("ULEB128(%d) = % x, stdlib % x", v, w.Bytes(), got)
		}
		if got := NewCursor(w.Bytes()).ULEB128(); got != v {
			t.Fatalf("ULEB128(%d) round trip = %d", v, got)
		}
	}

	signed := []int64{0, 1, -1, 63, 64, -64, -65, 127, -128, 300, -300, 8192, -8192, 1 << 20, -(1 << 27), 1 << 40, -(1 << 40), math.MaxInt64, math.MinInt64}
	for _, v := range signed {
		w := NewWriter()
		w.SLEB128(v)
		if got := NewCursor(w.Bytes()).SLEB128(); got != v {
			t.Fatalf("SLEB128(%d) round trip = %d", v, got)
		}
	}
}

func TestWriterState(t *testing.T) {
	w := NewWriter()
	if w.Len() != 0 || len(w.Bytes()) != 0 {
		t.Fatal("new writer must be empty")
	}

	w.U32LE(0x11223344)
	capBefore := cap(w.Bytes())
	w.Reset()
	if w.Len() != 0 {
		t.Fatalf("Len() after Reset = %d, want 0", w.Len())
	}
	if cap(w.Bytes()) != capBefore {
		t.Fatalf("Reset dropped capacity: cap = %d, want %d", cap(w.Bytes()), capBefore)
	}

	w.RawStr("hello")
	w.Truncate(3)
	if got := string(w.Bytes()); got != "hel" {
		t.Fatalf("after Truncate = %q, want %q", got, "hel")
	}
	mustPanic(t, "bt: negative size -1", func() { w.Truncate(-1) })
	mustPanic(t, "bt: truncate size 4 out of range (len 3)", func() { w.Truncate(4) })

	w = NewWriter()
	w.U32LE(0x01020304)
	before := append([]byte(nil), w.Bytes()...)
	w.Grow(64)
	w.Grow(64)
	if !bytes.Equal(w.Bytes(), before) {
		t.Fatalf("Grow changed bytes: % x, want % x", w.Bytes(), before)
	}
	if got := cap(w.Bytes()) - len(w.Bytes()); got < 64 {
		t.Fatalf("Grow headroom = %d, want >= 64", got)
	}
	w.Grow(0)
	mustPanic(t, "bt: negative size -2", func() { w.Grow(-2) })
}

func TestWriterBytesAliasBuffer(t *testing.T) {
	w := NewWriter()
	w.U8(1)
	w.U8(2)

	b := w.Bytes()
	b[0] = 0xFF
	if got := w.Bytes()[0]; got != 0xFF {
		t.Fatalf("Bytes result must alias the writer buffer, got %#x", got)
	}
}

func TestWriterIO(t *testing.T) {
	w := NewWriter()
	if n, err := w.Write([]byte("ab")); n != 2 || err != nil {
		t.Fatalf("Write = (%d, %v), want (2, nil)", n, err)
	}
	if err := w.WriteByte('c'); err != nil {
		t.Fatalf("WriteByte = %v, want nil", err)
	}
	if n, err := w.WriteString("de"); n != 2 || err != nil {
		t.Fatalf("WriteString = (%d, %v), want (2, nil)", n, err)
	}
	if got := string(w.Bytes()); got != "abcde" {
		t.Fatalf("Bytes() = %q, want %q", got, "abcde")
	}

	cw := &countingWriter{}
	w.Reset()
	if n, err := w.WriteTo(cw); n != 0 || err != nil || cw.calls != 0 {
		t.Fatalf("empty WriteTo = (%d, %v), dst calls %d", n, err, cw.calls)
	}

	w.RawStr("xyz")
	capBefore := cap(w.Bytes())
	n, err := w.WriteTo(cw)
	if n != 3 || err != nil || cw.n != 3 {
		t.Fatalf("WriteTo = (%d, %v), dst wrote %d", n, err, cw.n)
	}
	if w.Len() != 0 || cap(w.Bytes()) != capBefore {
		t.Fatalf("WriteTo must drain but keep capacity: len %d, cap %d", w.Len(), cap(w.Bytes()))
	}

	ew := &errLimitWriter{limit: 2, err: errBoom}
	w.RawStr("hello")
	n, err = w.WriteTo(ew)
	if n != 2 || !errors.Is(err, errBoom) {
		t.Fatalf("WriteTo error = (%d, %v), want (2, boom)", n, err)
	}
	if got := string(w.Bytes()); got != "llo" {
		t.Fatalf("after partial WriteTo = %q, want %q", got, "llo")
	}
	if got := string(ew.got); got != "he" {
		t.Fatalf("dst got %q, want %q", got, "he")
	}

	w.Reset()
	w.RawStr("hello")
	n, err = w.WriteTo(shortWriter{limit: 2})
	if n != 2 || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short WriteTo = (%d, %v), want (2, ErrShortWrite)", n, err)
	}
	if got := string(w.Bytes()); got != "llo" {
		t.Fatalf("after short WriteTo = %q, want %q", got, "llo")
	}

	bad := NewWriter()
	bad.U8(1)
	mustPanic(t, "bt: invalid Write count", func() { bad.WriteTo(badCountWriter{}) })
}

func TestWriterReserve(t *testing.T) {
	w := NewWriter()
	if pos := w.Reserve(0); pos != 0 || w.Len() != 0 {
		t.Fatalf("Reserve(0) = %d, len %d", pos, w.Len())
	}

	pos := w.Reserve(4)
	if pos != 0 || w.Len() != 4 {
		t.Fatalf("Reserve(4) = %d, len %d", pos, w.Len())
	}
	for i, b := range w.Bytes() {
		if b != 0 {
			t.Fatalf("byte %d = %#x, want 0", i, b)
		}
	}

	w.RawStr("end")
	if pos := w.Reserve(300); pos != 7 || w.Len() != 307 {
		t.Fatalf("Reserve(300) = %d, len %d", pos, w.Len())
	}
	mustPanic(t, "bt: negative size -1", func() { w.Reserve(-1) })
}

func TestWriterAlign(t *testing.T) {
	w := NewWriter()
	if got := w.Align(4); got != 0 || w.Len() != 0 {
		t.Fatalf("Align(4) at length 0 = %d, len %d", got, w.Len())
	}
	w.U8(0xAA)
	if got := w.Align(4); got != 3 {
		t.Fatalf("Align(4) at length 1 = %d, want 3", got)
	}
	if want := []byte{0xAA, 0, 0, 0}; !bytes.Equal(w.Bytes(), want) {
		t.Fatalf("bytes = % x, want % x", w.Bytes(), want)
	}
	if got := w.Align(1); got != 0 {
		t.Fatalf("Align(1) = %d, want 0", got)
	}
	if got := w.Align(8); got != 4 {
		t.Fatalf("Align(8) at length 4 = %d, want 4", got)
	}
	mustPanic(t, "bt: bad align size 0", func() { w.Align(0) })
	mustPanic(t, "bt: bad align size -1", func() { w.Align(-1) })
}

func TestWriterPatch(t *testing.T) {
	tests := []struct {
		name  string
		patch func(w *Writer)
		want  []byte
	}{
		{"PatchU8", func(w *Writer) { w.PatchU8(4, 0xAB) }, []byte{0, 0, 0, 0, 0xAB, 0, 0, 0, 0, 0, 0, 0}},
		{"PatchU16LE", func(w *Writer) { w.PatchU16LE(4, 0x1234) }, []byte{0, 0, 0, 0, 0x34, 0x12, 0, 0, 0, 0, 0, 0}},
		{"PatchU16BE", func(w *Writer) { w.PatchU16BE(4, 0x1234) }, []byte{0, 0, 0, 0, 0x12, 0x34, 0, 0, 0, 0, 0, 0}},
		{"PatchU32LE", func(w *Writer) { w.PatchU32LE(4, 0x12345678) }, []byte{0, 0, 0, 0, 0x78, 0x56, 0x34, 0x12, 0, 0, 0, 0}},
		{"PatchU32BE", func(w *Writer) { w.PatchU32BE(4, 0x12345678) }, []byte{0, 0, 0, 0, 0x12, 0x34, 0x56, 0x78, 0, 0, 0, 0}},
		{"PatchU64LE", func(w *Writer) { w.PatchU64LE(4, 0x0102030405060708) }, []byte{0, 0, 0, 0, 8, 7, 6, 5, 4, 3, 2, 1}},
		{"PatchU64BE", func(w *Writer) { w.PatchU64BE(4, 0x0102030405060708) }, []byte{0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewWriter()
			w.Reserve(12)
			tt.patch(w)
			if !bytes.Equal(w.Bytes(), tt.want) {
				t.Fatalf("Bytes() = % x, want % x", w.Bytes(), tt.want)
			}
		})
	}

	w := NewWriter()
	w.U8(1)
	w.U8(2)
	mustPanic(t, "bt: patch at offset -1 out of range (len 2)", func() { w.PatchU8(-1, 0) })
	mustPanic(t, "bt: patch at offset 2 out of range (len 2)", func() { w.PatchU8(2, 0) })
	mustPanic(t, "bt: patch at offset 1 out of range (len 2)", func() { w.PatchU16LE(1, 0) })
	mustPanic(t, "bt: patch at offset 0 out of range (len 0)", func() { NewWriter().PatchU64BE(0, 0) })
}

func TestWriterSections(t *testing.T) {
	w := NewWriter()
	w.LenU8(func(w *Writer) { w.RawStr("ab") })
	w.LenU16LE(func(w *Writer) { w.RawStr("cd") })
	w.LenU16BE(func(w *Writer) { w.U8(1) })
	w.LenU32LE(func(w *Writer) {})
	w.LenU32BE(func(w *Writer) { w.CStr("x") })
	want := []byte{
		2, 'a', 'b',
		2, 0, 'c', 'd',
		0, 1, 1,
		0, 0, 0, 0,
		0, 0, 0, 2, 'x', 0x00,
	}
	if !bytes.Equal(w.Bytes(), want) {
		t.Fatalf("Bytes() = % x, want % x", w.Bytes(), want)
	}

	w = NewWriter()
	w.LenU8(func(w *Writer) {
		w.U8(1)
		w.LenU8(func(w *Writer) { w.RawStr("inner") })
	})
	if got, want := w.Bytes(), []byte{7, 1, 5, 'i', 'n', 'n', 'e', 'r'}; !bytes.Equal(got, want) {
		t.Fatalf("nested = % x, want % x", got, want)
	}
}

func TestWriterSectionLen(t *testing.T) {
	if got := sectionLen(0xFF, 8); got != 0xFF {
		t.Fatalf("sectionLen = %d, want 255", got)
	}
	if got := sectionLen(0xFFFFFFFF, 32); got != 0xFFFFFFFF {
		t.Fatalf("sectionLen = %d, want 0xFFFFFFFF", got)
	}
	mustPanic(t, "bt: section length 256 does not fit in 8 bits", func() { sectionLen(256, 8) })
	mustPanic(t, "bt: section length 65536 does not fit in 16 bits", func() { sectionLen(65536, 16) })
	mustPanic(t, "bt: section length 4294967296 does not fit in 32 bits", func() { sectionLen(1<<32, 32) })

	mustPanic(t, "bt: section length 256 does not fit in 8 bits", func() {
		NewWriter().LenU8(func(w *Writer) { w.Write(bytes.Repeat([]byte{1}, 256)) })
	})
	mustPanic(t, "bt: section length 65536 does not fit in 16 bits", func() {
		NewWriter().LenU16BE(func(w *Writer) { w.Write(bytes.Repeat([]byte{1}, 65536)) })
	})
}

func TestWriterAllocations(t *testing.T) {
	w := NewWriter()
	w.Grow(64)

	allocs := testing.AllocsPerRun(100, func() {
		w.Reset()
		w.U8(1)
		w.U16LE(2)
		w.U16BE(3)
		w.U32LE(4)
		w.U32BE(5)
		w.U64LE(6)
		w.U24LE(7)
		w.I24BE(-8)
		w.F64LE(9)
		w.ULEB128(300)
		w.SLEB128(-300)
		w.RawStr("hello")
		w.CStr("hi")
	})
	if allocs > 0 {
		t.Fatalf("writer allocations = %v, want 0", allocs)
	}
}

func FuzzWriterScalarsRoundTrip(f *testing.F) {
	f.Add(uint64(0), uint64(0), uint64(0))
	f.Add(^uint64(0), ^uint64(0), ^uint64(0))

	f.Fuzz(func(t *testing.T, a, b, c uint64) {
		w := NewWriter()
		w.U8(byte(a))
		w.U16LE(uint16(a))
		w.U16BE(uint16(b))
		w.U32LE(uint32(a))
		w.U32BE(uint32(b))
		w.U64LE(a)
		w.U64BE(b)
		w.U24LE(uint32(a) & 0xFFFFFF)
		w.U24BE(uint32(b) & 0xFFFFFF)
		w.I8(int8(a))
		w.I16LE(int16(a))
		w.I16BE(int16(b))
		w.I32LE(int32(a))
		w.I32BE(int32(b))
		w.I64LE(int64(a))
		w.I64BE(int64(b))
		w.I24LE(int32(c&0xFFFFFF) - 0x800000)
		w.I24BE(int32(c&0xFFFFFF) - 0x800000)
		w.F32LE(math.Float32frombits(uint32(a)))
		w.F32BE(math.Float32frombits(uint32(b)))
		w.F64LE(math.Float64frombits(a))
		w.F64BE(math.Float64frombits(b))
		w.ULEB128(a)
		w.SLEB128(int64(b))
		raw := string([]byte{byte(c), 0x00, byte(a)})
		w.RawStr(raw)
		w.CStr("cstr")

		cur := NewCursor(w.Bytes())
		if got := cur.U8(); got != byte(a) {
			t.Fatalf("U8 = %d, want %d", got, byte(a))
		}
		if got := cur.U16LE(); got != uint16(a) {
			t.Fatalf("U16LE = %d, want %d", got, uint16(a))
		}
		if got := cur.U16BE(); got != uint16(b) {
			t.Fatalf("U16BE = %d, want %d", got, uint16(b))
		}
		if got := cur.U32LE(); got != uint32(a) {
			t.Fatalf("U32LE = %d, want %d", got, uint32(a))
		}
		if got := cur.U32BE(); got != uint32(b) {
			t.Fatalf("U32BE = %d, want %d", got, uint32(b))
		}
		if got := cur.U64LE(); got != a {
			t.Fatalf("U64LE = %d, want %d", got, a)
		}
		if got := cur.U64BE(); got != b {
			t.Fatalf("U64BE = %d, want %d", got, b)
		}
		if got := cur.U24LE(); got != uint32(a)&0xFFFFFF {
			t.Fatalf("U24LE = %d, want %d", got, uint32(a)&0xFFFFFF)
		}
		if got := cur.U24BE(); got != uint32(b)&0xFFFFFF {
			t.Fatalf("U24BE = %d, want %d", got, uint32(b)&0xFFFFFF)
		}
		if got := cur.I8(); got != int8(a) {
			t.Fatalf("I8 = %d, want %d", got, int8(a))
		}
		if got := cur.I16LE(); got != int16(a) {
			t.Fatalf("I16LE = %d, want %d", got, int16(a))
		}
		if got := cur.I16BE(); got != int16(b) {
			t.Fatalf("I16BE = %d, want %d", got, int16(b))
		}
		if got := cur.I32LE(); got != int32(a) {
			t.Fatalf("I32LE = %d, want %d", got, int32(a))
		}
		if got := cur.I32BE(); got != int32(b) {
			t.Fatalf("I32BE = %d, want %d", got, int32(b))
		}
		if got := cur.I64LE(); got != int64(a) {
			t.Fatalf("I64LE = %d, want %d", got, int64(a))
		}
		if got := cur.I64BE(); got != int64(b) {
			t.Fatalf("I64BE = %d, want %d", got, int64(b))
		}
		if got := cur.I24LE(); got != int32(c&0xFFFFFF)-0x800000 {
			t.Fatalf("I24LE = %d, want %d", got, int32(c&0xFFFFFF)-0x800000)
		}
		if got := cur.I24BE(); got != int32(c&0xFFFFFF)-0x800000 {
			t.Fatalf("I24BE = %d, want %d", got, int32(c&0xFFFFFF)-0x800000)
		}
		if got, want := cur.F32LE(), math.Float32frombits(uint32(a)); math.Float32bits(got) != math.Float32bits(want) {
			t.Fatalf("F32LE = %v, want %v", got, want)
		}
		if got, want := cur.F32BE(), math.Float32frombits(uint32(b)); math.Float32bits(got) != math.Float32bits(want) {
			t.Fatalf("F32BE = %v, want %v", got, want)
		}
		if got, want := cur.F64LE(), math.Float64frombits(a); math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("F64LE = %v, want %v", got, want)
		}
		if got, want := cur.F64BE(), math.Float64frombits(b); math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("F64BE = %v, want %v", got, want)
		}
		if got := cur.ULEB128(); got != a {
			t.Fatalf("ULEB128 = %d, want %d", got, a)
		}
		if got := cur.SLEB128(); got != int64(b) {
			t.Fatalf("SLEB128 = %d, want %d", got, int64(b))
		}
		if got := cur.RawStr(len(raw)); got != raw {
			t.Fatalf("RawStr = %q, want %q", got, raw)
		}
		if got := cur.StrOrRest(5); got != "cstr" {
			t.Fatalf("CStr = %q, want %q", got, "cstr")
		}
		if cur.BytesLeft() != 0 {
			t.Fatalf("BytesLeft = %d, want 0", cur.BytesLeft())
		}
	})
}

func FuzzWriterSectionsRoundTrip(f *testing.F) {
	f.Add([]byte("hello"), uint8(0))
	f.Add([]byte{}, uint8(4))

	f.Fuzz(func(t *testing.T, data []byte, op uint8) {
		switch op % 5 {
		case 0:
			if len(data) > 0xFF {
				return
			}
		case 1, 2:
			if len(data) > 0xFFFF {
				return
			}
		}

		w := NewWriter()
		switch op % 5 {
		case 0:
			w.LenU8(func(w *Writer) { w.Write(data) })
		case 1:
			w.LenU16LE(func(w *Writer) { w.Write(data) })
		case 2:
			w.LenU16BE(func(w *Writer) { w.Write(data) })
		case 3:
			w.LenU32LE(func(w *Writer) { w.Write(data) })
		case 4:
			w.LenU32BE(func(w *Writer) { w.Write(data) })
		}

		cur := NewCursor(w.Bytes())
		var size int
		switch op % 5 {
		case 0:
			size = int(cur.U8())
		case 1:
			size = int(cur.U16LE())
		case 2:
			size = int(cur.U16BE())
		case 3:
			size = int(cur.U32LE())
		case 4:
			size = int(cur.U32BE())
		}
		if size != len(data) {
			t.Fatalf("section length = %d, want %d", size, len(data))
		}
		if !bytes.Equal(cur.Bytes(size), data) {
			t.Fatalf("section payload mismatch")
		}
		if cur.BytesLeft() != 0 {
			t.Fatalf("BytesLeft = %d, want 0", cur.BytesLeft())
		}
	})
}
