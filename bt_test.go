package bt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

func mustPanic(t *testing.T, wantPrefix string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		if msg := fmt.Sprint(r); !strings.HasPrefix(msg, wantPrefix) {
			t.Fatalf("panic = %q, want prefix %q", msg, wantPrefix)
		}
	}()
	fn()
}

func TestCStr(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		want    string
		wantErr bool
	}{
		{"terminated", []byte("hello\x00world"), "hello", false},
		{"leading nul", []byte{0x00, 'x'}, "", false},
		{"only nul", []byte{0x00}, "", false},
		{"nul at end", []byte("abc\x00"), "abc", false},
		{"unterminated", []byte("hello"), "", true},
		{"empty", []byte{}, "", true},
		{"nil", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CStr(tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrNoNul) {
					t.Fatalf("CStr(%q) error = %v, want ErrNoNul", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CStr(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("CStr(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCStrOrRest(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"terminated", []byte("hello\x00world"), "hello"},
		{"unterminated", []byte("hello"), "hello"},
		{"empty", []byte{}, ""},
		{"nil", nil, ""},
		{"leading nul", []byte{0x00, 'x'}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CStrOrRest(tt.in); got != tt.want {
				t.Fatalf("CStrOrRest(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCursorReads(t *testing.T) {
	buf := []byte{
		0x01,       // u8
		0x02, 0x03, // u16le -> 0x0302
		0x04, 0x05, // u16be -> 0x0405
		0x06, 0x07, 0x08, 0x09, // u32le -> 0x09080706
		0x0a, 0x0b, 0x0c, 0x0d, // u32be -> 0x0a0b0c0d
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, // u64le
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, // u64be
		'h', 'i', 0x00, 0xde, 0xad, // str(5) -> "hi"
		0xbe, 0xef, // leftover
	}

	c := NewCursor(buf)
	if got, want := c.BytesLeft(), len(buf); got != want {
		t.Fatalf("BytesLeft() = %d, want %d", got, want)
	}

	if got := c.U8(); got != 0x01 {
		t.Errorf("U8() = %#x, want 0x01", got)
	}
	if got := c.U16(binary.LittleEndian); got != 0x0302 {
		t.Errorf("U16(LittleEndian) = %#x, want 0x0302", got)
	}
	if got := c.U16BE(); got != 0x0405 {
		t.Errorf("U16BE() = %#x, want 0x0405", got)
	}
	if got := c.U32LE(); got != 0x09080706 {
		t.Errorf("U32LE() = %#x, want 0x09080706", got)
	}
	if got := c.U32(binary.BigEndian); got != 0x0a0b0c0d {
		t.Errorf("U32(BigEndian) = %#x, want 0x0a0b0c0d", got)
	}
	if got := c.U64LE(); got != 0x0807060504030201 {
		t.Errorf("U64LE() = %#x, want 0x0807060504030201", got)
	}
	if got := c.U64BE(); got != 0x1112131415161718 {
		t.Errorf("U64BE() = %#x, want 0x1112131415161718", got)
	}
	if got := c.StrOrRest(5); got != "hi" {
		t.Errorf("StrOrRest(5) = %q, want %q", got, "hi")
	}
	if got, want := c.BytesLeft(), 2; got != want {
		t.Fatalf("BytesLeft() = %d, want %d", got, want)
	}
}

func TestCursorSignedReads(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		read func(c *Cursor) int64
		want int64
	}{
		{"I8", []byte{0xFF}, func(c *Cursor) int64 { return int64(c.I8()) }, -1},
		{"I8 min", []byte{0x80}, func(c *Cursor) int64 { return int64(c.I8()) }, -128},
		{"I16LE min", []byte{0x00, 0x80}, func(c *Cursor) int64 { return int64(c.I16LE()) }, -32768},
		{"I16BE min", []byte{0x80, 0x00}, func(c *Cursor) int64 { return int64(c.I16BE()) }, -32768},
		{"I32LE", []byte{0xFE, 0xFF, 0xFF, 0xFF}, func(c *Cursor) int64 { return int64(c.I32LE()) }, -2},
		{"I32BE", []byte{0xFF, 0xFF, 0xFF, 0xFE}, func(c *Cursor) int64 { return int64(c.I32BE()) }, -2},
		{"I64LE", bytes.Repeat([]byte{0xFF}, 8), func(c *Cursor) int64 { return c.I64LE() }, -1},
		{"I64BE min", append([]byte{0x80}, bytes.Repeat([]byte{0x00}, 7)...), func(c *Cursor) int64 { return c.I64BE() }, math.MinInt64},
		{"I16 dynamic LE", []byte{0xFE, 0xFF}, func(c *Cursor) int64 { return int64(c.I16(binary.LittleEndian)) }, -2},
		{"I16 dynamic BE", []byte{0xFF, 0xFE}, func(c *Cursor) int64 { return int64(c.I16(binary.BigEndian)) }, -2},
		{"I32 dynamic LE", []byte{0xFE, 0xFF, 0xFF, 0xFF}, func(c *Cursor) int64 { return int64(c.I32(binary.LittleEndian)) }, -2},
		{"I32 dynamic BE", []byte{0xFF, 0xFF, 0xFF, 0xFE}, func(c *Cursor) int64 { return int64(c.I32(binary.BigEndian)) }, -2},
		{"I64 dynamic LE", bytes.Repeat([]byte{0xFF}, 8), func(c *Cursor) int64 { return c.I64(binary.LittleEndian) }, -1},
		{"I64 dynamic BE", bytes.Repeat([]byte{0xFF}, 8), func(c *Cursor) int64 { return c.I64(binary.BigEndian) }, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCursor(tt.in)
			if got := tt.read(c); got != tt.want {
				t.Fatalf("read %q = %d, want %d", tt.in, got, tt.want)
			}
			if c.BytesLeft() != 0 {
				t.Fatalf("BytesLeft() = %d, want 0", c.BytesLeft())
			}
		})
	}
}

func TestCursorU64Order(t *testing.T) {
	le := []byte{0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11}

	if got := NewCursor(le).U64(binary.LittleEndian); got != 0x1122334455667788 {
		t.Fatalf("U64(LittleEndian) = %#x, want 0x1122334455667788", got)
	}
	if got := NewCursor(le).U64(binary.BigEndian); got != 0x8877665544332211 {
		t.Fatalf("U64(BigEndian) = %#x, want 0x8877665544332211", got)
	}
}

func TestCursor24Bit(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		read func(c *Cursor) int64
		want int64
	}{
		{"U24LE", []byte{0x01, 0x02, 0x03}, func(c *Cursor) int64 { return int64(c.U24LE()) }, 0x030201},
		{"U24BE", []byte{0x01, 0x02, 0x03}, func(c *Cursor) int64 { return int64(c.U24BE()) }, 0x010203},
		{"I24LE -1", []byte{0xFF, 0xFF, 0xFF}, func(c *Cursor) int64 { return int64(c.I24LE()) }, -1},
		{"I24BE -1", []byte{0xFF, 0xFF, 0xFF}, func(c *Cursor) int64 { return int64(c.I24BE()) }, -1},
		{"I24LE min", []byte{0x00, 0x00, 0x80}, func(c *Cursor) int64 { return int64(c.I24LE()) }, -1 << 23},
		{"I24BE min", []byte{0x80, 0x00, 0x00}, func(c *Cursor) int64 { return int64(c.I24BE()) }, -1 << 23},
		{"I24LE max", []byte{0xFF, 0xFF, 0x7F}, func(c *Cursor) int64 { return int64(c.I24LE()) }, 1<<23 - 1},
		{"I24BE max", []byte{0x7F, 0xFF, 0xFF}, func(c *Cursor) int64 { return int64(c.I24BE()) }, 1<<23 - 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCursor(tt.in)
			if got := tt.read(c); got != tt.want {
				t.Fatalf("read %q = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestCursor24BitFastPath(t *testing.T) {
	buf := []byte{0x01, 0x02, 0x03, 0xFF}

	if got := NewCursor(buf).U24LE(); got != 0x030201 {
		t.Fatalf("U24LE() = %#x, want 0x030201", got)
	}
	if got := NewCursor(buf).U24BE(); got != 0x010203 {
		t.Fatalf("U24BE() = %#x, want 0x010203", got)
	}

	c := NewCursor(buf)
	c.U24LE()
	if got := c.Pos(); got != 3 {
		t.Fatalf("Pos() = %d, want 3", got)
	}
}

func TestCursorFloats(t *testing.T) {
	le32 := func(f float32) []byte {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, math.Float32bits(f))
		return b
	}
	be32 := func(f float32) []byte {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, math.Float32bits(f))
		return b
	}
	le64 := func(f float64) []byte {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, math.Float64bits(f))
		return b
	}
	be64 := func(f float64) []byte {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, math.Float64bits(f))
		return b
	}

	if got := NewCursor(le32(3.5)).F32LE(); got != 3.5 {
		t.Errorf("F32LE() = %v, want 3.5", got)
	}
	if got := NewCursor(be32(3.5)).F32BE(); got != 3.5 {
		t.Errorf("F32BE() = %v, want 3.5", got)
	}
	if got := NewCursor(le32(3.5)).F32(binary.LittleEndian); got != 3.5 {
		t.Errorf("F32(LittleEndian) = %v, want 3.5", got)
	}
	if got := NewCursor(le64(-2.5)).F64LE(); got != -2.5 {
		t.Errorf("F64LE() = %v, want -2.5", got)
	}
	if got := NewCursor(be64(-2.5)).F64BE(); got != -2.5 {
		t.Errorf("F64BE() = %v, want -2.5", got)
	}
	if got := NewCursor(le64(-2.5)).F64(binary.LittleEndian); got != -2.5 {
		t.Errorf("F64(LittleEndian) = %v, want -2.5", got)
	}

	if got := NewCursor(le32(float32(math.NaN()))).F32LE(); !math.IsNaN(float64(got)) {
		t.Errorf("F32LE(NaN) = %v, want NaN", got)
	}
	if got := NewCursor(le64(math.Inf(-1))).F64LE(); !math.IsInf(got, -1) {
		t.Errorf("F64LE(-Inf) = %v, want -Inf", got)
	}
	if got := NewCursor(le32(float32(math.Copysign(0, -1)))).F32LE(); !math.Signbit(float64(got)) {
		t.Errorf("F32LE(-0) = %v, want negative zero", got)
	}
	if got := NewCursor(le32(math.SmallestNonzeroFloat32)).F32LE(); got != math.SmallestNonzeroFloat32 {
		t.Errorf("F32LE(smallest subnormal) = %v, want %v", got, math.SmallestNonzeroFloat32)
	}
}

func TestCursorULEB128(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want uint64
	}{
		{"zero", []byte{0x00}, 0},
		{"one", []byte{0x01}, 1},
		{"127", []byte{0x7F}, 127},
		{"128", []byte{0x80, 0x01}, 128},
		{"300", []byte{0xAC, 0x02}, 300},
		{"624485", []byte{0xE5, 0x8E, 0x26}, 624485},
		{"1<<21", []byte{0x80, 0x80, 0x80, 0x01}, 1 << 21},
		{"max", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01}, math.MaxUint64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCursor(tt.in)
			got := c.ULEB128()
			if got != tt.want {
				t.Fatalf("ULEB128(% x) = %d, want %d", tt.in, got, tt.want)
			}
			if c.BytesLeft() != 0 {
				t.Fatalf("BytesLeft() = %d, want 0", c.BytesLeft())
			}
		})
	}
}

func TestCursorULEB128Errors(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
	}{
		{"truncated empty", nil},
		{"truncated continuation", []byte{0x80}},
		{"truncated mid", []byte{0x80, 0x80}},
		{"truncated long", []byte{0x80, 0x80, 0x80}},
		{"overflow 10th byte", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x02}},
		{"continuation at 63", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x80}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCursor(tt.in)
			mustPanic(t, "bt: ", func() { c.ULEB128() })
			if got := c.Pos(); got != 0 {
				t.Fatalf("Pos() after failed ULEB128 = %d, want 0", got)
			}
		})
	}
}

