package bt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

type failingWriter struct {
	err error
}

func (fw failingWriter) Write(p []byte) (int, error) {
	return 0, fw.err
}

type sampleWriter interface {
	U8(byte)
	U16(binary.ByteOrder, uint16)
	U16LE(uint16)
	U16BE(uint16)
	U32(binary.ByteOrder, uint32)
	U32LE(uint32)
	U32BE(uint32)
	U64(binary.ByteOrder, uint64)
	U64LE(uint64)
	U64BE(uint64)
	I8(int8)
	I16(binary.ByteOrder, int16)
	I16LE(int16)
	I16BE(int16)
	I32(binary.ByteOrder, int32)
	I32LE(int32)
	I32BE(int32)
	I64(binary.ByteOrder, int64)
	I64LE(int64)
	I64BE(int64)
	U24LE(uint32)
	U24BE(uint32)
	I24LE(int32)
	I24BE(int32)
	F32(binary.ByteOrder, float32)
	F32LE(float32)
	F32BE(float32)
	F64(binary.ByteOrder, float64)
	F64LE(float64)
	F64BE(float64)
	ULEB128(uint64)
	SLEB128(int64)
	RawStr(string)
	CStr(string)
	Reserve(int) int
	PatchU8(int, byte)
	PatchU16LE(int, uint16)
	PatchU16BE(int, uint16)
	PatchU32LE(int, uint32)
	PatchU32BE(int, uint32)
	PatchU64LE(int, uint64)
	PatchU64BE(int, uint64)
	LenU8(func(*Writer))
	LenU16LE(func(*Writer))
	LenU16BE(func(*Writer))
	LenU32LE(func(*Writer))
	LenU32BE(func(*Writer))
}

func writeSample(w sampleWriter) {
	w.U8(0x12)
	w.U16(binary.LittleEndian, 0x1234)
	w.U16LE(0x5678)
	w.U16BE(0x9ABC)
	w.U32(binary.BigEndian, 0x12345678)
	w.U32LE(0xAABBCCDD)
	w.U32BE(0xDDCCBBAA)
	w.U64(binary.LittleEndian, 0x1122334455667788)
	w.U64LE(0x8877665544332211)
	w.U64BE(0x0102030405060708)
	w.I8(-1)
	w.I16(binary.LittleEndian, -2)
	w.I16LE(-3)
	w.I16BE(-4)
	w.I32(binary.BigEndian, -5)
	w.I32LE(-6)
	w.I32BE(-7)
	w.I64(binary.LittleEndian, -8)
	w.I64LE(-9)
	w.I64BE(-10)
	w.U24LE(0x123456)
	w.U24BE(0x654321)
	w.I24LE(-100)
	w.I24BE(100)
	w.F32(binary.LittleEndian, 1.5)
	w.F32LE(-2.5)
	w.F32BE(3.5)
	w.F64(binary.BigEndian, 1.25)
	w.F64LE(-4.5)
	w.F64BE(5.5)
	w.ULEB128(300)
	w.SLEB128(-300)
	w.RawStr("raw")
	w.CStr("cstr")

	p := w.Reserve(1)
	w.PatchU8(p, 1)
	p = w.Reserve(2)
	w.PatchU16LE(p, 2)
	p = w.Reserve(2)
	w.PatchU16BE(p, 3)
	p = w.Reserve(4)
	w.PatchU32LE(p, 4)
	p = w.Reserve(4)
	w.PatchU32BE(p, 5)
	p = w.Reserve(8)
	w.PatchU64LE(p, 6)
	p = w.Reserve(8)
	w.PatchU64BE(p, 7)

	w.LenU8(func(bw *Writer) { bw.U8(1) })
	w.LenU16LE(func(bw *Writer) { bw.RawStr("ab") })
	w.LenU16BE(func(bw *Writer) { bw.U16LE(9) })
	w.LenU32LE(func(bw *Writer) { bw.CStr("x") })
	w.LenU32BE(func(bw *Writer) {})
}

