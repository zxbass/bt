# 10. Cookbook

Short, complete recipes for the situations that come up most often. Snippets
are illustrative; error handling follows the contract from chapter 1.

## 1. TLV records from a buffer

Tag, length, value: the classic layout for trusted buffers.

```go
func walkTLV(data []byte, handle func(kind byte, value *bt.Cursor)) {
	c := bt.NewCursor(data)
	for c.BytesLeft() > 0 {
		kind := c.U8()
		size := int(c.U16LE())
		if !c.CanRead(size) {
			panic("tlv record overflows the buffer")
		}
		handle(kind, c.Sub(size))
	}
}
```

`Sub` bounds every handler to its record, so a handler bug cannot corrupt the
walk. If the per-record allocation matters, parse in place with `Skip`/`Bytes`
(chapter 5.5).

## 2. Fixed-size frames with an index

```go
func findMagic(data []byte, magic uint32) []int {
	var hits []int
	c := bt.NewCursor(data)
	for i, rec := range c.IndexedRecords(16) {
		if rec.U32BE() == magic {
			hits = append(hits, i)
		}
	}
	return hits
}
```

Zero allocations for the iteration itself; only the `hits` slice grows.

## 3. A big-endian header

```go
type Header struct {
	Magic   uint32
	Version uint16
	Flags   uint16
	Length  uint32
}

func readHeader(c *bt.Cursor) Header {
	c.Ensure(12)
	return Header{
		Magic:   c.U32BE(),
		Version: c.U16BE(),
		Flags:   c.U16BE(),
		Length:  c.U32BE(),
	}
}
```

`Ensure` turns a truncated header into the documented panic; the caller knows
the minimum size and can validate once at the top.

## 4. Length-prefixed messages with `Writer`

```go
func encodeMessage(name string, id uint32, body []byte) []byte {
	w := bt.NewWriter()
	w.Grow(64)

	w.LenU16LE(func(w *bt.Writer) {
		w.CStr(name)
		w.U32LE(id)
	})
	w.LenU32BE(func(w *bt.Writer) {
		w.Write(body)
	})
	return w.Bytes()
}
```

Both sections are independent; nesting is allowed and the length checks happen
after each closure runs.

## 5. Patching a checksum

```go
func encodeWithCRC(body []byte) []byte {
	w := bt.NewWriter()
	crcOff := w.Reserve(4)

	payloadStart := w.Len()
	w.Write(body)

	crc := crc32.ChecksumIEEE(w.Bytes()[payloadStart:])
	w.PatchU32LE(crcOff, crc)
	return w.Bytes()
}
```

`Reserve` returns the offset, the payload is written, and `PatchU32LE`
backfills the checksum. The same pattern works for total lengths, offsets into
a table, or forward references.

## 6. Streaming a frame protocol

```go
func readFrames(r io.Reader, handle func(kind byte, body []byte)) error {
	st := bt.NewStream(r, bt.WithMaxBuffer(1<<20))

	for {
		if err := st.Fill(3); err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) && st.Buffered() == 0 {
				return nil // clean EOF between frames
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
		handle(kind, body)
		st.Advance(size)
	}
}
```

`body` is only valid until the next `Fill`/`Advance`; copy it inside `handle`
if it must outlive the call. `WithMaxBuffer` turns an oversized frame into a
sticky `ErrBufferLimit` instead of unbounded memory growth.

## 7. Exporting records with `StreamWriter`

```go
func export(f *os.File, records []Record) error {
	sw := bt.NewStreamWriter(f, bt.WithFlushThreshold(1<<20))
	sw.Grow(1 << 16)

	for _, rec := range records {
		sw.U16LE(rec.Kind)
		sw.U32LE(rec.ID)
		sw.CStr(rec.Name)
	}
	return sw.Flush()
}
```

No per-field error checks: the first failure disables further writes and
`Flush` reports it. For a single huge record, prefer `Writer` plus one
`WriteTo(f)`.

## 8. Hashing or compressing in chunks

