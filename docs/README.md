# bt: internals and usage guide

A textbook-style walkthrough of the `bt` package: the contracts behind every
type, the tricks used in the implementation, and worked examples. It is meant
to be read in order, but each chapter is self-contained.

The generated API documentation (`go doc github.com/zxbass/bt`) stays the
authoritative reference; these chapters explain *why* the API looks the way it
does and how to keep it fast.

## Who this is for

- Users who want to write a parser or encoder and need to know which helper to
  reach for, and what it costs.
- Contributors who want to change the code without breaking its contracts or
  its performance envelope.

## Conventions

- Snippets assume `import "github.com/zxbass/bt"`.
- `c` is a `*bt.Cursor`, `w` is a `*bt.Writer`, `st` is a `*bt.Stream`, `sw` is
  a `*bt.StreamWriter`.
- Performance numbers come from the machine noted next to the table. Absolute
  nanoseconds are machine-dependent; relative comparisons from a single run are
  meaningful.
- "Allocation-free" always means *after* capacity is in place, usually via
  `Grow`.

## Reading paths

- **I want to parse a file**: [01-contract](01-contract.md) ->
  [02-reading](02-reading.md) -> [05-iterators](05-iterators.md) ->
  [06-streaming](06-streaming.md).
- **I want to encode a wire format**: [01-contract](01-contract.md) ->
  [03-writing](03-writing.md) -> [04-varints](04-varints.md) ->
  [10-cookbook](10-cookbook.md).
- **I maintain the package**: [07-performance](07-performance.md) ->
  [08-testing](08-testing.md) -> [09-design-decisions](09-design-decisions.md).
- **I just want recipes**: [10-cookbook](10-cookbook.md).

## Package map

| File | Public surface | Subtleties |
| --- | --- | --- |
| `bt.go` | `Cursor`, `CStr`, `CStrOrRest`, `ErrNoNul`, `ErrTruncated`, `ErrVarintOverflow` | panic/rollback contract, zero-copy slices, unrolled varints, `TryULEB128`/`TrySLEB128`, `Align` |
| `io.go` | `NewCursorFromReader`, `Cursor.Read`, `Cursor.ReadByte` | `io.EOF` instead of panics at the buffer end |
| `iter.go` | `Records`, `IndexedRecords`, `Chunks` | `iter.Seq`, zero allocations, capped slices |
| `stream.go` | `Stream`, `WithMaxBuffer`, `ErrBufferLimit` | sticky I/O errors, window compaction, buffer limits |
| `writer.go` | `Writer`, `zeroPad` | append model, `WriteTo` draining, `Reserve`/`Patch`, `Len*` sections |
| `append.go` | `AppendU24LE/BE`, `AppendULEB128`, `AppendSLEB128`, `ULEB128Size`, `SLEB128Size` | stateless appends, `bits.Len64` size formulas |
| `stream_writer.go` | `StreamWriter`, `WithFlushThreshold` | sticky errors, flush threshold, reserve ranges |

## Chapters

1. [The contract](01-contract.md)
2. [Reading with Cursor](02-reading.md)
3. [Writing with Writer](03-writing.md)
4. [Varints in depth](04-varints.md)
5. [Iterators](05-iterators.md)
6. [Streaming in and out](06-streaming.md)
7. [Performance](07-performance.md)
8. [Testing and fuzzing](08-testing.md)
9. [Design decisions](09-design-decisions.md)
10. [Cookbook](10-cookbook.md)
11. [References](references.md)

## Related documentation

- `README.md` at the repository root: pitch, API table, benchmarks, CI.
- `CHANGELOG.md`: what changed in each release and why it is safe to upgrade.
- `example_test.go`: runnable examples that `go test` executes.
