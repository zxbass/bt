package bt

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestAppendU24(t *testing.T) {
	tests := []struct {
		name string
		fn   func([]byte, uint32) []byte
		v    uint32
		want []byte
	}{
		{"LE zero", AppendU24LE, 0, []byte{0x00, 0x00, 0x00}},
		{"LE", AppendU24LE, 0x123456, []byte{0x56, 0x34, 0x12}},
		{"LE max", AppendU24LE, 0xFFFFFF, []byte{0xFF, 0xFF, 0xFF}},
		{"BE", AppendU24BE, 0x123456, []byte{0x12, 0x34, 0x56}},
		{"BE max", AppendU24BE, 0xFFFFFF, []byte{0xFF, 0xFF, 0xFF}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn([]byte{0xAA}, tt.v)
			want := append([]byte{0xAA}, tt.want...)
			if !bytes.Equal(got, want) {
				t.Fatalf("append = % x, want % x", got, want)
			}
		})
	}

	mustPanic(t, "bt: value 0x1000000 does not fit in 24 bits", func() {
		AppendU24LE(nil, 0x1000000)
	})
	mustPanic(t, "bt: value 0x1000000 does not fit in 24 bits", func() {
		AppendU24BE(nil, 0x1000000)
	})
}

func TestAppendVarints(t *testing.T) {
	unsigned := []uint64{0, 1, 0x7F, 0x80, 300, 1 << 21, 1 << 32, math.MaxUint64}
	for _, v := range unsigned {
		got := AppendULEB128([]byte{0xAA}, v)
		want := append([]byte{0xAA}, binary.AppendUvarint(nil, v)...)
		if !bytes.Equal(got, want) {
			t.Fatalf("AppendULEB128(%d) = % x, want % x", v, got, want)
		}
		if size := ULEB128Size(v); size != len(got)-1 {
			t.Fatalf("ULEB128Size(%d) = %d, want %d", v, size, len(got)-1)
		}
	}

	signed := []int64{
		0, 1, -1, 63, 64, -64, -65, 8192, -8192,
		1 << 20, -(1 << 27), 1 << 40, -(1 << 40),
		math.MaxInt64, math.MinInt64,
	}
	for _, v := range signed {
		enc := AppendSLEB128(nil, v)
		if got := NewCursor(enc).SLEB128(); got != v {
			t.Fatalf("AppendSLEB128(%d) round trip = %d", v, got)
		}
		if size := SLEB128Size(v); size != len(enc) {
			t.Fatalf("SLEB128Size(%d) = %d, want %d", v, size, len(enc))
		}
	}
}

func TestAppendAllocations(t *testing.T) {
	buf := make([]byte, 0, 16)

	allocs := testing.AllocsPerRun(100, func() {
		buf = buf[:0]
		buf = AppendU24LE(buf, 0x123456)
		buf = AppendULEB128(buf, 1<<32)
		buf = AppendSLEB128(buf, -300)
	})
	if allocs > 0 {
		t.Fatalf("append allocations = %v, want 0", allocs)
	}
	sinkBytes = buf
}
