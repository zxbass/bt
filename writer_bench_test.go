package bt

import (
	"bufio"
	"encoding/binary"
	"io"
	"testing"
)

func benchWriter(b *testing.B, size int, write func(*Writer)) {
	b.Helper()
	w := NewWriter()
	w.Grow(1 << 20)
	b.SetBytes(int64(size))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if w.Len() > (1<<20)-64 {
			w.Reset()
		}
		write(w)
	}
}

func BenchmarkWriterU16LE(b *testing.B) {
	benchWriter(b, 2, func(w *Writer) { w.U16LE(uint16(w.Len())) })
}

func BenchmarkWriterU16Order(b *testing.B) {
	benchWriter(b, 2, func(w *Writer) { w.U16(binary.BigEndian, uint16(w.Len())) })
}

func BenchmarkWriterU32BE(b *testing.B) {
	benchWriter(b, 4, func(w *Writer) { w.U32BE(uint32(w.Len())) })
}

func BenchmarkWriterU32Order(b *testing.B) {
	benchWriter(b, 4, func(w *Writer) { w.U32(binary.LittleEndian, uint32(w.Len())) })
}

func BenchmarkWriterU64LE(b *testing.B) {
	benchWriter(b, 8, func(w *Writer) { w.U64LE(uint64(w.Len())) })
}

func BenchmarkWriterU64Order(b *testing.B) {
	benchWriter(b, 8, func(w *Writer) { w.U64(binary.BigEndian, uint64(w.Len())) })
}

func BenchmarkWriterU24LE(b *testing.B) {
	benchWriter(b, 3, func(w *Writer) { w.U24LE(uint32(w.Len()) & 0xFFFFFF) })
}

func BenchmarkWriterULEB128(b *testing.B) {
	benchWriter(b, 2, func(w *Writer) { w.ULEB128(uint64(w.Len())) })
}

func BenchmarkWriterSLEB128(b *testing.B) {
	benchWriter(b, 2, func(w *Writer) { w.SLEB128(int64(w.Len())) })
}

func BenchmarkWriterRawStr(b *testing.B) {
	benchWriter(b, 8, func(w *Writer) { w.RawStr("12345678") })
}

func BenchmarkWriterCStr(b *testing.B) {
	benchWriter(b, 9, func(w *Writer) { w.CStr("12345678") })
}

func BenchmarkWriterSection(b *testing.B) {
	benchWriter(b, 9, func(w *Writer) {
		w.LenU8(func(w *Writer) { w.RawStr("payload!") })
	})
}

func BenchmarkStdlibAppendU16LE(b *testing.B) {
	buf := make([]byte, 0, 1<<20)
	b.SetBytes(2)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if len(buf) > (1<<20)-64 {
			buf = buf[:0]
		}
		buf = binary.LittleEndian.AppendUint16(buf, uint16(len(buf)))
	}
	sinkBytes = buf
}

func BenchmarkStdlibAppendU32BE(b *testing.B) {
	buf := make([]byte, 0, 1<<20)
	b.SetBytes(4)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if len(buf) > (1<<20)-64 {
			buf = buf[:0]
		}
		buf = binary.BigEndian.AppendUint32(buf, uint32(len(buf)))
	}
	sinkBytes = buf
}

func BenchmarkStdlibAppendU64LE(b *testing.B) {
	buf := make([]byte, 0, 1<<20)
	b.SetBytes(8)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if len(buf) > (1<<20)-64 {
			buf = buf[:0]
		}
		buf = binary.LittleEndian.AppendUint64(buf, uint64(len(buf)))
	}
	sinkBytes = buf
}

func BenchmarkStdlibAppendUvarint(b *testing.B) {
	buf := make([]byte, 0, 1<<20)
	b.SetBytes(2)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if len(buf) > (1<<20)-64 {
			buf = buf[:0]
		}
		buf = binary.AppendUvarint(buf, uint64(len(buf)))
	}
	sinkBytes = buf
}

func BenchmarkAppendULEB128(b *testing.B) {
	buf := make([]byte, 0, 1<<20)
	b.SetBytes(2)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if len(buf) > (1<<20)-64 {
			buf = buf[:0]
		}
		buf = AppendULEB128(buf, uint64(len(buf)))
	}
	sinkBytes = buf
}

func BenchmarkAppendU24LE(b *testing.B) {
	buf := make([]byte, 0, 1<<20)
	b.SetBytes(3)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if len(buf) > (1<<20)-64 {
			buf = buf[:0]
		}
		buf = AppendU24LE(buf, uint32(len(buf))&0xFFFFFF)
	}
	sinkBytes = buf
}

func BenchmarkWriteRecords(b *testing.B) {
	const (
		records = 1024
		nameLen = 8
	)
	w := NewWriter()
	w.Grow(64 << 10)
	b.SetBytes(records * (1 + 2 + 4 + 4 + 1 + nameLen + 2))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w.Reset()
		for j := 0; j < records; j++ {
			w.U8(byte(j))
			w.U16LE(uint16(j))
			w.U32LE(uint32(j))
			w.F32LE(float32(j))
			w.U8(nameLen)
			w.RawStr("aaaaaaaa")
			w.U16LE(uint16(j))
		}
	}
	b.ReportMetric(float64(records)/float64(b.N), "records/op")
}

func BenchmarkWriteFixedRecords(b *testing.B) {
	const (
		recSize = 16
		records = 4096
	)
	b.SetBytes(recSize * records)
	b.ReportAllocs()

	b.Run("writer", func(b *testing.B) {
		w := NewWriter()
		w.Grow(recSize * records)
		for i := 0; i < b.N; i++ {
			w.Reset()
			for j := 0; j < records; j++ {
				writeFixedRecord(w, j)
			}
		}
		sinkBytes = w.Bytes()
	})

	b.Run("streamwriter", func(b *testing.B) {
		sw := NewStreamWriter(io.Discard)
		for i := 0; i < b.N; i++ {
			for j := 0; j < records; j++ {
				sw.U8(byte(j))
				sw.U16LE(uint16(j))
				sw.U32LE(uint32(j))
				sw.U64LE(uint64(j))
				sw.U8(0)
			}
			if err := sw.Flush(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("streamwriter-grow", func(b *testing.B) {
		sw := NewStreamWriter(io.Discard)
		sw.Grow(recSize * records)
		for i := 0; i < b.N; i++ {
			for j := 0; j < records; j++ {
				sw.U8(byte(j))
				sw.U16LE(uint16(j))
				sw.U32LE(uint32(j))
				sw.U64LE(uint64(j))
				sw.U8(0)
			}
			if err := sw.Flush(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("bufio", func(b *testing.B) {
		bw := bufio.NewWriterSize(io.Discard, 64<<10)
		var rec [recSize]byte
		for i := 0; i < b.N; i++ {
			for j := 0; j < records; j++ {
				rec[0] = byte(j)
				binary.LittleEndian.PutUint16(rec[1:], uint16(j))
				binary.LittleEndian.PutUint32(rec[3:], uint32(j))
				binary.LittleEndian.PutUint64(rec[7:], uint64(j))
				rec[15] = 0
				if _, err := bw.Write(rec[:]); err != nil {
					b.Fatal(err)
				}
			}
			if err := bw.Flush(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func writeFixedRecord(w *Writer, j int) {
	w.U8(byte(j))
	w.U16LE(uint16(j))
	w.U32LE(uint32(j))
	w.U64LE(uint64(j))
	w.U8(0)
}
