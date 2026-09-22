# 7. Performance

`bt` is small enough that its performance can be explained rather than
memorised: a few compiler facts, one allocation rule, and the usual
CPU-versus-I/O trade-off. This chapter collects all of it.

## 7.1 How the numbers are produced

```sh
go test -run=^$ -bench=. -benchmem ./...
go test -run=^$ -bench=BenchmarkParseRecords -count=10 . | tee /tmp/old.txt
# apply a change
go test -run=^$ -bench=BenchmarkParseRecords -count=10 . | tee /tmp/new.txt
go run golang.org/x/perf/cmd/benchstat@latest /tmp/old.txt /tmp/new.txt
```

- `-benchmem` adds `B/op` and `allocs/op`; the allocation tests use
  `testing.AllocsPerRun` to assert zero.
- `benchstat` reports medians with confidence; a 2-3% delta across ten runs is
  real, a single run is not.
- Compiler decisions are visible with `go build -gcflags='-m -m' ./...`; this
  is how the inline budgets quoted below were measured.

Numbers in this chapter come from two machines; each table is labelled.

## 7.2 Reader numbers

i3-10100, Go 1.26 (from the README table):

| Operation | Time | Allocs |
| --- | --- | --- |
| `U16LE` / `U32LE` / `U64LE` | 4.2 / 4.0 / 4.1 ns | 0 |
| `RawStr(4)` / `StrUnsafe(4)` | 30 / 13 ns | 1 / 0 |
| `ParseRecords` (mixed fields + names) | 461 MB/s, ~38 ns/record | 1 per name copy |
| `Records(16)` | ~6 GB/s | 0 |
| `Chunks(16)` | ~11.7 GB/s | 0 |
| `Sub(16)` loop | ~0.3 GB/s | 1/record |
| `Stream` over `bytes.Reader` | ~1 GB/s, ~15 ns/record | 3 per stream |
| `UnsafeCast` (theoretical floor) | 0.54 ns | 0 |

Ryzen 5 5600, Go 1.27 (recent runs):

| Operation | Time | Notes |
| --- | --- | --- |
| `ULEB128` 1/2/3/5/10 bytes | 2.21 / 2.44 / 4.10 / 4.97 / 6.37 ns | unrolled, ch. 4 |
| `SLEB128` 1/2/3/5/10 bytes | 2.31 / 2.85 / 4.65 / 5.51 / 6.74 ns | unrolled, ch. 4 |
| `ParseRecords` | 821 MB/s, ~21 ns/record | includes string copies |
| `Records(16)` / `Chunks(16)` | 6.4 / 16.4 GB/s | 0 allocs |
| `Sub(16)` loop | 0.73 GB/s, 4096 allocs/op | 32 B per record |
| `Stream` over `bytes.Reader` | 1.26 GB/s | 3 allocs per stream |

The gap between `Chunks` (16 GB/s) and `Sub` (0.73 GB/s) is the practical
message: iterators yield values and slice views, `Sub` yields a heap cursor.

## 7.3 Writer numbers

Ryzen 5 5600, Go 1.27:

| Operation | Time | Compare |
| --- | --- | --- |
| `U16LE` / `U32BE` / `U64LE` | 2.4 / 2.2 / 2.5 ns | 0.7-0.8 ns bare `binary.*.AppendUint*` |
| `U16(order)` / `U32(order)` / `U64(order)` | 2.5 / 2.7 / 2.5 ns | devirtualised when the order is a constant |
| `U24LE` | 3.4 ns | no stdlib equivalent |
| `Writer.ULEB128` / `Writer.SLEB128` | 4.7 / 4.7 ns | no stdlib signed LEB |
| `AppendULEB128` | 3.7 ns | 3.0 ns `binary.AppendUvarint` |
| `AppendU24LE` | 2.4 ns | — |
| `RawStr(8)` / `CStr(8)` | 2.1 / 7.6 ns | `CStr` scans for NUL |
| `LenU8` section, 8-byte payload | 9.7 ns | reserve + closure + patch |
| 1024 mixed records | 3.8 GB/s, ~5.7 ns/record | 0 allocs |
| 4096×16 B via `Writer` | ~17 µs | one call each |
| 4096×16 B via `StreamWriter` | ~46 µs | per-field API |
| 4096×16 B via `bufio.Writer` | ~26 µs | byte batches, no encoding |

The ~1.7 ns gap between `bt` writers and bare `AppendUint*` is the price of the
slice header living behind a pointer (`w.b = append(w.b, ...)`) plus the
range/panic checks. The range checks are branches, not allocations.

## 7.4 Compiler facts worth knowing

Measured inline costs (budget 80):

