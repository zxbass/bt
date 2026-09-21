package bt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"testing/iotest"
)

var (
	sinkU8    byte
	sinkU16   uint16
	sinkU32   uint32
	sinkU64   uint64
	sinkI8    int8
	sinkF32   float32
	sinkF64   float64
	sinkStr   string
	sinkBytes []byte
	sinkErr   error
)

func benchBuf() []byte {
	buf := make([]byte, 64<<10)
	for i := range buf {
		buf[i] = byte(i)
	}
	return buf
}

func benchCursor(b *testing.B, size int, read func(*Cursor)) {
	b.Helper()
	buf := benchBuf()
	b.SetBytes(int64(size))
	b.ReportAllocs()

	c := NewCursor(buf)
	for i := 0; i < b.N; i++ {
		if c.BytesLeft() < size {
			c = NewCursor(buf)
		}
		read(c)
	}
}

func BenchmarkU8(b *testing.B) {
	benchCursor(b, 1, func(c *Cursor) { sinkU8 = c.U8() })
}

func BenchmarkU16LE(b *testing.B) {
	benchCursor(b, 2, func(c *Cursor) { sinkU16 = c.U16LE() })
}

func BenchmarkU16BE(b *testing.B) {
	benchCursor(b, 2, func(c *Cursor) { sinkU16 = c.U16BE() })
}

func BenchmarkU16Order(b *testing.B) {
	benchCursor(b, 2, func(c *Cursor) { sinkU16 = c.U16(binary.LittleEndian) })
}

func BenchmarkU32LE(b *testing.B) {
	benchCursor(b, 4, func(c *Cursor) { sinkU32 = c.U32LE() })
}

func BenchmarkU32BE(b *testing.B) {
	benchCursor(b, 4, func(c *Cursor) { sinkU32 = c.U32BE() })
}

func BenchmarkU32Order(b *testing.B) {
	benchCursor(b, 4, func(c *Cursor) { sinkU32 = c.U32(binary.LittleEndian) })
}

func BenchmarkU64LE(b *testing.B) {
	benchCursor(b, 8, func(c *Cursor) { sinkU64 = c.U64LE() })
}

func BenchmarkU64BE(b *testing.B) {
	benchCursor(b, 8, func(c *Cursor) { sinkU64 = c.U64BE() })
}

func BenchmarkU64Order(b *testing.B) {
	benchCursor(b, 8, func(c *Cursor) { sinkU64 = c.U64(binary.LittleEndian) })
}

func BenchmarkI8(b *testing.B) {
	benchCursor(b, 1, func(c *Cursor) { sinkI8 = c.I8() })
}

func BenchmarkI16LE(b *testing.B) {
	benchCursor(b, 2, func(c *Cursor) { sinkU16 = uint16(c.I16LE()) })
}

func BenchmarkU24LE(b *testing.B) {
	benchCursor(b, 3, func(c *Cursor) { sinkU32 = c.U24LE() })
}

func BenchmarkI24LE(b *testing.B) {
	benchCursor(b, 3, func(c *Cursor) { sinkU32 = uint32(c.I24LE()) })
}

func BenchmarkF32LE(b *testing.B) {
	benchCursor(b, 4, func(c *Cursor) { sinkF32 = c.F32LE() })
}

func BenchmarkF64LE(b *testing.B) {
	benchCursor(b, 8, func(c *Cursor) { sinkF64 = c.F64LE() })
}

func BenchmarkNavigation(b *testing.B) {
	b.Run("Bytes4", func(b *testing.B) {
		benchCursor(b, 4, func(c *Cursor) { sinkBytes = c.Bytes(4) })
	})
	b.Run("Peek4", func(b *testing.B) {
		benchCursor(b, 4, func(c *Cursor) { sinkBytes = c.Peek(4); c.Skip(4) })
	})
	b.Run("Skip4", func(b *testing.B) {
		benchCursor(b, 4, func(c *Cursor) { c.Skip(4) })
	})
	b.Run("Ensure4", func(b *testing.B) {
		benchCursor(b, 4, func(c *Cursor) { c.Ensure(4); c.Skip(4) })
	})
	b.Run("Pos", func(b *testing.B) {
		c := NewCursor(benchBuf())
		for i := 0; i < b.N; i++ {
			sinkU64 = uint64(c.Pos())
		}
	})
}

