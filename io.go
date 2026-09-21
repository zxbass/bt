package bt

import "io"

// NewCursorFromReader reads r until EOF and returns a cursor over the data.
// The whole stream is buffered in memory; for incremental parsing use Stream.
func NewCursorFromReader(r io.Reader) (*Cursor, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return NewCursor(data), nil
}

// Read implements io.Reader: it copies up to len(p) unread bytes into p and
// advances the cursor. At the end of the buffer it returns 0, io.EOF.
func (c *Cursor) Read(p []byte) (int, error) {
	if c.BytesLeft() == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.b[c.off:])
	c.off += n
	return n, nil
}

// ReadByte implements io.ByteReader. At the end of the buffer it returns
// 0, io.EOF instead of panicking.
func (c *Cursor) ReadByte() (byte, error) {
	if c.BytesLeft() == 0 {
		return 0, io.EOF
	}
	v := c.b[c.off]
	c.off++
	return v, nil
}