func TestStreamWriterMatchesWriter(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(8))
	writeSample(sw)
	if err := sw.Flush(); err != nil {
		t.Fatalf("Flush = %v, want nil", err)
	}
	if sw.Err() != nil {
		t.Fatalf("Err = %v, want nil", sw.Err())
	}
	if sw.Buffered() != 0 {
		t.Fatalf("Buffered = %d, want 0", sw.Buffered())
	}

	w := NewWriter()
	writeSample(w)
	if !bytes.Equal(buf.Bytes(), w.Bytes()) {
		t.Fatalf("stream bytes differ from writer bytes:\nstream % x\nwriter % x", buf.Bytes(), w.Bytes())
	}
}

func TestStreamWriterFlushThreshold(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(4))
	sw.U16LE(1)
	if buf.Len() != 0 {
		t.Fatalf("flushed early: %d bytes", buf.Len())
	}
	sw.U16LE(2)
	if buf.Len() != 4 {
		t.Fatalf("threshold flush = %d bytes, want 4", buf.Len())
	}
	if sw.Buffered() != 0 {
		t.Fatalf("Buffered = %d, want 0", sw.Buffered())
	}
	if err := sw.Flush(); err != nil {
		t.Fatalf("empty Flush = %v", err)
	}
}

func TestStreamWriterDefaultThreshold(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf)
	payload := bytes.Repeat([]byte{0xAB}, streamWriterFlushThreshold)
	if n, err := sw.Write(payload); n != len(payload) || err != nil {
		t.Fatalf("Write = (%d, %v)", n, err)
	}
	if buf.Len() != len(payload) {
		t.Fatalf("default threshold flush = %d bytes, want %d", buf.Len(), len(payload))
	}
	if sw.Buffered() != 0 {
		t.Fatalf("Buffered = %d, want 0", sw.Buffered())
	}
}

func TestStreamWriterManualFlush(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(0))
	sw.U32LE(1)
	if err := sw.WriteByte(2); err != nil {
		t.Fatalf("WriteByte = %v", err)
	}
	if n, err := sw.WriteString("ab"); n != 2 || err != nil {
		t.Fatalf("WriteString = (%d, %v)", n, err)
	}
	if buf.Len() != 0 {
		t.Fatalf("manual mode flushed early: %d bytes", buf.Len())
	}
	if err := sw.Flush(); err != nil {
		t.Fatalf("Flush = %v", err)
	}
	if got, want := buf.Len(), 7; got != want {
		t.Fatalf("flushed %d bytes, want %d", got, want)
	}
}

func TestStreamWriterShortWrite(t *testing.T) {
	sw := NewStreamWriter(shortWriter{limit: 2}, WithFlushThreshold(0))
	sw.U32LE(1)
	if err := sw.Flush(); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Flush = %v, want ErrShortWrite", err)
	}
	if !errors.Is(sw.Err(), io.ErrShortWrite) {
		t.Fatalf("Err = %v, want ErrShortWrite", sw.Err())
	}
	if sw.Buffered() != 0 {
		t.Fatalf("Buffered = %d, want 0 after failure", sw.Buffered())
	}
	if err := sw.Flush(); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("sticky Flush = %v, want ErrShortWrite", err)
	}
}

