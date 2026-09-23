# 9. Design decisions

Every sharp edge in `bt` is deliberate. This chapter collects the decisions,
the alternatives that were rejected, and where the ideas came from.

## 9.1 Panic instead of error returns

**Decision.** Reads past the end and writes that do not fit panic with a string.

**Why.** The package targets parsers for trusted input. Returning `(value,
error)` for every field triples the size of a parser and buries the two error
paths that matter (truncation of a region, and I/O) in noise. `CanRead`/
`Ensure` move validation to the region boundary, where it can be written once.

**Rejected alternatives.**

- `(value, error)` for every read: safer for untrusted data, verbose for the
  intended use case, and measurably slower (an error result is a branch and a
  register pair on every call).
- Saturating reads that return zero on out-of-bounds: silent data corruption.
- A debug/trace mode with checks compiled out: two behaviours to test and
  document, and the fast path would no longer be the tested path.

**Consequence.** Untrusted input needs an explicit validation layer. This is
documented prominently rather than hidden.

## 9.2 Failed reads roll back the offset

**Decision.** A panicking read leaves `Pos()` unchanged, including varints.

**Why.** Recovery and diagnostics. `cmd/btdebug` catches the panic, prints the
offset and the remaining bytes, and can be pointed at the exact failing field.
Sticky half-advances would make that impossible.

**Implementation.** Varints only commit `c.off` when the terminal byte is seen;
the unrolled tail applies `c.off += n` after the last check. Tests assert the
rollback for every truncation length.

## 9.3 Two error models, not one

**Decision.** Memory operations panic; I/O operations return sticky errors.

**Why.** I/O failures are environmental: disks fill up, sockets close. Panicking
on them would be hostile. Format errors are programmer errors: the validation
was wrong. Keeping the two apart means `Stream`/`StreamWriter` can promise
"never panic on data, always report I/O once".

## 9.4 Deliberate duplication

**Decision.** `U16LE`/`U16BE` duplicate the body of `U16(order)`, and
`StreamWriter` mirrors all of `Writer`'s typed methods.

**Why.** Inlining. A concrete byte order lets the compiler inline the read; an
interface call does not. A small per-field body keeps `StreamWriter.U16LE`
inside the 80-unit inline budget (cost 78) while still wrapping the writer and
the flush check. Shared helper code would push both over the budget and slow
the hot paths (see chapter 7 for the measured costs).

**Trade-off.** New methods must be added in two places. The `sampleWriter`
interface in the tests ensures both types expose the same method set, so a
missing mirror fails to compile.

## 9.5 `WriteTo` drains the buffer

**Decision.** `Writer.WriteTo` follows `bytes.Buffer`: it writes the buffered
bytes and removes them, keeping the capacity. Partial writes compact the
unwritten tail to the front.

**Why.** `io.WriterTo` semantics and retry-ability. Callers that want to keep
the bytes can copy or use `Bytes()` before the call; callers that stream a
large file want reuse without a second buffer.

**Rejected alternative.** Non-draining `WriteTo`: surprising for `io.Copy` and
for anyone who has used `bytes.Buffer`.

## 9.6 `CStr` refuses embedded NULs

**Decision.** `Writer.CStr(s)` panics if `s` contains `\x00`.

**Why.** The reader side (`CStr`, `StrOrRest`) stops at the first NUL. Writing
an embedded NUL would produce a value that reads back shorter than written, a
silent round-trip failure. Panicking at the write site makes the bug local.

## 9.7 `Reserve` suspends flushing; `Patch` only releases

**Decision.** On `StreamWriter`, `Reserve` suspends automatic flushing until
the reservation is released by a patch, and a patch never flushes by itself.
`Flush` with an open reservation panics.

**Why.** Patch positions are buffer offsets. If a flush could happen between
`Reserve` and `Patch`, positions could refer to already-sent bytes, and a later
patch would either panic or (after refilling the buffer) silently overwrite the
wrong bytes. Suspension makes the section safe by construction.

**Evolution.** The first implementation counted open reservations with an
`int`. That fails when an unrelated patch decrements the counter and re-enables
flushing. The current version stores `(start, end)` ranges and releases the
range containing the patch position, which also makes interleaved patches and
repeated patches inside one reservation safe.

**Why not flush on patch**: two patches into one reserved window would race a
flush between them. Patching is not a natural flush point; the next write or an
explicit `Flush` is.

## 9.8 `Sub` returns a pointer; `SubInto` is the zero-alloc escape hatch

**Decision.** Keep `func (c *Cursor) Sub(n int) *Cursor` as is (32 bytes and one
allocation per call) and add `SubInto(dst *Cursor, n int) *Cursor` for callers
who want to reuse a cursor.

**Why.** Returning `Cursor` by value would remove the allocation but break
chaining (`c.Sub(4).U8()` is invalid on a non-addressable value) and change the
public API. `Records`/`Chunks` cover fixed-size frames; `SubInto` covers
variable-size records in a hot loop without hidden state, and the benchmark
difference is large: `Sub` ~0.4 GB/s with 1 alloc/record versus `SubInto`
~3 GB/s with 0 allocs on the same data.

