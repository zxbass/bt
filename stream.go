package bt

import (
	"errors"
	"fmt"
	"io"
)

// ErrBufferLimit is returned by Fill when WithMaxBuffer prevents the window
// from growing enough for the requested size.
var ErrBufferLimit = errors.New("bt: stream buffer limit exceeded")

const (
	streamInitCap       = 512
	streamCompactMin    = 4096
	streamMaxNoProgress = 100
)

// Stream reads from an io.Reader into a growable window and lets callers parse
// it with a regular Cursor.
//
// Fill guarantees that at least n bytes are buffered. Parsing inside that
// window keeps the panic-on-out-of-bounds contract of Cursor, while I/O
// problems are reported as errors from Fill and are sticky: once Fill fails it
// keeps returning the same error, and Err reports the underlying cause
// (io.EOF on a clean end of stream).
//
// Cursors returned by Cursor alias the window and are invalidated by the next
// Fill or Advance call.
//
// Stream is not safe for concurrent use.
type Stream struct {
	r     io.Reader
	buf   []byte
	off   int
	err   error
	max   int
	empty int
}

// StreamOption configures a Stream.
type StreamOption func(*Stream)

// WithMaxBuffer limits the window to n bytes; a Fill that needs more returns
// ErrBufferLimit. Values <= 0 mean no limit.
func WithMaxBuffer(n int) StreamOption {
	return func(s *Stream) { s.max = n }
}

// NewStream returns a stream reading from r.
func NewStream(r io.Reader, opts ...StreamOption) *Stream {
	s := &Stream{r: r}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Buffered returns the number of bytes currently in the window.
func (s *Stream) Buffered() int {
	return len(s.buf) - s.off
}

// Cursor returns a cursor over the buffered unread bytes. The cursor is
// invalidated by the next Fill or Advance call.
func (s *Stream) Cursor() *Cursor {
	return NewCursor(s.buf[s.off:])
}

// Err returns the sticky underlying error, io.EOF included, or nil.
func (s *Stream) Err() error {
	return s.err
}

// Fill buffers at least n bytes. It returns io.ErrUnexpectedEOF if the stream
// ends first, the underlying read error on I/O failure, and ErrBufferLimit if
// WithMaxBuffer is too small. Bytes that were buffered before an error stay
// readable: Fill succeeds while Buffered() >= n. Fill(0) always succeeds.
func (s *Stream) Fill(n int) error {
	if n < 0 {
		panic(fmt.Sprintf("bt: negative size %d", n))
	}
	if s.Buffered() >= n {
		return nil
	}
	if s.err != nil {
		if s.err == io.EOF {
			return io.ErrUnexpectedEOF
		}
		return s.err
	}

	for s.Buffered() < n {
		err := s.readMore()
		if err != nil {
			s.err = err
			if err == io.EOF && s.Buffered() < n {
				return io.ErrUnexpectedEOF
			}
			if err != io.EOF {
				return err
			}
			break
		}
	}
	return nil
}

// Advance consumes n buffered bytes. It panics if n is negative or larger than
// Buffered.
func (s *Stream) Advance(n int) {
	if n < 0 {
		panic(fmt.Sprintf("bt: negative size %d", n))
	}
	if n > s.Buffered() {
		panic(fmt.Sprintf("bt: advance %d bytes at offset %d, buffered %d",
			n, s.off, s.Buffered()))
	}
	s.off += n
	if s.off == len(s.buf) {
		s.buf = s.buf[:0]
		s.off = 0
	}
}

// Discard buffers and drops the next n bytes.
func (s *Stream) Discard(n int) error {
	if err := s.Fill(n); err != nil {
		return err
	}
	s.Advance(n)
	return nil
}

func (s *Stream) readMore() error {
	if s.off == len(s.buf) {
		s.buf = s.buf[:0]
		s.off = 0
	} else if s.off >= streamCompactMin && s.off*2 >= len(s.buf) {
		n := copy(s.buf, s.buf[s.off:])
		s.buf = s.buf[:n]
		s.off = 0
	}

	if len(s.buf) == cap(s.buf) {
		grow := cap(s.buf) * 2
		if grow < streamInitCap {
			grow = streamInitCap
		}
		if s.max > 0 && grow > s.max {
			grow = s.max
		}
		if grow <= cap(s.buf) {
			return ErrBufferLimit
		}
		grown := make([]byte, len(s.buf), grow)
		copy(grown, s.buf)
		s.buf = grown
	}

	old := len(s.buf)
	s.buf = s.buf[:cap(s.buf)]
	n, err := s.r.Read(s.buf[old:])
	s.buf = s.buf[:old+n]

	if n == 0 && err == nil {
		s.empty++
		if s.empty >= streamMaxNoProgress {
			return io.ErrNoProgress
		}
	} else {
		s.empty = 0
	}
	return err
}