func TestCursorULEB128RollsBackToStart(t *testing.T) {
	c := NewCursor([]byte{0x2A, 0x80})

	if got := c.U8(); got != 0x2A {
		t.Fatalf("U8() = %d, want 42", got)
	}
	mustPanic(t, "bt: ", func() { c.ULEB128() })
	if got, want := c.Pos(), 1; got != want {
		t.Fatalf("Pos() after failed ULEB128 = %d, want %d", got, want)
	}
}

func TestCursorVarintLengths(t *testing.T) {
	for k := 1; k <= 10; k++ {
		v := uint64(0)
		if k > 1 {
			v = 1 << (7 * (k - 1))
		}
		enc := binary.AppendUvarint(nil, v)
		if len(enc) != k {
			t.Fatalf("Uvarint(%d) took %d bytes, want %d", v, len(enc), k)
		}
		c := NewCursor(enc)
		if got := c.ULEB128(); got != v {
			t.Fatalf("ULEB128(%d-byte) = %d, want %d", k, got, v)
		}
		if c.BytesLeft() != 0 {
			t.Fatalf("ULEB128(%d-byte) left %d bytes", k, c.BytesLeft())
		}
	}

	for k := 1; k <= 9; k++ {
		v := int64(0)
		if k > 1 {
			v = 1 << (7 * (k - 1))
		}
		enc := encodeSLEB(v)
		if len(enc) != k {
			t.Fatalf("SLEB128(%d) took %d bytes, want %d", v, len(enc), k)
		}
		c := NewCursor(enc)
		if got := c.SLEB128(); got != v {
			t.Fatalf("SLEB128(%d-byte) = %d, want %d", k, got, v)
		}
		if c.BytesLeft() != 0 {
			t.Fatalf("SLEB128(%d-byte) left %d bytes", k, c.BytesLeft())
		}
	}

	for _, v := range []int64{
		-(1 << 31), -(1 << 38), -(1 << 45), -(1 << 52), -(1 << 59),
		math.MinInt64, math.MaxInt64,
	} {
		c := NewCursor(encodeSLEB(v))
		if got := c.SLEB128(); got != v {
			t.Fatalf("SLEB128(%d) = %d, want %d", v, got, v)
		}
		if c.BytesLeft() != 0 {
			t.Fatalf("SLEB128(%d) left %d bytes", v, c.BytesLeft())
		}
	}
}

