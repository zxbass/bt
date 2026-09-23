package bt

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"
)

type chunkReader struct {
	data  []byte
	pos   int
	chunk int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := len(p)
	if r.chunk > 0 && n > r.chunk {
		n = r.chunk
	}
	if n > len(r.data)-r.pos {
		n = len(r.data) - r.pos
	}
	copy(p, r.data[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}

type errAfterReader struct {
	data []byte
	pos  int
	err  error
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

type noProgressReader struct{}

func (noProgressReader) Read([]byte) (int, error) { return 0, nil }

type eofWithDataReader struct{ data []byte }

func (r *eofWithDataReader) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, io.EOF
	}
	return n, nil
}

func TestStreamRecords(t *testing.T) {
	const recSize = 4
	data := []byte{0, 1, 2, 3, 10, 11, 12, 13, 20, 21, 22, 23}

	st := NewStream(bytes.NewReader(data))
	var got []byte
	for {
		if err := st.Fill(recSize); err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) && st.Buffered() == 0 {
				break
			}
			t.Fatalf("Fill(%d): %v", recSize, err)
		}

		c := st.Cursor()
		got = append(got, c.U8(), c.U8(), c.U8(), c.U8())
		st.Advance(recSize)
	}

	if !bytes.Equal(got, data) {
		t.Fatalf("parsed %v, want %v", got, data)
	}
	if !errors.Is(st.Err(), io.EOF) {
		t.Fatalf("Err() = %v, want io.EOF", st.Err())
	}
}

func TestStreamOneByteReader(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6}
	st := NewStream(iotest.OneByteReader(bytes.NewReader(data)))

	for i := 0; i < len(data); i++ {
		if err := st.Fill(1); err != nil {
			t.Fatalf("Fill(1) at %d: %v", i, err)
		}
		if got := st.Cursor().U8(); got != data[i] {
			t.Fatalf("byte %d = %d, want %d", i, got, data[i])
		}
		st.Advance(1)
	}

	if err := st.Fill(1); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Fill(1) at end = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestStreamPartialFill(t *testing.T) {
	st := NewStream(bytes.NewReader([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}))

	if err := st.Fill(4); err != nil {
		t.Fatalf("Fill(4): %v", err)
	}
	c := st.Cursor()
	if got := c.U16LE(); got != 0x0201 {
		t.Fatalf("U16LE() = %#x, want 0x0201", got)
	}
	st.Advance(2)

	if err := st.Fill(100); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Fill(100) = %v, want io.ErrUnexpectedEOF", err)
	}
	if got := st.Buffered(); got != 8 {
		t.Fatalf("Buffered() = %d, want 8", got)
	}
	if !errors.Is(st.Err(), io.EOF) {
		t.Fatalf("Err() = %v, want io.EOF", st.Err())
	}

	if err := st.Fill(8); err != nil {
		t.Fatalf("Fill(8) with buffered data after EOF: %v", err)
	}
}

func TestStreamReadError(t *testing.T) {
	wantErr := errors.New("boom")
	st := NewStream(&errAfterReader{data: []byte{1, 2, 3}, err: wantErr})

	if err := st.Fill(10); !errors.Is(err, wantErr) {
		t.Fatalf("Fill(10) = %v, want %v", err, wantErr)
	}
	if !errors.Is(st.Err(), wantErr) {
		t.Fatalf("Err() = %v, want %v", st.Err(), wantErr)
	}
	if err := st.Fill(4); !errors.Is(err, wantErr) {
		t.Fatalf("second Fill(4) = %v, want %v", err, wantErr)
	}

	if err := st.Fill(3); err != nil {
		t.Fatalf("Fill(3) from buffered data = %v, want nil", err)
	}
	if got := string(st.Cursor().Bytes(3)); got != "\x01\x02\x03" {
		t.Fatalf("buffered data = %q", got)
	}
}

func TestStreamEmpty(t *testing.T) {
	st := NewStream(bytes.NewReader(nil))

	if err := st.Fill(1); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Fill(1) = %v, want io.ErrUnexpectedEOF", err)
	}
	if !errors.Is(st.Err(), io.EOF) {
		t.Fatalf("Err() = %v, want io.EOF", st.Err())
	}
	if got := st.Buffered(); got != 0 {
		t.Fatalf("Buffered() = %d, want 0", got)
	}
	if err := st.Fill(0); err != nil {
		t.Fatalf("Fill(0) = %v, want nil", err)
	}
}

func TestStreamEOFWithData(t *testing.T) {
	st := NewStream(&eofWithDataReader{data: []byte{1, 2, 3, 4}})

	if err := st.Fill(4); err != nil {
		t.Fatalf("Fill(4) = %v, want nil", err)
	}
	if got := st.Buffered(); got != 4 {
		t.Fatalf("Buffered() = %d, want 4", got)
	}
	if !errors.Is(st.Err(), io.EOF) {
		t.Fatalf("Err() = %v, want io.EOF", st.Err())
	}

	st.Advance(3)
	if err := st.Fill(2); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Fill(2) after EOF = %v, want io.ErrUnexpectedEOF", err)
	}
	if err := st.Fill(1); err != nil {
		t.Fatalf("Fill(1) from buffered data after EOF = %v, want nil", err)
	}
}

