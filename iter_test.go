package bt

import (
	"testing"
)

func TestRecords(t *testing.T) {
	data := make([]byte, 12)
	for i := range data {
		data[i] = byte(i)
	}

	c := NewCursor(data)
	var got []byte
	count := 0
	for rec := range c.Records(4) {
		count++
		for rec.BytesLeft() > 0 {
			got = append(got, rec.U8())
		}
	}

	if count != 3 {
		t.Fatalf("Records(4) yielded %d records, want 3", count)
	}
	if len(got) != len(data) {
		t.Fatalf("records produced %d bytes, want %d", len(got), len(data))
	}
	for i, b := range got {
		if b != data[i] {
			t.Fatalf("byte %d = %d, want %d", i, b, data[i])
		}
	}
	if c.BytesLeft() != 0 {
		t.Fatalf("parent BytesLeft() = %d, want 0", c.BytesLeft())
	}
}

func TestRecordsBreak(t *testing.T) {
	c := NewCursor([]byte{1, 2, 3, 4, 5, 6, 7, 8})

	for range c.Records(4) {
		break
	}

	if got := c.Pos(); got != 4 {
		t.Fatalf("Pos() after break = %d, want 4", got)
	}
}

func TestIndexedRecords(t *testing.T) {
	c := NewCursor([]byte{0, 0, 0, 0, 0, 0})

	var indexes []int
	for i, rec := range c.IndexedRecords(2) {
		indexes = append(indexes, i)
		if rec.BytesLeft() != 2 {
			t.Fatalf("record %d has %d bytes, want 2", i, rec.BytesLeft())
		}
	}

	if len(indexes) != 3 || indexes[0] != 0 || indexes[1] != 1 || indexes[2] != 2 {
		t.Fatalf("indexes = %v, want [0 1 2]", indexes)
	}
}

func TestChunks(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6}
	c := NewCursor(data)

	var chunks int
	for chunk := range c.Chunks(2) {
		chunks++
		if cap(chunk) != 2 {
			t.Fatalf("cap(chunk) = %d, want 2", cap(chunk))
		}
		if chunk[0] != data[(chunks-1)*2] {
			t.Fatalf("chunk %d = %v, unexpected", chunks, chunk)
		}
	}
	if chunks != 3 {
		t.Fatalf("Chunks(2) yielded %d chunks, want 3", chunks)
	}
	if c.BytesLeft() != 0 {
		t.Fatalf("parent BytesLeft() = %d, want 0", c.BytesLeft())
	}
}

func TestIndexedRecordsBreak(t *testing.T) {
	c := NewCursor([]byte{1, 2, 3, 4, 5, 6})

	for range c.IndexedRecords(2) {
		break
	}

	if got := c.Pos(); got != 2 {
		t.Fatalf("Pos() after break = %d, want 2", got)
	}
}

func TestChunksBreak(t *testing.T) {
	c := NewCursor([]byte{1, 2, 3, 4, 5, 6})

	for range c.Chunks(2) {
		break
	}

	if got := c.Pos(); got != 2 {
		t.Fatalf("Pos() after break = %d, want 2", got)
	}
}

func TestIteratorBadSizes(t *testing.T) {
	tests := []struct {
		name string
		run  func(c *Cursor)
	}{
		{"Records zero", func(c *Cursor) {
			for range c.Records(0) {
			}
		}},
		{"Records negative", func(c *Cursor) {
			for range c.Records(-1) {
			}
		}},
		{"Records trailing", func(c *Cursor) {
			for range c.Records(4) {
			}
		}},
		{"IndexedRecords zero", func(c *Cursor) {
			for range c.IndexedRecords(0) {
			}
		}},
		{"Chunks negative", func(c *Cursor) {
			for range c.Chunks(-1) {
			}
		}},
		{"Chunks trailing", func(c *Cursor) {
			for range c.Chunks(4) {
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCursor([]byte{1, 2, 3, 4, 5, 6})
			mustPanic(t, "bt: ", func() { tt.run(c) })
		})
	}
}

func TestRecordsAllocations(t *testing.T) {
	data := make([]byte, 64)

	allocs := testing.AllocsPerRun(100, func() {
		c := NewCursor(data)
		for rec := range c.Records(4) {
			sinkU8 = rec.U8()
		}
	})
	if allocs > 0 {
		t.Fatalf("Records iteration allocations = %v, want 0", allocs)
	}

	allocs = testing.AllocsPerRun(100, func() {
		c := NewCursor(data)
		for chunk := range c.Chunks(4) {
			sinkBytes = chunk
		}
	})
	if allocs > 0 {
		t.Fatalf("Chunks iteration allocations = %v, want 0", allocs)
	}
}