func TestCursorVarintTruncatedPrefixes(t *testing.T) {
	u := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01}
	for i := range u {
		c := NewCursor(u[:i])
		mustPanic(t, "bt: ", func() { c.ULEB128() })
		if got := c.Pos(); got != 0 {
			t.Fatalf("ULEB128 prefix %d moved offset to %d", i, got)
		}
	}

	s := []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x7F}
	for i := range s {
		c := NewCursor(s[:i])
		mustPanic(t, "bt: ", func() { c.SLEB128() })
		if got := c.Pos(); got != 0 {
			t.Fatalf("SLEB128 prefix %d moved offset to %d", i, got)
		}
	}
}

func TestCursorSub(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6}
	c := NewCursor(data)

	sub := c.Sub(3)
	if got := sub.BytesLeft(); got != 3 {
		t.Fatalf("Sub(3).BytesLeft() = %d, want 3", got)
	}
	if got := sub.Pos(); got != 0 {
		t.Fatalf("Sub(3).Pos() = %d, want 0", got)
	}
	if got := c.Pos(); got != 3 {
		t.Fatalf("parent Pos() after Sub(3) = %d, want 3", got)
	}

	if got := sub.U8(); got != 1 {
		t.Fatalf("sub.U8() = %d, want 1", got)
	}
	if got := c.U8(); got != 4 {
		t.Fatalf("parent U8() = %d, want 4 (cursors must be independent)", got)
	}

	mustPanic(t, "bt: need", func() { c.Sub(4) })
	if got := c.Pos(); got != 4 {
		t.Fatalf("parent Pos() after failed Sub = %d, want 4", got)
	}

	empty := c.Sub(0)
	if got := empty.BytesLeft(); got != 0 {
		t.Fatalf("Sub(0).BytesLeft() = %d, want 0", got)
	}
	if got := c.Pos(); got != 4 {
		t.Fatalf("parent Pos() after Sub(0) = %d, want 4", got)
	}
}