func TestStreamWriterStickyError(t *testing.T) {
	sw := NewStreamWriter(failingWriter{err: errBoom}, WithFlushThreshold(1))
	sw.U8(1)
	if !errors.Is(sw.Err(), errBoom) {
		t.Fatalf("Err = %v, want boom", sw.Err())
	}
	if sw.Buffered() != 0 {
		t.Fatalf("Buffered = %d, want 0", sw.Buffered())
	}

	if n, err := sw.Write([]byte("x")); n != 0 || !errors.Is(err, errBoom) {
		t.Fatalf("Write after error = (%d, %v)", n, err)
	}
	if err := sw.WriteByte(1); !errors.Is(err, errBoom) {
		t.Fatalf("WriteByte after error = %v", err)
	}
	if n, err := sw.WriteString("x"); n != 0 || !errors.Is(err, errBoom) {
		t.Fatalf("WriteString after error = (%d, %v)", n, err)
	}

	called := false
	section := func(*Writer) { called = true }
	ops := []struct {
		name string
		fn   func()
	}{
		{"U8", func() { sw.U8(1) }},
		{"U16", func() { sw.U16(binary.LittleEndian, 1) }},
		{"U16LE", func() { sw.U16LE(1) }},
		{"U16BE", func() { sw.U16BE(1) }},
		{"U32", func() { sw.U32(binary.LittleEndian, 1) }},
		{"U32LE", func() { sw.U32LE(1) }},
		{"U32BE", func() { sw.U32BE(1) }},
		{"U64", func() { sw.U64(binary.LittleEndian, 1) }},
		{"U64LE", func() { sw.U64LE(1) }},
		{"U64BE", func() { sw.U64BE(1) }},
		{"I8", func() { sw.I8(1) }},
		{"I16", func() { sw.I16(binary.LittleEndian, 1) }},
		{"I16LE", func() { sw.I16LE(1) }},
		{"I16BE", func() { sw.I16BE(1) }},
		{"I32", func() { sw.I32(binary.LittleEndian, 1) }},
		{"I32LE", func() { sw.I32LE(1) }},
		{"I32BE", func() { sw.I32BE(1) }},
		{"I64", func() { sw.I64(binary.LittleEndian, 1) }},
		{"I64LE", func() { sw.I64LE(1) }},
		{"I64BE", func() { sw.I64BE(1) }},
		{"U24LE", func() { sw.U24LE(1) }},
		{"U24BE", func() { sw.U24BE(1) }},
		{"I24LE", func() { sw.I24LE(1) }},
		{"I24BE", func() { sw.I24BE(1) }},
		{"F32", func() { sw.F32(binary.LittleEndian, 1) }},
		{"F32LE", func() { sw.F32LE(1) }},
		{"F32BE", func() { sw.F32BE(1) }},
		{"F64", func() { sw.F64(binary.LittleEndian, 1) }},
		{"F64LE", func() { sw.F64LE(1) }},
		{"F64BE", func() { sw.F64BE(1) }},
		{"ULEB128", func() { sw.ULEB128(1) }},
		{"SLEB128", func() { sw.SLEB128(1) }},
		{"RawStr", func() { sw.RawStr("x") }},
		{"CStr", func() { sw.CStr("x") }},
		{"Reserve", func() { sw.Reserve(4) }},
		{"PatchU8", func() { sw.PatchU8(0, 1) }},
		{"PatchU16LE", func() { sw.PatchU16LE(0, 1) }},
		{"PatchU16BE", func() { sw.PatchU16BE(0, 1) }},
		{"PatchU32LE", func() { sw.PatchU32LE(0, 1) }},
		{"PatchU32BE", func() { sw.PatchU32BE(0, 1) }},
		{"PatchU64LE", func() { sw.PatchU64LE(0, 1) }},
		{"PatchU64BE", func() { sw.PatchU64BE(0, 1) }},
		{"Grow", func() { sw.Grow(8) }},
		{"LenU8", func() { sw.LenU8(section) }},
		{"LenU16LE", func() { sw.LenU16LE(section) }},
		{"LenU16BE", func() { sw.LenU16BE(section) }},
		{"LenU32LE", func() { sw.LenU32LE(section) }},
		{"LenU32BE", func() { sw.LenU32BE(section) }},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			op.fn()
		})
	}
	if called {
		t.Fatal("section callbacks must not run after a sticky error")
	}
	if sw.Buffered() != 0 {
		t.Fatalf("Buffered = %d, want 0", sw.Buffered())
	}
	if err := sw.Flush(); !errors.Is(err, errBoom) {
		t.Fatalf("sticky Flush = %v, want boom", err)
	}

	sw.Reset()
	if sw.Err() != nil || sw.Buffered() != 0 {
		t.Fatalf("Reset left err %v / buffered %d", sw.Err(), sw.Buffered())
	}
}

func TestStreamWriterInvalidValues(t *testing.T) {
	sw := NewStreamWriter(io.Discard, WithFlushThreshold(0))
	mustPanic(t, "bt: value 0x1000000 does not fit in 24 bits", func() { sw.U24LE(0x1000000) })
	mustPanic(t, "bt: CStr contains NUL byte", func() { sw.CStr("a\x00b") })

	sticky := NewStreamWriter(failingWriter{err: errBoom}, WithFlushThreshold(0))
	sticky.U8(1)
	if err := sticky.Flush(); !errors.Is(err, errBoom) {
		t.Fatalf("Flush = %v, want boom", err)
	}
	sticky.U24LE(0x1000000)
	sticky.CStr("a\x00b")
	if !errors.Is(sticky.Err(), errBoom) {
		t.Fatalf("Err = %v, want boom", sticky.Err())
	}
}