func BenchmarkULEB128(b *testing.B) {
	b.Run("1byte", func(b *testing.B) {
		buf := bytes.Repeat([]byte{0x7F}, 64<<10)
		b.ReportAllocs()

		c := NewCursor(buf)
		for i := 0; i < b.N; i++ {
			if c.BytesLeft() == 0 {
				c = NewCursor(buf)
			}
			sinkU64 = c.ULEB128()
		}
	})

	b.Run("2byte", func(b *testing.B) {
		buf := bytes.Repeat([]byte{0x80, 0x01}, 32<<10)
		b.SetBytes(2)
		b.ReportAllocs()

		c := NewCursor(buf)
		for i := 0; i < b.N; i++ {
			if c.BytesLeft() < 2 {
				c = NewCursor(buf)
			}
			sinkU64 = c.ULEB128()
		}
	})

	b.Run("10byte", func(b *testing.B) {
		unit := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01}
		buf := bytes.Repeat(unit, (64<<10)/len(unit))
		b.SetBytes(int64(len(unit)))
		b.ReportAllocs()

		c := NewCursor(buf)
		for i := 0; i < b.N; i++ {
			if c.BytesLeft() < len(unit) {
				c = NewCursor(buf)
			}
			sinkU64 = c.ULEB128()
		}
	})
}

func BenchmarkSLEB128(b *testing.B) {
	b.Run("1byte", func(b *testing.B) {
		buf := bytes.Repeat([]byte{0x3F}, 64<<10)
		b.ReportAllocs()

		c := NewCursor(buf)
		for i := 0; i < b.N; i++ {
			if c.BytesLeft() == 0 {
				c = NewCursor(buf)
			}
			sinkU64 = uint64(c.SLEB128())
		}
	})

	b.Run("2byte", func(b *testing.B) {
		buf := bytes.Repeat([]byte{0xC0, 0x00}, 32<<10)
		b.SetBytes(2)
		b.ReportAllocs()

		c := NewCursor(buf)
		for i := 0; i < b.N; i++ {
			if c.BytesLeft() < 2 {
				c = NewCursor(buf)
			}
			sinkU64 = uint64(c.SLEB128())
		}
	})
}

func BenchmarkStrings(b *testing.B) {
	buf := []byte("abcd0123456789abcdefghijklmnopqrstuvwxyz\x00")

	run := func(b *testing.B, f func(c *Cursor) string) {
		b.Helper()
		b.ReportAllocs()
		c := NewCursor(buf)
		for i := 0; i < b.N; i++ {
			if c.BytesLeft() < 4 {
				c = NewCursor(buf)
			}
			sinkStr = f(c)
		}
	}

	b.Run("RawStr4", func(b *testing.B) { run(b, func(c *Cursor) string { return c.RawStr(4) }) })
	b.Run("StrUnsafe4", func(b *testing.B) { run(b, func(c *Cursor) string { return c.StrUnsafe(4) }) })
	b.Run("StrOrRest4", func(b *testing.B) { run(b, func(c *Cursor) string { return c.StrOrRest(4) }) })
}

func BenchmarkCStr(b *testing.B) {
	cases := []struct {
		name string
		buf  []byte
	}{
		{"short", []byte("hello\x00")},
		{"nul at end", append(bytes.Repeat([]byte{'a'}, 63), 0x00)},
		{"no nul", bytes.Repeat([]byte{'a'}, 64)},
	}
	for _, tc := range cases {
		b.Run("CStr/"+tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.buf)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sinkStr, sinkErr = CStr(tc.buf)
			}
		})
		b.Run("CStrOrRest/"+tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.buf)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sinkStr = CStrOrRest(tc.buf)
			}
		})
	}
}

func buildRecordData(records int) []byte {
	var out []byte
	for i := 0; i < records; i++ {
		nameLen := i % 8
		out = append(out,
			byte(i),             // tag u8
			byte(i), byte(i>>8), // id u16le
			byte(i), byte(i>>8), byte(i>>16), byte(i>>24), // timestamp u32le
			0, 0, 0, 0, // value f32le
			byte(nameLen), // name length u8
		)
		for j := 0; j < nameLen; j++ {
			out = append(out, 'a'+byte(j))
		}
		out = append(out, byte(i), byte(i>>8)) // checksum u16le
	}
	return out
}

