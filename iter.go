package bt

import (
	"fmt"
	"iter"
)

func (c *Cursor) checkChunkSize(size int) {
	if size <= 0 {
		panic(fmt.Sprintf("bt: bad chunk size %d", size))
	}
	if rem := c.BytesLeft() % size; rem != 0 {
		panic(fmt.Sprintf("bt: %d trailing bytes do not fit %d-byte chunks", rem, size))
	}
}

// Records yields fixed-size record cursors and consumes the parent cursor.
// It panics if size is not positive or if the remaining bytes are not a
// multiple of size. Records are yielded by value, so iterating allocates
// nothing as long as the loop body does not keep pointers to them.
func (c *Cursor) Records(size int) iter.Seq[Cursor] {
	return func(yield func(Cursor) bool) {
		c.checkChunkSize(size)
		for c.CanRead(size) {
			rec := Cursor{b: c.b[c.off : c.off+size : c.off+size]}
			c.off += size
			if !yield(rec) {
				return
			}
		}
	}
}

// IndexedRecords is like Records but also yields the zero-based record index.
func (c *Cursor) IndexedRecords(size int) iter.Seq2[int, Cursor] {
	return func(yield func(int, Cursor) bool) {
		c.checkChunkSize(size)
		for i := 0; c.CanRead(size); i++ {
			rec := Cursor{b: c.b[c.off : c.off+size : c.off+size]}
			c.off += size
			if !yield(i, rec) {
				return
			}
		}
	}
}

// Chunks yields zero-copy slices of size bytes and consumes the parent cursor.
// The same size rules as Records apply. The slices alias the cursor's buffer
// and are capped at size.
func (c *Cursor) Chunks(size int) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		c.checkChunkSize(size)
		for c.CanRead(size) {
			chunk := c.b[c.off : c.off+size : c.off+size]
			c.off += size
			if !yield(chunk) {
				return
			}
		}
	}
}