func TestStreamWriterResetReuse(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(0))
	sw.U8(1)
	sw.Reset()
	if sw.Buffered() != 0 || buf.Len() != 0 {
		t.Fatalf("Reset kept %d buffered / %d written bytes", sw.Buffered(), buf.Len())
	}
	sw.U8(2)
	if err := sw.Flush(); err != nil {
		t.Fatalf("Flush = %v", err)
	}
	if !bytes.Equal(buf.Bytes(), []byte{2}) {
		t.Fatalf("reused stream wrote % x, want 02", buf.Bytes())
	}
}

func TestStreamWriterSections(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(3))
	sw.LenU16LE(func(w *Writer) { w.RawStr("hello") })
	pos := sw.Reserve(2)
	sw.PatchU16BE(pos, 9)
	if err := sw.Flush(); err != nil {
		t.Fatalf("Flush = %v", err)
	}

	c := NewCursor(buf.Bytes())
	if got := c.U16LE(); got != 5 {
		t.Fatalf("section size = %d, want 5", got)
	}
	if got := c.RawStr(5); got != "hello" {
		t.Fatalf("section payload = %q", got)
	}
	if got := c.U16BE(); got != 9 {
		t.Fatalf("patched value = %d, want 9", got)
	}
	if c.BytesLeft() != 0 {
		t.Fatalf("BytesLeft = %d, want 0", c.BytesLeft())
	}
}

func TestStreamWriterReserveSuspendsFlush(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(4))
	p := sw.Reserve(2)
	if sw.Buffered() != 2 {
		t.Fatalf("Buffered = %d, want 2", sw.Buffered())
	}
	sw.RawStr("abcd")
	if buf.Len() != 0 {
		t.Fatalf("flushed with open Reserve: %d bytes", buf.Len())
	}
	if sw.Buffered() != 6 {
		t.Fatalf("Buffered = %d, want 6", sw.Buffered())
	}
	mustPanic(t, "bt: Flush with open Reserve", func() { sw.Flush() })

	sw.PatchU16LE(p, 3)
	if buf.Len() != 0 {
		t.Fatalf("patch flushed on its own: %d bytes", buf.Len())
	}
	sw.U8(9)
	if got := buf.Len(); got != 7 {
		t.Fatalf("flush after patch = %d bytes, want 7", got)
	}
	if sw.Buffered() != 0 {
		t.Fatalf("Buffered = %d, want 0", sw.Buffered())
	}
	c := NewCursor(buf.Bytes())
	if got := c.U16LE(); got != 3 {
		t.Fatalf("patched value = %d, want 3", got)
	}
	if got := c.RawStr(4); got != "abcd" {
		t.Fatalf("payload = %q", got)
	}
	if got := c.U8(); got != 9 {
		t.Fatalf("trailing byte = %d, want 9", got)
	}
}

func TestStreamWriterInterleavedPatch(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(4))
	pos := sw.Reserve(2)
	sw.RawStr("abcd")
	sw.U8(0)
	sw.PatchU8(6, 1)
	if buf.Len() != 0 {
		t.Fatalf("unrelated patch released the reserve: %d bytes flushed", buf.Len())
	}
	sw.PatchU16LE(pos, 3)
	if buf.Len() != 0 {
		t.Fatalf("patch flushed: %d bytes", buf.Len())
	}
	sw.U8(2)

	c := NewCursor(buf.Bytes())
	if got := c.U16LE(); got != 3 {
		t.Fatalf("patched value = %d, want 3", got)
	}
	if got := c.RawStr(4); got != "abcd" {
		t.Fatalf("payload = %q", got)
	}
	if got := c.U8(); got != 1 {
		t.Fatalf("interleaved byte = %d, want 1", got)
	}
	if got := c.U8(); got != 2 {
		t.Fatalf("trailing byte = %d, want 2", got)
	}
	if c.BytesLeft() != 0 {
		t.Fatalf("BytesLeft = %d, want 0", c.BytesLeft())
	}
}

func TestStreamWriterPatchInsideReserve(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(1))
	pos := sw.Reserve(4)
	sw.PatchU8(pos+3, 9)
	sw.U8(1)

	if want := []byte{0, 0, 0, 9, 1}; !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("bytes = % x, want % x", buf.Bytes(), want)
	}
}

