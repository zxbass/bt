# References

Sources behind the design, the implementation tricks and the performance
claims. Links were live at the time of writing; chapter references point at
where each source is used.

## Go language and standard library

- [The Go Programming Language Specification](https://go.dev/ref/spec) -
  shifts, conversions and two's complement semantics used throughout the
  scalar and varint code (chapters 2, 4).
- [Go Fuzzing](https://go.dev/doc/security/fuzz/) - the official guide to
  `testing.F`, corpus format and regression seeds (chapter 8).
- [`testing` package](https://pkg.go.dev/testing) - `AllocsPerRun`,
  `testing.F`, benchmark semantics (chapters 7, 8).
- [`encoding/binary`](https://pkg.go.dev/encoding/binary) - the baseline this
  package is compared against; `AppendByteOrder`, `AppendUvarint`, `Varint`
  (zigzag) are referenced in chapters 3, 4, 7.
- [`unsafe`](https://pkg.go.dev/unsafe) - `unsafe.String`/`unsafe.SliceData`
  behind `Cursor.StrUnsafe` (chapter 1).
- [`iter`](https://pkg.go.dev/iter) - the iterator types behind `Records`,
  `IndexedRecords`, `Chunks` (chapter 5).
- [`bytes.Buffer`](https://pkg.go.dev/bytes#Buffer) - the drain-on-`WriteTo`
  convention copied by `Writer.WriteTo` (chapter 3).
- [`bufio`](https://pkg.go.dev/bufio) - the batched-writer baseline in the
  `StreamWriter` benchmarks (chapters 6, 7).
- [`io`](https://pkg.go.dev/io) - `io.EOF`, `io.ErrUnexpectedEOF`,
  `io.ErrShortWrite`, `io.ErrNoProgress` semantics (chapters 1, 6).
- [Go blog: Strings, bytes, runes and characters in Go](https://go.dev/blog/strings) -
  background for the string helpers (chapter 2).
- [Go blog: Arrays, slices (and strings): The mechanics of 'append'](https://go.dev/blog/slices-intro) -
  why aliasing and capacity matter so much in this package (chapters 1, 3).
- [Go blog: Profiling Go Programs](https://go.dev/blog/pprof) - the workflow
  behind the benchmark methodology (chapter 7).

## Binary formats and varints

- [Wikipedia: LEB128](https://en.wikipedia.org/wiki/LEB128) - the encoding
  implemented by `ULEB128`/`SLEB128`, including the DWARF variant (chapter 4).
- [Wikipedia: Variable-length quantity](https://en.wikipedia.org/wiki/Variable-length_quantity) -
  the broader family of variable-length integer encodings (chapter 4).
- [Protocol Buffers: Encoding](https://protobuf.dev/programming-guides/encoding/) -
  wire types, base-128 varints, zigzag, and why `sintN` differs from `intN`
  (chapter 4.2).
- [DWARF Debugging Information Format](https://dwarfstd.org/) - the standard
  that gives `SLEB128` its sign-extension rules (chapter 4).
- [Wikipedia: DWARF](https://en.wikipedia.org/wiki/DWARF) - accessible
  overview (chapter 4).
- [Wikipedia: Two's complement](https://en.wikipedia.org/wiki/Two%27s_complement) -
  why negative varints cost ten bytes (chapter 4.9).
- [Wikipedia: Endianness](https://en.wikipedia.org/wiki/Endianness) - the
  `LE`/`BE`/`order` split (chapter 2.4).
- [Wikipedia: IEEE 754](https://en.wikipedia.org/wiki/IEEE_754) - the float
  bit layouts reinterpreted by `F32`/`F64` (chapter 2.4).
- [Wikipedia: Data structure alignment](https://en.wikipedia.org/wiki/Data_structure_alignment) -
  the padding handled by `Cursor.Align`/`Writer.Align` (chapter 3, recipe 9).
- [Wikipedia: Resource Interchange File Format](https://en.wikipedia.org/wiki/Resource_Interchange_File_Format)
  and [WAV](https://en.wikipedia.org/wiki/WAV) - chunk layout used in the WAV
  recipe (recipe 9).
- [Wikipedia: Cyclic redundancy check](https://en.wikipedia.org/wiki/Cyclic_redundancy_check) -
  the checksum patched in recipe 5.
- [Wikipedia: Protocol Buffers](https://en.wikipedia.org/wiki/Protocol_Buffers) -
  the format that popularised varints (chapter 4).

## Articles and blog posts

- [The CPU Cost of Protobuf Varints in Go](https://kmcd.dev/posts/protobuf-varint-vs-fixed/) -
  measured comparison of varint versus fixed-width encoding, including how
  bulk `memmove` beats per-element loops. Motivated the `Chunks`-first advice
  (chapters 5, 7).
- [How Protobuf Works - The Art of Data Encoding](https://victoriametrics.com/blog/go-protobuf/) -
  wire format walkthrough, `protowire` examples and easyproto benchmarks
  (chapters 4, 9).
- [Handling Binary Files in Go](https://lucasklassmann.com/blog/2018-07-21-handling-binary-files-in-go/) -
  the `binary.Read`-per-field style that `bt` replaces, and why it allocates
  (chapters 2, 9).
- [protowire source: `ConsumeVarint`](https://go.googlesource.com/protobuf/+/master/encoding/protowire/wire.go) -
  the flat, fully unrolled varint decoder that inspired `uleb128Rest`
  (chapter 4.4).
- [protobuf-go: micro-optimize `SizeVarint`](https://github.com/protocolbuffers/protobuf-go/commit/8e8926ef675d99b1c9612f5d008f4dc803839f7a) -
  the `bits.Len64`/`LZCNT` trick and its measured impact (chapters 4.6, 7.5).
- [golang/go#66253: cache `dataSize` for binary.Read/Write](https://github.com/golang/go/issues/66253) -
  reflection allocation profile of `binary.Read`/`Write`, the problem `bt`
  avoids entirely (chapter 9.1).
- [Inlining optimisations in Go](https://dave.cheney.net/2020/04/25/inlining-optimisations-in-go) -
  how the 80-unit inline budget works, which explains the duplicated LE/BE and
  `StreamWriter` bodies (chapters 7.4, 9.4).
- [Language Mechanics On Escape Analysis](https://www.ardanlabs.com/blog/2017/05/language-mechanics-on-escape-analysis.html) -
  why `Sub` allocates and `Records` does not (chapters 5.4, 7.4).

## Libraries compared

- [`google.golang.org/protobuf/encoding/protowire`](https://pkg.go.dev/google.golang.org/protobuf/encoding/protowire) -
  low-level protobuf wire helpers; the closest relative of this package.
- [protobuf-go](https://github.com/protocolbuffers/protobuf-go) - the Go
  protobuf runtime built on `protowire`.
- [`github.com/dennwc/varint`](https://pkg.go.dev/github.com/dennwc/varint) -
  specialized varint encode/decode; its benchmarks show what a tuned loop
  achieves (chapter 4.7).
- [easyproto](https://github.com/VictoriaMetrics/easyproto) - minimal
  zero-allocation protobuf marshalling used as a comparison point in the
  VictoriaMetrics article.
- [`github.com/happyxcj/alignbinary`](https://github.com/happyxcj/alignbinary) -
  alignment-aware struct codec; source of the `Align` idea (chapter 9.12).
- [`github.com/oy3o/codec`](https://github.com/oy3o/codec) - binary codec with
  error latching and zero-copy readers; independent validation of the
  `Stream`/`StreamWriter` error model (chapter 9.12).
- [`github.com/go-audio/wav`](https://github.com/go-audio/wav) - a typical
  domain parser built on `encoding/binary`; the contrast that shaped the WAV
  recipe (recipe 9).
- [FlatBuffers](https://flatbuffers.dev/) - access without parsing; the
  inspiration for treating chunk iteration as the fast path (chapter 9.12).
- [Cap'n Proto](https://capnproto.org/) - zero-copy serialization, same
  motivation.
- [Kaitai Struct](https://kaitai.io/) - declarative binary format descriptions
  compiled to parsers; the code-generation approach `bt` deliberately avoids
  (chapter 9.11).
- [Rust `byteorder`](https://crates.io/crates/byteorder) - the closest Rust
  analogue: slice-based primitives with explicit lifetimes.
- [Rust `binrw`](https://crates.io/crates/binrw) and
  [`nom`](https://crates.io/crates/nom) - derive-based and combinator-based
  parsing, compared conceptually in chapter 9.
- [Python `struct`](https://docs.python.org/3/library/struct.html) - the
  readability/speed trade-off in the other direction.

## Storage and I/O background

- [Wikipedia: Solid-state drive](https://en.wikipedia.org/wiki/Solid-state_drive) -
  SATA/NVMe throughput classes quoted in chapter 7.6.
- [Wikipedia: NVM Express](https://en.wikipedia.org/wiki/NVM_Express) -
  PCIe generation bandwidth.
- [Wikipedia: Hard disk drive](https://en.wikipedia.org/wiki/Hard_disk_drive) -
  the 0.15-0.2 GB/s sequential tier.
- [Wikipedia: IOPS](https://en.wikipedia.org/wiki/IOPS) - why random access is
  latency-bound, not throughput-bound.
- [Wikipedia: Page cache](https://en.wikipedia.org/wiki/Page_cache) - why the
  second read of a file is memory-speed, and what `fsync` changes.

## Tools

- [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) - the
  benchmark comparison tool used for every performance claim (chapter 7.1).
- [`go tool cover`](https://pkg.go.dev/cmd/cover) - the coverage driver behind
  the 100% policy (chapter 8.1).
- `go build -gcflags='-m -m'` - the compiler's inlining and escape decisions
  quoted in chapter 7.4.
- [`cmd/btdebug`](../cmd/btdebug/main.go) - the interactive byte playground
  (recipe 11).

## Books and standards

- Randal E. Bryant, David R. O'Hallaron, *Computer Systems: A Programmer's
  Perspective* - two's complement, IEEE 754, memory hierarchy, and the
  CPU-versus-I/O framing in chapter 7.6.
- Donald E. Knuth, *The Art of Computer Programming, Vol. 4A* - variable-length
  codes and bit-level data structures (chapter 4).
- *IEEE 754-2019, Standard for Floating-Point Arithmetic* - the exact bit
  layouts reinterpreted by `F32`/`F64`.
- *DWARF Debugging Information Format, Version 5* - normative source for the
  signed LEB128 variant implemented here.
- David Goldberg, [What Every Computer Scientist Should Know About
  Floating-Point Arithmetic](https://docs.oracle.com/cd/E19957-01/806-3568/ncg_goldberg.html) -
  background for float-bit round-tripping.

## This project

- Repository: <https://github.com/zxbass/bt>
- Issues: <https://github.com/zxbass/bt/issues>
- Changelog: [`CHANGELOG.md`](../CHANGELOG.md)
- Runnable examples: [`example_test.go`](../example_test.go)