// BenchmarkParseRecords parses a stream of records
// (u8 tag + u16le id + u32le ts + f32le value + u8 len + name + u16le crc)
// end to end.
func BenchmarkParseRecords(b *testing.B) {
	data := buildRecordData(1024)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	records := 0
	for i := 0; i < b.N; i++ {
		c := NewCursor(data)
		for c.BytesLeft() > 0 {
			c.U8()
			c.U16LE()
			c.U32LE()
			c.F32LE()
			n := int(c.U8())
			sinkStr = c.RawStr(n)
			c.U16LE()
			records++
		}
	}
	b.ReportMetric(float64(records)/float64(b.N), "records/op")
}

func BenchmarkRecordsIter(b *testing.B) {
	data := benchBuf()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c := NewCursor(data)
		for rec := range c.Records(16) {
			sinkU8 = rec.U8()
		}
	}
}

func BenchmarkSubLoop(b *testing.B) {
	data := benchBuf()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c := NewCursor(data)
		for c.CanRead(16) {
			sinkU8 = c.Sub(16).U8()
		}
	}
}

func BenchmarkChunksIter(b *testing.B) {
	data := benchBuf()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c := NewCursor(data)
		for chunk := range c.Chunks(16) {
			sinkBytes = chunk
		}
	}
}

func benchStream(b *testing.B, total int, newReader func() io.Reader) {
	const recSize = 16
	b.SetBytes(int64(total))
	b.ReportAllocs()

	records := 0
	for i := 0; i < b.N; i++ {
		st := NewStream(newReader())
		for {
			if err := st.Fill(recSize); err != nil {
				if errors.Is(err, io.ErrUnexpectedEOF) && st.Buffered() == 0 {
					break
				}
				b.Fatal(err)
			}
			c := st.Cursor()
			c.U8()
			c.U16LE()
			c.U32LE()
			c.U64LE()
			c.U8()
			st.Advance(recSize)
			records++
		}
	}
	b.ReportMetric(float64(records)/float64(b.N), "records/op")
}

func BenchmarkStreamFixedRecords(b *testing.B) {
	data := benchBuf()

	b.Run("bytes.Reader", func(b *testing.B) {
		benchStream(b, len(data), func() io.Reader { return bytes.NewReader(data) })
	})
	b.Run("chunk7", func(b *testing.B) {
		benchStream(b, len(data), func() io.Reader { return &chunkReader{data: data, chunk: 7} })
	})
	b.Run("onebyte", func(b *testing.B) {
		benchStream(b, len(data), func() io.Reader { return iotest.OneByteReader(bytes.NewReader(data)) })
	})
}

func BenchmarkStdlibBinaryRead(b *testing.B) {
	data := benchBuf()
	cases := []struct {
		name string
		size int
		v    any
	}{
		{"U16", 2, new(uint16)},
		{"U32", 4, new(uint32)},
		{"U64", 8, new(uint64)},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			r := bytes.NewReader(data)
			b.SetBytes(int64(tc.size))
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				if r.Len() < tc.size {
					r.Reset(data)
				}
				if err := binary.Read(r, binary.LittleEndian, tc.v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkStdlibBinary(b *testing.B) {
	buf := benchBuf()
	cases := []struct {
		name string
		size int
		read func([]byte)
	}{
		{"U16LE", 2, func(p []byte) { sinkU16 = binary.LittleEndian.Uint16(p) }},
		{"U32LE", 4, func(p []byte) { sinkU32 = binary.LittleEndian.Uint32(p) }},
		{"U64LE", 8, func(p []byte) { sinkU64 = binary.LittleEndian.Uint64(p) }},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(tc.size))
			b.ReportAllocs()
			off := 0
			for i := 0; i < b.N; i++ {
				if off > len(buf)-tc.size {
					off = 0
				}
				tc.read(buf[off:])
				off += tc.size
			}
		})
	}
}
