# 6. Streaming in and out

`Stream` and `StreamWriter` adapt the package to `io.Reader`/`io.Writer` while
keeping the no-error-noise style: parsing still panics on malformed access, but
I/O failures become sticky errors.

## 6.1 `Stream`: incremental parsing

`Stream` reads from an `io.Reader` into a growable window. The workflow is
three steps:

1. `Fill(n)` guarantees at least `n` bytes are buffered (or returns the I/O
   error that prevented it).
2. `Cursor()` returns a cursor over the buffered bytes; parse freely.
3. `Advance(n)` commits how many bytes you consumed; `Discard(n)` fills and
   commits in one call.

```go
st := bt.NewStream(conn)

for {
	if err := st.Fill(4); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) && st.Buffered() == 0 {
			break // clean end of stream
		}
		return err
	}

	c := st.Cursor()
	kind := c.U8()
	size := int(c.U16LE())
	st.Advance(3)

	if err := st.Fill(size); err != nil {
		return err
	}
	body := st.Cursor().Bytes(size)
	st.Advance(size)

	handle(kind, body)
}
```

Key points:

- `Fill` returns `io.ErrUnexpectedEOF` when the stream ends before `n` bytes;
  a clean end (`io.EOF` with nothing buffered) is recognised as above.
- Errors are **sticky**: after a failure `Fill` keeps returning the same error,
  and `Err()` exposes the underlying cause (`io.EOF` for a clean end).
- Bytes buffered before a failure stay readable: `Fill` succeeds while
  `Buffered() >= n`, which lets a parser drain a final partial record.
- Cursors from `Cursor()` are invalidated by the next `Fill`/`Advance`, because
  the window may move or compact. Copy what must outlive the step.
- `Fill(0)` always succeeds.

### Memory behaviour

Without a limit, the window grows by doubling starting from 512 bytes. A
consumed prefix is compacted to the front once it is at least 4096 bytes and
accounts for half the buffer:

```go
} else if s.off >= streamCompactMin && s.off*2 >= len(s.buf) {
	n := copy(s.buf, s.buf[s.off:])
	s.buf = s.buf[:n]
	s.off = 0
}
```

`WithMaxBuffer(n)` caps growth; a `Fill` that would exceed the cap returns
`ErrBufferLimit` (sticky, like any other error):

```go
st := bt.NewStream(r, bt.WithMaxBuffer(1<<20))
if err := st.Fill(1 << 21); errors.Is(err, bt.ErrBufferLimit) {
	// record larger than the configured cap
}
```

A reader that returns `(0, nil)` forever would otherwise spin; after 100
zero-progress reads `Fill` returns `io.ErrNoProgress`.

### One-shot reads

If buffering everything is acceptable, skip `Stream`:

```go
c, err := bt.NewCursorFromReader(r)
if err != nil {
	return err
}
if !c.CanRead(4) {
	return io.ErrUnexpectedEOF
}
```

## 6.2 `StreamWriter`: buffered writes with sticky errors

`StreamWriter` is the mirror image. The typed methods never return errors; the
first failure is recorded and every later typed write becomes a no-op.

```go
sw := bt.NewStreamWriter(conn)

for _, rec := range records {
	sw.U16LE(rec.Kind)
	sw.U32LE(rec.ID)
	sw.RawStr(rec.Name)
}

if err := sw.Flush(); err != nil {
	return err
}
```

### Flush threshold

Buffered bytes are flushed automatically once they **reach** the threshold
(`>=`), 64 KiB by default. `WithFlushThreshold(n)` changes it; values `<= 0`
switch to manual mode where only `Flush` writes to the destination.

```go
sw := bt.NewStreamWriter(f, bt.WithFlushThreshold(1<<20))
// ... write a lot ...
if err := sw.Flush(); err != nil {
	return err
}
```

`Grow(n)` reserves buffer capacity (a no-op after a sticky error), so a large
export can be allocation-free after one call:

```go
sw.Grow(1 << 20)
```

### Error propagation

- Typed writes (`U16LE`, `RawStr`, ...) discard the error; check `Err()` or the
  final `Flush()`.
- `Write`, `WriteByte`, `WriteString` return the sticky error, **including a
  flush failure caused by the call itself**:

```go
n, err := sw.Write(payload)
// n == len(payload) when the bytes were buffered; err != nil when the
// auto-flush that this call triggered failed and dropped the buffer
```

- After a failure every typed write is a no-op until `Reset()`, which clears
  the error, the buffer and any open reserves.

That last rule is what makes `io.Copy(sw, src)` safe: the copy stops at the
first error instead of reporting success after lost bytes.

### Sections on a stream

Length prefixing needs the reserved bytes and the body to stay in the buffer
until the length is known, which conflicts with automatic flushing. `Reserve`
handles it by tracking reservations as ranges:

```go
sw := bt.NewStreamWriter(out)

off := sw.Reserve(2)
start := sw.Buffered()
sw.RawStr(payload)
sw.PatchU16LE(off, uint16(sw.Buffered()-start))
```

Rules:

- while any reservation is open, automatic flushing is suspended;
- a `Patch*` call whose position lies inside a reserved range releases that
  range; patching an unrelated earlier field does **not** release it;
- a patch never flushes by itself; flushing resumes on the next write or an
  explicit `Flush`;
- `Flush` with an open reservation panics (`bt: Flush with open Reserve`)
  instead of silently invalidating positions;
- because patches do not flush, several patches inside one reservation are
  safe;
- finish every patch **before** the next write or `Flush`: that write may flush
  and drain the buffer, and an old position then either panics or, worse, points
  at unrelated bytes of a later record. Reserve, write the body, patch, and only
  then continue writing;
- inside a `Len*` closure write only through the provided `*Writer`. The
  `StreamWriter` does not register a reservation for `Len*`, so writing through
  `sw` from the closure can trigger a flush in the middle of the section.

The closure helpers are simpler when the prefix is at the start of the value:

```go
sw.LenU16BE(func(w *bt.Writer) {
	w.CStr("id")
	w.U32LE(7)
})
```

`Len*` builds the whole section in the internal buffer and only then checks the
threshold, so it needs no reservation bookkeeping.

### When to use what

| Goal | Tool | Why |
| --- | --- | --- |
| Many small fields to a socket/file | `StreamWriter` | sticky errors, threshold batching, no per-field error checks |
| One big encoded record | `Writer` + `WriteTo` | single `Write` call, `WriteTo` drains |
| Raw byte batching | `bufio.Writer` | less per-field overhead; `bt` buys the panic-free encoding API |
| Parse from a reader | `Stream` | bounded window, panic parsing, sticky I/O errors |

Measured on the Ryzen machine: 4096 fixed 16-byte records take ~46 µs through
`StreamWriter` versus ~26 µs through `bufio.Writer`; the gap is the per-field
method call. `Writer` + `WriteTo` produces the same bytes at ~17 µs plus one
system call, which is why it is the recommendation for bulk output.