func TestStreamWriterRepeatedPatches(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(2))
	pos := sw.Reserve(4)
	sw.PatchU8(pos, 1)
	sw.PatchU8(pos+1, 2)
	sw.U8(3)

	if want := []byte{1, 2, 0, 0, 3}; !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("bytes = % x, want % x", buf.Bytes(), want)
	}
}

func TestStreamWriterIOFlushError(t *testing.T) {
	sw := NewStreamWriter(failingWriter{err: errBoom}, WithFlushThreshold(2))
	if n, err := sw.Write([]byte("ab")); n != 2 || !errors.Is(err, errBoom) {
		t.Fatalf("triggering Write = (%d, %v), want (2, boom)", n, err)
	}

	sw = NewStreamWriter(failingWriter{err: errBoom}, WithFlushThreshold(1))
	if err := sw.WriteByte(1); !errors.Is(err, errBoom) {
		t.Fatalf("triggering WriteByte = %v, want boom", err)
	}

	sw = NewStreamWriter(failingWriter{err: errBoom}, WithFlushThreshold(1))
	if n, err := sw.WriteString("a"); n != 1 || !errors.Is(err, errBoom) {
		t.Fatalf("triggering WriteString = (%d, %v), want (1, boom)", n, err)
	}

	var buf bytes.Buffer
	sw = NewStreamWriter(&buf, WithFlushThreshold(2))
	if n, err := sw.Write([]byte("ab")); n != 2 || err != nil {
		t.Fatalf("Write = (%d, %v), want (2, nil)", n, err)
	}
	if err := sw.WriteByte('c'); err != nil {
		t.Fatalf("WriteByte = %v, want nil", err)
	}
	if n, err := sw.WriteString("de"); n != 2 || err != nil {
		t.Fatalf("WriteString = (%d, %v), want (2, nil)", n, err)
	}
	if !bytes.Equal(buf.Bytes(), []byte("abcde")) {
		t.Fatalf("destination = %q, want %q", buf.Bytes(), "abcde")
	}

	sw = NewStreamWriter(failingWriter{err: errBoom})
	if _, err := io.Copy(sw, bytes.NewReader(make([]byte, streamWriterFlushThreshold))); !errors.Is(err, errBoom) {
		t.Fatalf("io.Copy = %v, want boom", err)
	}
}

func TestStreamWriterResetClearsReserve(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(4))
	sw.Reserve(2)
	sw.U8(1)
	sw.Reset()
	sw.U32LE(2)
	if buf.Len() != 4 {
		t.Fatalf("flush after Reset = %d bytes, want 4", buf.Len())
	}
}

func TestStreamWriterGrow(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(0))
	sw.Grow(64)

	allocs := testing.AllocsPerRun(100, func() {
		sw.Reset()
		sw.U32LE(1)
		sw.U64LE(2)
		sw.RawStr("0123456789abcdef")
	})
	if allocs > 0 {
		t.Fatalf("writes after Grow allocated %v times, want 0", allocs)
	}
	mustPanic(t, "bt: negative size -1", func() { sw.Grow(-1) })

	failing := NewStreamWriter(failingWriter{err: errBoom}, WithFlushThreshold(1))
	failing.U8(1)
	mustPanic(t, "bt: negative size -1", func() { failing.Grow(-1) })
	mustPanic(t, "bt: negative size -1", func() { failing.Reserve(-1) })
	if !errors.Is(failing.Err(), errBoom) {
		t.Fatalf("Err = %v, want boom", failing.Err())
	}
}

func TestStreamWriterReserveZeroDoesNotFlush(t *testing.T) {
	var buf bytes.Buffer
	sw := NewStreamWriter(&buf, WithFlushThreshold(1))

	p := sw.Reserve(1)
	sw.U8(1)
	sw.PatchU8(p, 0)
	if buf.Len() != 0 {
		t.Fatalf("patch flushed %d bytes", buf.Len())
	}

	sw.Reserve(0)
	if buf.Len() != 0 {
		t.Fatalf("Reserve(0) flushed %d bytes", buf.Len())
	}

	sw.U8(2)
	if want := []byte{0, 1, 2}; !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("bytes = % x, want % x", buf.Bytes(), want)
	}
}