**Rejected alternative: an internal `sync.Pool`.** `Cursor` is 32 bytes, so
`Get`/`Put` overhead is comparable to the allocation it would save; the pool
also needs a `Release` API (use-after-release, double release, retaining large
buffers from GC) and introduces hidden global state, which this package
deliberately avoids (section 9.11). Caller-owned reuse keeps the lifetime
explicit.

## 9.9 Strict 24-bit range checks

**Decision.** `U24LE(0x1000000)` panics instead of writing the low 24 bits.

**Why.** Silent truncation is the worst failure mode in an encoder: the file
is produced, the checksum matches the truncated value, and the bug surfaces
only when a consumer reads a field that wraps around. A panic names the value
and its limit.

## 9.10 `sectionLen` takes `uint64`

**Decision.** `func sectionLen(n uint64, bits int) uint64`.

**Why.** On 32-bit platforms an `int` length cannot exceed `MaxInt32`, and
tests that exercise 32-bit overflow (`1<<32`) do not compile. The wider
parameter keeps the guard and its tests portable; see chapter 8.5 for the CI
failure that motivated it.

## 9.11 Non-goals

| Not provided | Why |
| --- | --- |
| Code generation / schemas | different project; generated code wins on schema-heavy formats, `bt` targets hand-written parsers |
| `mmap` | `bt` parses slices; mapping is an I/O concern |
| Zigzag varints | `encoding/binary` covers them; mixing both under one name would confuse (chapter 4.2) |
| Buffer pools | `Writer.Reset`/`Grow` give the caller control without hidden shared state |
| Text formats | JSON/CSV/XML have their own parsers |
| Concurrent types | single-owner slices; sharing is a design decision above this layer |

## 9.12 Ideas borrowed from other projects

| Source | Idea | Result in `bt` |
| --- | --- | --- |
| protobuf-go `encoding/protowire` | unrolled varint decoding, `SizeVarint` LZCNT trick | `uleb128Rest`/`sleb128Rest`, `ULEB128Size`/`SLEB128Size` |
| `github.com/dennwc/varint` | specialised varint fast paths beat generic loops | 3-10 byte varints ~2.5x faster, 1-2 byte unchanged |
| `encoding/binary` `Append*` convention | stateless append into an existing slice | `AppendU24LE/BE`, `AppendULEB128`, `AppendSLEB128` |
| `github.com/happyxcj/alignbinary` | alignment helpers for padded structs | `Cursor.Align`, `Writer.Align` |
| `github.com/oy3o/codec` | error latching for streaming writers | validates the `Stream`/`StreamWriter` sticky-error model |
| FlatBuffers / Cap'n Proto | zero-copy access beats parsing where possible | motivates `Chunks`/`Records` and the recipes that use them |
| `github.com/go-audio/wav` and friends | domain parsers trade speed for convenience | cookbook shows a WAV header parser in ~30 lines |
| Rust `byteorder`/`binrw`/`nom` | slice-based primitives with explicit lifetimes | same philosophy, Go-idiomatic names |

Ideas deliberately **not** borrowed: reflection-based struct codecs (slow),
generic buffer pools (hidden state), zero-copy schema formats (different
problem domain), and generated marshallers (out of scope).

## 9.13 Versioning

The package follows semantic versioning with an honest changelog:

- v0.1: readers, strings, varints, panic contract.
- v0.2: `io.Reader` support, iterators, `Stream`.
- v0.3: `Writer`, sections, `StreamWriter`.
- v0.4: unrolled varints, append helpers, `Align`.
- v0.5: `SubInto`, `TryULEB128`/`TrySLEB128`.

Additive changes bump the minor version, or a patch when they are a small
follow-up to the current minor (as `TryULEB128`/`TrySLEB128` are to v0.5); the
changelog describes the effect on the contract, not just the API. No breaking
change has been made yet, and the chapter 9.8 decision is the reason `Sub`
still returns a pointer.

## 9.14 `TryULEB128`/`TrySLEB128` for data-dependent lengths

**Decision.** Keep the panicking `ULEB128`/`SLEB128` and add error-returning
`TryULEB128`/`TrySLEB128` returning `ErrTruncated` or `ErrVarintOverflow`.

**Why.** A varint is the one read whose size cannot be validated up front:
`CanRead(10)` over-reads and fails on a valid short varint at the end of a
buffer, so at a buffer or stream boundary the panic is not preventable. The
package already has this shape for `CStr` (`ErrNoNul`) and `Stream` (sticky I/O
errors); `Try*` extends it to varints and makes the `Stream` refill loop
expressible without `recover`.

**Why not change `ULEB128`/`SLEB128` themselves.** It would force `if err !=
nil` at every call site, which is the cost the package exists to avoid, and it
would not help callers who validate their regions. `Try*` is opt-in.

**Why not `(value, bool)`.** The two failures need different responses:
`ErrTruncated` means refill and retry, `ErrVarintOverflow` means the data is
malformed. A boolean cannot carry that.

**Implementation.** A loop with a bounds check per byte, committing the offset
only on success; the unrolled panicking decoder stays the hot path. On the
i3-10100 the loop costs 4.0/5.6/7.3/10.8/18.5 ns for 1/2/3/5/10 bytes versus
2.6/3.2/5.3/6.3/8.5 ns unrolled, with zero allocations, and
`FuzzTryVarintMatchesPanic` keeps both implementations in agreement.