```go
func digest(data []byte) uint32 {
	h := crc32.NewIEEE()
	c := bt.NewCursor(data)
	for chunk := range c.Chunks(1 << 20) {
		h.Write(chunk)
	}
	return h.Sum32()
}
```

Zero copies and zero allocations in the walk; the hash dominates. Use the same
pattern to feed `io.CopyBuffer`, `flate.Writer`, or a syscall.

## 9. Parsing a WAV header

RIFF is little-endian, chunk-based, and padded to even sizes, which makes it a
good demonstration of `Sub`, `Bytes`, and `Align`.

```go
func parseWAV(data []byte) error {
	c := bt.NewCursor(data)

	if c.RawStr(4) != "RIFF" {
		return errors.New("not a RIFF file")
	}
	_ = c.U32LE() // file size minus 8
	if c.RawStr(4) != "WAVE" {
		return errors.New("not a WAVE file")
	}

	for c.BytesLeft() >= 8 {
		id := c.RawStr(4)
		size := int(c.U32LE())
		if !c.CanRead(size) {
			return fmt.Errorf("chunk %q overflows the file", id)
		}

		switch id {
		case "fmt ":
			f := c.Sub(size)
			format := f.U16LE()
			channels := f.U16LE()
			sampleRate := f.U32LE()
			_ = f.U32LE() // byte rate
			_ = f.U16LE() // block align
			bits := f.U16LE()
			fmt.Println(format, channels, sampleRate, bits)
		case "data":
			samples := c.Bytes(size) // zero-copy view
			_ = samples
		default:
			c.Skip(size)
		}

		if size%2 == 1 {
			_ = c.Align(2) // RIFF chunks start even-aligned; add the pad byte
		}
	}
	return nil
}
```

For a 20 MB `data` chunk, `Bytes` returns a slice into the original buffer; a
peak scan would hash or reduce it in place, and a full per-sample loop would
cost roughly 2-3 ns per 16-bit sample (chapter 7.6).

## 10. Round-trip fuzz template

```go
func FuzzMyRecordRoundTrip(f *testing.F) {
	f.Add(uint32(0), int32(0), "name")

	f.Fuzz(func(t *testing.T, id uint32, delta int32, name string) {
		if strings.ContainsRune(name, 0) {
			t.Skip() // CStr rejects embedded NULs
		}

		w := bt.NewWriter()
		w.U32LE(id)
		w.SLEB128(int64(delta))
		w.CStr(name)

		c := bt.NewCursor(w.Bytes())
		if c.U32LE() != id {
			t.Fatal("id mismatch")
		}
		if c.SLEB128() != int64(delta) {
			t.Fatal("delta mismatch")
		}
		if got := c.StrOrRest(c.BytesLeft()); got != name {
			t.Fatalf("name = %q, want %q", got, name)
		}
		if c.BytesLeft() != 0 {
			t.Fatal("trailing bytes after round trip")
		}
	})
}
```

Add the target to the CI fuzz matrix and commit any seed Go writes to
`testdata/fuzz/` (chapter 8.3).

## 11. Debugging bytes with `cmd/btdebug`

```sh
go run ./cmd/btdebug -demo
go run ./cmd/btdebug -hex "42 02 01 01 02" -seq "u8,u16le,u16be"
printf '\x01\x02\x03\x04' | go run ./cmd/btdebug -seq "u32le,sleb"
```

`-seq` accepts every reader in the package (`u24be`, `i24le`, `f64be`,
`uleb`, `sleb`, `str:N`, `cstr:N`, `sub:N`, ...). On a
panic the tool prints the failing op, offset, and message, which is often
enough to locate a bad header field without a debugger.

## 12. Reading files

```go
func open(path string) (*bt.Cursor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bt.NewCursor(data), nil
}
```

- For a one-shot small file this is the simplest option.
- For large files, stream with `bt.NewStream(f)` and parse incrementally.
- For many files, reuse a `bt.Writer`/buffer where possible; the OS page cache
  usually dominates the parse cost on SATA (chapter 7.6).
- If the file is memory-mapped elsewhere, `bt` accepts the resulting slice
  directly; it does not care where the bytes came from.