func TestCursorSlicesAreCapped(t *testing.T) {
	c := NewCursor([]byte{1, 2, 3, 4, 5, 6})

	if got := cap(c.Bytes(2)); got != 2 {
		t.Fatalf("cap(Bytes(2)) = %d, want 2", got)
	}
	if got := cap(c.Peek(2)); got != 2 {
		t.Fatalf("cap(Peek(2)) = %d, want 2", got)
	}

	sub := c.Sub(2)
	if got := cap(sub.Bytes(2)); got != 2 {
		t.Fatalf("cap(sub.Bytes(2)) = %d, want 2", got)
	}
}

func TestCursorSLEB128(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want int64
	}{
		{"zero", []byte{0x00}, 0},
		{"one", []byte{0x01}, 1},
		{"minus one", []byte{0x7F}, -1},
		{"63", []byte{0x3F}, 63},
		{"minus 64", []byte{0x40}, -64},
		{"64", []byte{0xC0, 0x00}, 64},
		{"minus 65", []byte{0xBF, 0x7F}, -65},
		{"128", []byte{0x80, 0x01}, 128},
		{"129", []byte{0x81, 0x01}, 129},
		{"minus 2", []byte{0x7E}, -2},
		{"minus 8193", []byte{0xFF, 0xBF, 0x7F}, -8193},
		{"8192", []byte{0x80, 0xC0, 0x00}, 8192},
		{"1<<20", []byte{0x80, 0x80, 0xC0, 0x00}, 1 << 20},
		{"-(1<<27)", []byte{0x80, 0x80, 0x80, 0x40}, -(1 << 27)},
		{"max int64", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00}, math.MaxInt64},
		{"min int64", []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x7F}, math.MinInt64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCursor(tt.in)
			got := c.SLEB128()
			if got != tt.want {
				t.Fatalf("SLEB128(% x) = %d, want %d", tt.in, got, tt.want)
			}
			if c.BytesLeft() != 0 {
				t.Fatalf("BytesLeft() = %d, want 0", c.BytesLeft())
			}
		})
	}
}

