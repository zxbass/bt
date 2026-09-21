package bt

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

var (
	_ io.Reader     = (*Cursor)(nil)
	_ io.ByteReader = (*Cursor)(nil)
)

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestNewCursorFromReader(t *testing.T) {
	c, err := NewCursorFromReader(strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("NewCursorFromReader: %v", err)
	}
	if got := string(c.Bytes(5)); got != "hello" {
		t.Fatalf("Bytes(5) = %q, want %q", got, "hello")
	}
	if got := c.BytesLeft(); got != 0 {
		t.Fatalf("BytesLeft() = %d, want 0", got)
	}

	wantErr := errors.New("boom")
	if _, err := NewCursorFromReader(errReader{err: wantErr}); !errors.Is(err, wantErr) {
		t.Fatalf("NewCursorFromReader error = %v, want %v", err, wantErr)
	}
}

func TestCursorRead(t *testing.T) {
	c := NewCursor([]byte{1, 2, 3, 4})

	p := make([]byte, 2)
	if n, err := c.Read(p); n != 2 || err != nil {
		t.Fatalf("Read() = %d, %v, want 2, nil", n, err)
	}
	if !bytes.Equal(p, []byte{1, 2}) {
		t.Fatalf("Read() = %v, want [1 2]", p)
	}

	if n, err := c.Read(p); n != 2 || err != nil {
		t.Fatalf("Read() = %d, %v, want 2, nil", n, err)
	}
	if !bytes.Equal(p, []byte{3, 4}) {
		t.Fatalf("Read() = %v, want [3 4]", p)
	}

	if n, err := c.Read(p); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("Read() at end = %d, %v, want 0, io.EOF", n, err)
	}

	var dst bytes.Buffer
	if _, err := io.Copy(&dst, NewCursor([]byte("abc"))); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	if got := dst.String(); got != "abc" {
		t.Fatalf("io.Copy = %q, want %q", got, "abc")
	}
}

func TestCursorReadByte(t *testing.T) {
	c := NewCursor([]byte{7, 8})

	if b, err := c.ReadByte(); b != 7 || err != nil {
		t.Fatalf("ReadByte() = %d, %v, want 7, nil", b, err)
	}
	if b, err := c.ReadByte(); b != 8 || err != nil {
		t.Fatalf("ReadByte() = %d, %v, want 8, nil", b, err)
	}
	if b, err := c.ReadByte(); b != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadByte() at end = %d, %v, want 0, io.EOF", b, err)
	}
}