| Function | Cost | Inlinable |
| --- | --- | --- |
| `Cursor.need`, `CanRead`, `Pos`, `Skip`, `I*`, `U16LE/BE`, `U24LE/BE` | small | yes |
| `Writer.ULEB128` / `SLEB128` | 25 / 49 | yes |
| `AppendULEB128` / `AppendSLEB128` | 39 / 63 | yes |
| `AppendU24LE` | 71 | yes |
| `StreamWriter.U16LE` and friends | 77-78 | yes, barely |
| `StreamWriter.afterWrite` | 85 | no |
| `Writer.U24LE` (contains `fmt.Sprintf`) | 86 | no |
| `Writer.WriteTo` | 128 | no |
| `Cursor.Sub` | 106 | no |
| `Cursor.ULEB128` / `SLEB128` (full) | 292 / 391 | no |

Consequences:

- `StreamWriter` sits at an equilibrium: making `afterWrite` inlineable would
  push every per-field method over the budget, so the current split (one call
  per field) is the best available arrangement.
- `Cursor.Sub` cannot inline, which is why it allocates; `Records`/`Chunks`
  avoid that by yielding values.
- `Writer.ULEB128` inlines, so the `Append*` wrappers that build a stack
  `Writer` remain cheap and allocation-free.
- The `LE`/`BE` reader wrappers duplicate the generic `order` bodies because a
  concrete byte order lets the compiler inline the whole read; the generic
  forms pay an interface dispatch (~1.5 ns on the i3 run).

Escape analysis highlights:

- `Cursor` itself usually stays on the stack when the caller does not leak it;
- `Sub` returns a pointer, so the cursor escapes and lands on the heap;
- `Append*` wrappers do not allocate the temporary `Writer`; only the returned
  slice may grow the backing array;
- `Writer.WriteTo` drains and compacts in place, no allocation.

## 7.5 Where the CPU actually goes

For a mixed record parser, per-field method calls and string copies dominate;
for bulk data, memory bandwidth does. Rule of thumb on a modern desktop core:

- zero-copy walk (`Chunks`, `Records`): 6-16 GB/s;
- per-field scalar loop: 200-800 MB/s depending on field mix;
- string copying (`RawStr`): ~1 GB/s per core;
- `Sub` per record: ~0.7 GB/s with one allocation per record.

`GOAMD64=v3` (or the equivalent on other architectures) can matter: the
`bits.Len64` in `ULEB128Size` compiles to `LZCNT`, which is roughly twice as
fast as `BSR` on older AMD cores. The protobuf team measured a 50% speed-up on
Zen 2 for `SizeVarint` from that alone.

## 7.6 CPU versus I/O: when `bt` is the bottleneck

Storage tiers (sequential throughput):

| Device | Throughput |
| --- | --- |
| HDD | 0.15-0.2 GB/s |
| SATA SSD | ~0.55 GB/s |
| NVMe PCIe 3.0 x4 | ~3.5 GB/s |
| NVMe PCIe 4.0 x4 | 5-7.5 GB/s |
| NVMe PCIe 5.0 x4 | 10-14 GB/s |
| Page cache (RAM) | 10-20 GB/s single core |

Case study: parse 100 WAV files of 20 MB (2 GB total, ~1e9 16-bit samples).

| Scenario | I/O | CPU | Bottleneck |
| --- | --- | --- | --- |
| Headers only (44 B each) | ~10-50 ms | ~0 | file opens |
| Read all, skip payload (`Skip`/`Chunks`) | 3.6 s cold / ~0.2 s cached | ~0.1-0.5 s | disk on SATA |
| Per-sample loop (`U16LE` × 1e9) | 3.6 s cold | ~2-3 s | disk on SATA, CPU on NVMe Gen4 |
| Same from page cache | ~0.2 s | ~2-3 s | CPU |

General rules:

- On SATA and HDD, even a naive per-field parser usually outruns the device;
  optimisation effort is better spent elsewhere.
- On Gen4/Gen5 NVMe, CPU is the wall. Zero-copy iterators keep up; per-field
  loops and `RawStr`/`Sub` per record do not.
- Writing is the mirror: `Writer` + one `WriteTo` moves the bottleneck to the
  device; `StreamWriter` per field can bottleneck first on fast NVMe.
- `fsync` makes the write durable at device speed; without it the page cache
  absorbs bursts and writeback happens in the background. Do not `fsync` per
  record.
- Random and small I/O is IOPS/latency-bound; parsing cost is irrelevant there.

## 7.7 Tuning checklist

1. Validate once per region, then read without checks.
2. `Grow` (or `Reserve`) before a write loop; `Reset` to reuse capacity.
3. Prefer `Chunks`/`Records` over `Sub` on hot paths.
4. Use `StrUnsafe` when the buffer outlives the string; `RawStr` otherwise.
5. For bulk output use `Writer` + `WriteTo`; for field-by-field streaming use
   `StreamWriter` with a threshold proportional to the record batch.
6. Bound streaming memory with `WithMaxBuffer` on the read side.
7. Measure with `benchstat`; a single `-bench` run is noise.
8. Check allocations with `testing.AllocsPerRun`, not with intuition.