func TestCursorSLEB128Errors(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
	}{
		{"truncated empty", nil},
		{"truncated continuation", []byte{0x80}},
		{"truncated mid", []byte{0x80, 0x80}},
		{"truncated long", []byte{0x80, 0x80, 0x80}},
		{"overflow 10th byte positive", []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01}},
		{"overflow 10th byte negative", []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x40}},
		{"continuation at 10th byte", []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCursor(tt.in)
			mustPanic(t, "bt: ", func() { c.SLEB128() })
			if got := c.Pos(); got != 0 {
				t.Fatalf("Pos() after failed SLEB128 = %d, want 0", got)
			}
		})
	}
}

func TestCursorSLEB128RollsBackToStart(t *testing.T) {
	c := NewCursor([]byte{0x2A, 0x80})

	if got := c.U8(); got != 0x2A {
		t.Fatalf("U8() = %d, want 42", got)
	}
	mustPanic(t, "bt: ", func() { c.SLEB128() })
	if got, want := c.Pos(), 1; got != want {
		t.Fatalf("Pos() after failed SLEB128 = %d, want %d", got, want)
	}
}

func TestCursorNavigation(t *testing.T) {
	data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	c := NewCursor(data)

	if got := c.Pos(); got != 0 {
		t.Fatalf("Pos() = %d, want 0", got)
	}
	if got := c.BytesLeft(); got != 10 {
		t.Fatalf("BytesLeft() = %d, want 10", got)
	}
	if !c.CanRead(0) || !c.CanRead(10) {
		t.Fatal("CanRead(0) and CanRead(10) must be true")
	}
	if c.CanRead(11) {
		t.Fatal("CanRead(11) must be false")
	}

	c.Skip(3)
	if got := c.Pos(); got != 3 {
		t.Fatalf("Pos() after Skip(3) = %d, want 3", got)
	}

	got := c.Bytes(2)
	if !bytes.Equal(got, data[3:5]) {
		t.Fatalf("Bytes(2) = %v, want %v", got, data[3:5])
	}
	if got := c.Pos(); got != 5 {
		t.Fatalf("Pos() after Bytes(2) = %d, want 5", got)
	}

	peeked := c.Peek(2)
	if !bytes.Equal(peeked, data[5:7]) {
		t.Fatalf("Peek(2) = %v, want %v", peeked, data[5:7])
	}
	if got := c.Pos(); got != 5 {
		t.Fatalf("Pos() after Peek(2) = %d, want 5", got)
	}

	c.Ensure(5)
	mustPanic(t, "bt: need", func() { c.Ensure(6) })

	c.Skip(5)
	if got := c.BytesLeft(); got != 0 {
		t.Fatalf("BytesLeft() = %d, want 0", got)
	}
	if c.CanRead(1) {
		t.Fatal("CanRead(1) at end must be false")
	}
	mustPanic(t, "bt: need", func() { c.Skip(1) })
	if got := c.Pos(); got != 10 {
		t.Fatalf("Pos() after failed Skip = %d, want 10", got)
	}
}

