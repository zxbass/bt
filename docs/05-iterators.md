# 5. Iterators

Fixed-size records are the most common binary layout: frames, sectors, table
rows, tiles. `bt` exposes three Go 1.23 range-over-function iterators that walk
them without allocating or copying.

## 5.1 `Records(size)`

```go
for rec := range c.Records(16) {
	id := rec.U16LE()
	flags := rec.U8()
	_ = id
	_ = flags
}
```

Properties:

- yields `Cursor` **by value**, each backed by a capped sub-slice of the parent
  buffer;
- consumes the parent: after the loop, `c.BytesLeft() == 0` (or the position
  after the last yielded record if the loop `break`s);
- panics if `size <= 0` with `bt: bad chunk size N`;
- panics if the remaining bytes are not a multiple of `size`:
  `bt: 5 trailing bytes do not fit 4-byte chunks`.

The trailing-bytes panic is intentional. Silent truncation of a record stream
is a classic source of bugs; if a trailing partial record is expected, check
`c.BytesLeft() % size` yourself before iterating.

`break` stops the iterator where it is:

```go
for rec := range c.Records(4) {
	if rec.U8() == 0 {
		break
	}
}
// c.Pos() is a multiple of 4, pointing after the record that broke the loop
```

## 5.2 `IndexedRecords(size)`

Identical, plus the zero-based index:

```go
for i, rec := range c.IndexedRecords(recSize) {
	if rec.U32LE() == magic {
		fmt.Println("magic at record", i)
	}
}
```

## 5.3 `Chunks(size)`

`Chunks` yields raw `[]byte` windows instead of cursors, which is what bulk
processing wants:

```go
h := crc32.NewIEEE()
for chunk := range c.Chunks(1 << 20) {
	h.Write(chunk)
}
sum := h.Sum32()
```

- slices alias the parent buffer and are capped at `size`;
- same size rules as `Records`: positive size, no partial tail;
- zero copies: the hash sees the original bytes.

## 5.4 Why these allocate nothing

Two details make the iterators allocation-free:

1. **Value yields.** `Records` builds `Cursor{b: ...}` on the stack and passes
   it by value; nothing escapes to the heap. A pointer-returning API would
   allocate per record unless the compiler could prove the pointer dead.
2. **Capped slices.** Each record window is `b[off : off+size : off+size]`, so
   the yielded cursor can never expose memory outside its record even if the
   caller reslices.

The cost of the abstraction is one closure call per record, which the compiler
inlines into the range loop in most cases. Measured:

| Iterator | Throughput (Ryzen 5 5600) | Allocations |
| --- | --- | --- |
| `Records(16)` over 64 KiB | ~6.4 GB/s | 0 |
| `Chunks(16)` over 64 KiB | ~16.4 GB/s | 0 |
| `Chunks` + CRC32 | limited by the hash, still 0 by `bt` | 0 |

The allocation tests in `iter_test.go` use `testing.AllocsPerRun` to keep this
property from regressing.

## 5.5 Choosing between iterators, `Sub`, and `Stream`

| Tool | Use when | Cost |
| --- | --- | --- |
| `Records`/`IndexedRecords` | uniform fixed-size frames in memory | 0 allocs |
| `Chunks` | bulk copy/hash/compress of fixed-size pieces | 0 allocs |
| `Sub` | variable-size records with a length field | 32 B/record |
| `SubInto` | same, with a caller-owned cursor to reuse | 0 allocs |
| `Stream` | data arrives incrementally or must not be fully buffered | 3 allocs per stream |

`Sub` is the odd one out because it returns `*Cursor` (chapter 2.6). For
variable-size records in memory, the zero-allocation alternative is to parse in
place with `Skip`/`Bytes` and copy out only what escapes.

## 5.6 Example: a record file with a directory

```go
const recSize = 24

c := bt.NewCursor(data)

// First pass: find the record with the highest id.
best, bestID := -1, uint32(0)
for i, rec := range c.IndexedRecords(recSize) {
	if id := rec.U32LE(); id >= bestID {
		best, bestID = i, id
	}
}

// c is consumed; use a fresh cursor for the second pass.
c = bt.NewCursor(data)
for i, rec := range c.IndexedRecords(recSize) {
	if i != best {
		continue
	}
	_ = rec.Bytes(recSize)
}
```

Iterators consume the parent, so a second pass needs a fresh `NewCursor` over
the same buffer. That is cheap: a cursor is three words.