func TestStreamNegativeFillPanics(t *testing.T) {
	st := NewStream(bytes.NewReader(nil))
	mustPanic(t, "bt: negative size -1", func() { st.Fill(-1) })
}

func TestStreamAdvancePanics(t *testing.T) {
	st := NewStream(bytes.NewReader([]byte{1, 2, 3}))
	if err := st.Fill(3); err != nil {
		t.Fatalf("Fill(3): %v", err)
	}

	mustPanic(t, "bt: advance 4 bytes", func() { st.Advance(4) })
	mustPanic(t, "bt: negative size -1", func() { st.Advance(-1) })
	if got := st.Buffered(); got != 3 {
		t.Fatalf("Buffered() after failed Advance = %d, want 3", got)
	}

	st.Advance(3)
	if got := st.Buffered(); got != 0 {
		t.Fatalf("Buffered() after Advance(3) = %d, want 0", got)
	}
}

func TestStreamMaxBuffer(t *testing.T) {
	st := NewStream(bytes.NewReader(make([]byte, 100)), WithMaxBuffer(8))

	if err := st.Fill(8); err != nil {
		t.Fatalf("Fill(8): %v", err)
	}
	if err := st.Fill(9); !errors.Is(err, ErrBufferLimit) {
		t.Fatalf("Fill(9) = %v, want ErrBufferLimit", err)
	}
	if err := st.Fill(8); err != nil {
		t.Fatalf("Fill(8) from buffered data = %v, want nil", err)
	}
	if err := st.Fill(9); !errors.Is(err, ErrBufferLimit) {
		t.Fatalf("sticky Fill(9) = %v, want ErrBufferLimit", err)
	}
}

func TestStreamNoProgress(t *testing.T) {
	st := NewStream(noProgressReader{})

	if err := st.Fill(1); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("Fill(1) = %v, want io.ErrNoProgress", err)
	}
}

func TestStreamDiscard(t *testing.T) {
	st := NewStream(bytes.NewReader([]byte{1, 2, 3, 4}))

	if err := st.Discard(2); err != nil {
		t.Fatalf("Discard(2): %v", err)
	}
	if err := st.Fill(2); err != nil {
		t.Fatalf("Fill(2): %v", err)
	}
	if got := st.Cursor().U16BE(); got != 0x0304 {
		t.Fatalf("U16BE() = %#x, want 0x0304", got)
	}
	st.Advance(2)

	if err := st.Discard(1); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Discard(1) = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestStreamCompaction(t *testing.T) {
	const (
		recordSize = 100_000
		records    = 20
	)
	data := make([]byte, recordSize*records)
	for i := range data {
		data[i] = byte(i)
	}

	st := NewStream(bytes.NewReader(data))
	for r := 0; r < records; r++ {
		if err := st.Fill(recordSize); err != nil {
			t.Fatalf("record %d: %v", r, err)
		}
		c := st.Cursor()
		chunk := c.Bytes(recordSize)
		if chunk[0] != byte(r*recordSize) || chunk[recordSize-1] != byte(r*recordSize+recordSize-1) {
			t.Fatalf("record %d corrupted", r)
		}
		st.Advance(recordSize)
	}

	if cap(st.buf) > 2*recordSize*2 {
		t.Fatalf("window cap = %d, compaction is not keeping it bounded", cap(st.buf))
	}
}

func FuzzStreamRoundTrip(f *testing.F) {
	f.Add([]byte("hello\x00world"), uint8(1))
	f.Add([]byte{}, uint8(0))
	f.Add(bytes.Repeat([]byte{0xAB}, 100), uint8(7))

	f.Fuzz(func(t *testing.T, data []byte, chunk uint8) {
		readers := []struct {
			name string
			new  func() io.Reader
		}{
			{"bytes.Reader", func() io.Reader { return bytes.NewReader(data) }},
			{"chunked", func() io.Reader { return &chunkReader{data: data, chunk: 1 + int(chunk)%16} }},
			{"onebyte", func() io.Reader { return iotest.OneByteReader(bytes.NewReader(data)) }},
		}

		for _, tc := range readers {
			st := NewStream(tc.new())
			got := make([]byte, 0, len(data))

			for {
				if err := st.Fill(1); err != nil {
					if errors.Is(err, io.ErrUnexpectedEOF) && st.Buffered() == 0 {
						break
					}
					t.Fatalf("%s: Fill(1): %v", tc.name, err)
				}
				got = append(got, st.Cursor().U8())
				st.Advance(1)
			}

			if !bytes.Equal(got, data) {
				t.Fatalf("%s: read %q, want %q", tc.name, got, data)
			}
			if !errors.Is(st.Err(), io.EOF) {
				t.Fatalf("%s: Err() = %v, want io.EOF", tc.name, st.Err())
			}
		}
	})
}