func TestCursorAlign(t *testing.T) {
	c := NewCursor([]byte{1, 2, 3, 4, 5, 6, 7, 8})

	if got := c.Align(4); got != 0 {
		t.Fatalf("Align(4) at offset 0 = %d, want 0", got)
	}
	c.Skip(1)
	if got := c.Align(4); got != 3 {
		t.Fatalf("Align(4) at offset 1 = %d, want 3", got)
	}
	if got := c.Pos(); got != 4 {
		t.Fatalf("Pos() = %d, want 4", got)
	}
	if got := c.Align(1); got != 0 {
		t.Fatalf("Align(1) = %d, want 0", got)
	}
	if got := c.Align(8); got != 4 {
		t.Fatalf("Align(8) at offset 4 = %d, want 4", got)
	}
	if got := c.BytesLeft(); got != 0 {
		t.Fatalf("BytesLeft() = %d, want 0", got)
	}

	mustPanic(t, "bt: need 3 bytes at offset 1, have 1", func() {
		c := NewCursor([]byte{1, 2})
		c.Skip(1)
		c.Align(4)
	})
	mustPanic(t, "bt: bad align size 0", func() { NewCursor(nil).Align(0) })
	mustPanic(t, "bt: bad align size -4", func() { NewCursor(nil).Align(-4) })
}

func TestCursorNegativeSizes(t *testing.T) {
	ops := []struct {
		name string
		fn   func(c *Cursor)
	}{
		{"Bytes", func(c *Cursor) { c.Bytes(-1) }},
		{"Peek", func(c *Cursor) { c.Peek(-1) }},
		{"Skip", func(c *Cursor) { c.Skip(-1) }},
		{"Ensure", func(c *Cursor) { c.Ensure(-1) }},
		{"StrOrRest", func(c *Cursor) { c.StrOrRest(-1) }},
		{"RawStr", func(c *Cursor) { c.RawStr(-1) }},
		{"StrUnsafe", func(c *Cursor) { c.StrUnsafe(-1) }},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			c := NewCursor([]byte{1, 2, 3})
			mustPanic(t, "bt: negative size -1", func() { op.fn(c) })
			if got := c.Pos(); got != 0 {
				t.Fatalf("Pos() = %d, want 0", got)
			}
		})
	}

	if NewCursor([]byte{1}).CanRead(-1) {
		t.Fatal("CanRead(-1) must be false")
	}
}

func TestCursorPanicMessage(t *testing.T) {
	c := NewCursor([]byte{0xAA})
	mustPanic(t, "bt: need 4 bytes at offset 0, have 1", func() { c.U32LE() })

	c.Skip(1)
	mustPanic(t, "bt: need 1 bytes at offset 1, have 0", func() { c.U8() })
}

func TestCursorOffsetUnchangedOnFailedRead(t *testing.T) {
	ops := []struct {
		name string
		data []byte
		fn   func(c *Cursor)
	}{
		{"U8", nil, func(c *Cursor) { c.U8() }},
		{"U16LE", []byte{1}, func(c *Cursor) { c.U16LE() }},
		{"U16BE", []byte{1}, func(c *Cursor) { c.U16BE() }},
		{"U32LE", []byte{1, 2, 3}, func(c *Cursor) { c.U32LE() }},
		{"U32BE", []byte{1, 2, 3}, func(c *Cursor) { c.U32BE() }},
		{"U64LE", bytes.Repeat([]byte{1}, 7), func(c *Cursor) { c.U64LE() }},
		{"U64BE", bytes.Repeat([]byte{1}, 7), func(c *Cursor) { c.U64BE() }},
		{"I16LE", []byte{1}, func(c *Cursor) { c.I16LE() }},
		{"I32BE", []byte{1, 2, 3}, func(c *Cursor) { c.I32BE() }},
		{"I64LE", bytes.Repeat([]byte{1}, 7), func(c *Cursor) { c.I64LE() }},
		{"U24LE", []byte{1, 2}, func(c *Cursor) { c.U24LE() }},
		{"U24BE", []byte{1, 2}, func(c *Cursor) { c.U24BE() }},
		{"I24LE", []byte{1, 2}, func(c *Cursor) { c.I24LE() }},
		{"I24BE", []byte{1, 2}, func(c *Cursor) { c.I24BE() }},
		{"F32LE", []byte{1, 2, 3}, func(c *Cursor) { c.F32LE() }},
		{"F32BE", []byte{1, 2, 3}, func(c *Cursor) { c.F32BE() }},
		{"F64LE", bytes.Repeat([]byte{1}, 7), func(c *Cursor) { c.F64LE() }},
		{"F64BE", bytes.Repeat([]byte{1}, 7), func(c *Cursor) { c.F64BE() }},
		{"ULEB128", []byte{0x80}, func(c *Cursor) { c.ULEB128() }},
		{"SLEB128", []byte{0x80}, func(c *Cursor) { c.SLEB128() }},
		{"StrOrRest", []byte{1, 2}, func(c *Cursor) { c.StrOrRest(3) }},
		{"RawStr", []byte{1, 2}, func(c *Cursor) { c.RawStr(3) }},
		{"StrUnsafe", []byte{1, 2}, func(c *Cursor) { c.StrUnsafe(3) }},
		{"Bytes", []byte{1, 2}, func(c *Cursor) { c.Bytes(3) }},
		{"Peek", []byte{1, 2}, func(c *Cursor) { c.Peek(3) }},
		{"Skip", []byte{1, 2}, func(c *Cursor) { c.Skip(3) }},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			c := NewCursor(op.data)
			mustPanic(t, "bt: need", func() { op.fn(c) })
			if got := c.Pos(); got != 0 {
				t.Fatalf("Pos() after failed read = %d, want 0", got)
			}
			if got := c.BytesLeft(); got != len(op.data) {
				t.Fatalf("BytesLeft() after failed read = %d, want %d", got, len(op.data))
			}
		})
	}
}

func TestCursorBytesAliasesBuffer(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	c := NewCursor(data)

	got := c.Bytes(2)
	data[0] = 0xFF
	if got[0] != 0xFF {
		t.Fatal("Bytes result must alias the underlying buffer")
	}

	got[1] = 0xEE
	if data[1] != 0xEE {
		t.Fatal("mutating Bytes result must affect the underlying buffer")
	}
}

func TestCursorStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		sz   int
		want string
	}{
		{"strOrRest no nul", []byte("abc"), 3, "abc"},
		{"strOrRest nul inside", []byte("ab\x00d"), 4, "ab"},
		{"strOrRest zero size", []byte("abc"), 0, ""},
		{"strOrRest takes window", []byte("abcdef"), 3, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCursor(tt.in)
			if got := c.StrOrRest(tt.sz); got != tt.want {
				t.Fatalf("StrOrRest(%d) = %q, want %q", tt.sz, got, tt.want)
			}
			if got, want := c.BytesLeft(), len(tt.in)-tt.sz; got != want {
				t.Fatalf("BytesLeft() = %d, want %d", got, want)
			}
		})
	}

	c := NewCursor([]byte{'a', 0x00, 'b', 0xFF})
	if got := c.RawStr(4); got != "a\x00b\xff" {
		t.Fatalf("RawStr(4) = %q, want %q", got, "a\x00b\xff")
	}
	if got := NewCursor([]byte{'a', 0x00, 'b'}).RawStr(3); got != "a\x00b" {
		t.Fatalf("RawStr keeps NULs, got %q", got)
	}
	if got := NewCursor([]byte{'a', 0x00, 'b'}).StrUnsafe(3); got != "a\x00b" {
		t.Fatalf("StrUnsafe keeps NULs, got %q", got)
	}
	if got := NewCursor([]byte("abc")).StrUnsafe(0); got != "" {
		t.Fatalf("StrUnsafe(0) = %q, want empty", got)
	}
}

func TestCursorStringAllocations(t *testing.T) {
	buf := []byte("0123456789abcdef")

	rawAllocs := testing.AllocsPerRun(100, func() {
		c := NewCursor(buf)
		sinkStr = c.RawStr(8)
	})
	if rawAllocs < 1 {
		t.Fatalf("RawStr allocations = %v, want at least 1", rawAllocs)
	}

	unsafeAllocs := testing.AllocsPerRun(100, func() {
		c := NewCursor(buf)
		sinkStr = c.StrUnsafe(8)
	})
	if unsafeAllocs > 0 {
		t.Fatalf("StrUnsafe allocations = %v, want 0", unsafeAllocs)
	}
}

func TestNewCursorDoesNotAliasCursorState(t *testing.T) {
	buf := []byte{0x01, 0x02, 0x03, 0x04}

	a := NewCursor(buf)
	b := NewCursor(buf)

	a.U8()
	a.U8()

	if got := b.U8(); got != 0x01 {
		t.Fatalf("second cursor U8() = %#x, want 0x01 (offsets must be independent)", got)
	}
}

func FuzzCursorNeverPanicsWhenEnoughBytes(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte{})
	f.Add([]byte("hello\x00world"))

	f.Fuzz(func(t *testing.T, data []byte) {
		c := NewCursor(data)
		for {
			switch {
			case c.BytesLeft() >= 8:
				c.U64BE()
			case c.BytesLeft() >= 4:
				c.U32LE()
			case c.BytesLeft() >= 2:
				c.U16BE()
			case c.BytesLeft() >= 1:
				c.U8()
			default:
				return
			}
			if p := c.Pos(); p < 0 || p > len(data) {
				t.Fatalf("Pos() = %d, len(data) = %d", p, len(data))
			}
		}
	})
}

func FuzzCursorNavigation(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4}, []byte{0x00, 0x05, 0x0A, 0x0F})

	f.Fuzz(func(t *testing.T, data, script []byte) {
		c := NewCursor(data)
		for _, cmd := range script {
			n := int(cmd >> 2)
			pos := c.Pos()

			switch cmd & 3 {
			case 0:
				if c.CanRead(n) {
					if got := len(c.Bytes(n)); got != n {
						t.Fatalf("Bytes(%d) returned %d bytes", n, got)
					}
					if c.Pos() != pos+n {
						t.Fatalf("Bytes(%d) advanced to %d, want %d", n, c.Pos(), pos+n)
					}
				} else {
					mustPanic(t, "bt: need", func() { c.Bytes(n) })
				}
			case 1:
				if c.CanRead(n) {
					if got := len(c.Peek(n)); got != n {
						t.Fatalf("Peek(%d) returned %d bytes", n, got)
					}
				} else {
					mustPanic(t, "bt: need", func() { c.Peek(n) })
				}
				if c.Pos() != pos {
					t.Fatalf("Peek(%d) moved cursor to %d, want %d", n, c.Pos(), pos)
				}
			case 2:
				if c.CanRead(n) {
					c.Skip(n)
					if c.Pos() != pos+n {
						t.Fatalf("Skip(%d) advanced to %d, want %d", n, c.Pos(), pos+n)
					}
				} else {
					mustPanic(t, "bt: need", func() { c.Skip(n) })
				}
			case 3:
				if c.CanRead(n) {
					c.Ensure(n)
				} else {
					mustPanic(t, "bt: need", func() { c.Ensure(n) })
				}
			}

			if p := c.Pos(); p < pos || p > len(data) {
				t.Fatalf("Pos() = %d, want within [%d, %d]", p, pos, len(data))
			}
		}
	})
}

func FuzzULEB128RoundTrip(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(300))
	f.Add(uint64(math.MaxUint64))

	f.Fuzz(func(t *testing.T, v uint64) {
		var buf [binary.MaxVarintLen64]byte
		n := binary.PutUvarint(buf[:], v)

		c := NewCursor(buf[:n])
		if got := c.ULEB128(); got != v {
			t.Fatalf("ULEB128 round-trip of %d = %d", v, got)
		}
		if c.BytesLeft() != 0 {
			t.Fatalf("BytesLeft() = %d, want 0", c.BytesLeft())
		}
	})
}

// encodeSLEB is the inverse of SLEB128, used to fuzz round-trips.
func encodeSLEB(v int64) []byte {
	var out []byte
	for {
		b := byte(v) & 0x7F
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

func FuzzSLEB128RoundTrip(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(-1))
	f.Add(int64(math.MinInt64))
	f.Add(int64(math.MaxInt64))

	f.Fuzz(func(t *testing.T, v int64) {
		enc := encodeSLEB(v)

		c := NewCursor(enc)
		got := c.SLEB128()
		if got != v {
			t.Fatalf("SLEB128 round-trip of %d: encoded % x, got %d", v, enc, got)
		}
		if c.BytesLeft() != 0 {
			t.Fatalf("BytesLeft() = %d, want 0 (encoded % x)", c.BytesLeft(), enc)
		}
	})
}

func FuzzCStr(f *testing.F) {
	f.Add([]byte("hello\x00world"))
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0})

	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := CStr(data)
		if err != nil {
			if !errors.Is(err, ErrNoNul) {
				t.Fatalf("unexpected error: %v", err)
			}
			if bytes.IndexByte(data, 0) != -1 {
				t.Fatal("ErrNoNul returned for data containing a null byte")
			}
			return
		}
		if !bytes.HasPrefix(data, []byte(got)) {
			t.Fatalf("CStr(%q) = %q, not a prefix of input", data, got)
		}
		if bytes.IndexByte(data, 0) != len(got) {
			t.Fatalf("CStr(%q) = %q, does not stop at the null byte", data, got)
		}
	})
}
