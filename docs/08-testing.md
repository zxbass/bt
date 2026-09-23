# 8. Testing and fuzzing

The package keeps a hard bar: **100% statement coverage for the library
package** (`github.com/zxbass/bt`), fuzz targets for every decoding path, and
allocation assertions for the hot paths. `cmd/btdebug` is a debug tool and is
exempt (around 45%).

This chapter explains the machinery so new code can meet the same bar.

## 8.1 Coverage policy

```sh
go test -race -cover ./...
# ok  github.com/zxbass/bt
# ok  github.com/zxbass/bt/cmd/btdebug
```

100% is enforceable because the package is small and every branch exists for a
reason. When a line is hard to cover, the fix is usually one of:

- factor the cold path into a helper and call it directly from a test
  (`sectionLen` is tested directly for the 32-bit overflow case, which cannot
  be produced on 64-bit with real data);
- restructure so the fallthrough is a terminating `panic`, not dead code (the
  `needMessage` helper exists for exactly this reason in the varint decoders).

Dead code and unreachable `return`s show up immediately in coverage, which is
the point.

## 8.2 Test idioms

**Table tests everywhere.** A typical case:

```go
tests := []struct {
	name  string
	write func(w *Writer)
	want  []byte
}{
	{"U16LE", func(w *Writer) { w.U16LE(0x1234) }, []byte{0x34, 0x12}},
	...
}
```

**`mustPanic`** asserts both the panic and its message prefix:

```go
func mustPanic(t *testing.T, wantPrefix string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		if msg := fmt.Sprint(r); !strings.HasPrefix(msg, wantPrefix) {
			t.Fatalf("panic = %q, want prefix %q", msg, wantPrefix)
		}
	}()
	fn()
}
```

Because the panic strings are part of the contract, the prefix is checked, not
just "something panicked".

**Round-trip tests.** Every writer has a reader; the test writes a boundary
value and decodes it:

```go
w := bt.NewWriter()
w.SLEB128(v)
if got := bt.NewCursor(w.Bytes()).SLEB128(); got != v {
	t.Fatalf("round trip %d = %d", v, got)
}
```

Boundary tables include `0`, `±1`, powers of two around every encoding
boundary (`63/64`, `-64/-65`, `127/128`, `1<<14`, `1<<21`, ...), and
`math.MaxInt64`/`math.MinInt64`.

**Prefix tests for rollback.** A truncated varint must panic *and* leave the
offset unchanged:

```go
for i := range maxEncoding {
	c := bt.NewCursor(maxEncoding[:i])
	mustPanic(t, "bt: ", func() { c.ULEB128() })
	if c.Pos() != 0 {
		t.Fatalf("prefix %d moved the offset", i)
	}
}
```

**Allocation tests.** `testing.AllocsPerRun` turns "this should not allocate"
into a test failure instead of a code-review opinion:

```go
allocs := testing.AllocsPerRun(100, func() {
	w.Reset()
	w.U32LE(1)
	w.ULEB128(300)
	w.RawStr("hello")
})
if allocs > 0 {
	t.Fatalf("allocations = %v, want 0", allocs)
}
```

## 8.3 Fuzz targets

| Target | Property checked |
| --- | --- |
| `FuzzCursorNeverPanicsWhenEnoughBytes` | reads never panic when `CanRead` says they fit |
| `FuzzCursorNavigation` | `Bytes`/`Peek`/`Skip`/`Ensure` advance or panic consistently, never move out of bounds |
| `FuzzULEB128RoundTrip` | encoded bytes from `binary.PutUvarint` decode back to the same value |
| `FuzzSLEB128RoundTrip` | same for the independent DWARF-style encoder |
| `FuzzCStr` | `ErrNoNul` only without a NUL; the result is the prefix up to the first NUL |
| `FuzzWriterScalarsRoundTrip` | every writer/reader scalar pair round-trips, including float bit patterns |
| `FuzzWriterSectionsRoundTrip` | `LenU8`/`LenU16*`/`LenU32*` payload lengths read back correctly |

Run one locally:

```sh
go test -run=^$ -fuzz=FuzzWriterScalarsRoundTrip -fuzztime=30s .
```

When a fuzzer finds a failure, Go writes the input to
`testdata/fuzz/<Target>/` and keeps it in the repository as a regression seed.
The checked-in seed for `FuzzWriterSectionsRoundTrip` is a 300-byte payload with
`op=0`: it exercises the guard that skips payloads the `LenU8` prefix cannot
hold, so it runs on every `go test` without reaching the section body.

Lessons from the seeds:

- the section fuzzer found that `LenU8` panics for payloads over 255 bytes;
  the fix was to teach the fuzzer to skip lengths the prefix cannot hold;
- the writer scalar fuzzer confirmed that float round-trips must compare bit
  patterns (`math.Float32bits`), not values, because `NaN != NaN`.

## 8.4 Benchmarks

Benchmarks live next to the code (`bench_test.go`, `writer_bench_test.go`,
`unsafe_bench_test.go` with an `amd64 || arm64` build tag). They cover:

- every scalar read and write, plus the generic `order` forms;
- varints at 1/2/3/5/10 bytes;
- iterators and the `Sub` loop;
- `Stream` over `bytes.Reader`, 7-byte chunks, and a one-byte reader;
- `Writer`, `StreamWriter`, and `bufio.Writer` on identical record streams;
- stdlib baselines (`binary.Uint*`, `binary.Read`, `Append*`).

Baseline discipline:

```sh
go test -run=^$ -bench='BenchmarkULEB128|BenchmarkSLEB128' -count=10 . > /tmp/old.txt
# change the code
go test -run=^$ -bench='BenchmarkULEB128|BenchmarkSLEB128' -count=10 . > /tmp/new.txt
go run golang.org/x/perf/cmd/benchstat@latest /tmp/old.txt /tmp/new.txt
```

A change is accepted when `benchstat` says the intended cases improved and the
unintended ones did not regress. The varint unrolling (chapter 4) went through
three variants before one showed no regression on 1-2 byte values.

## 8.5 CI

`.github/workflows/ci.yml` runs four jobs:

1. **test**: matrix Go 1.23 and stable; `gofmt -l .` must be empty, `go vet`,
   then `go test -race -cover ./...`.
2. **staticcheck**: `honnef.co/go/tools/cmd/staticcheck@latest ./...`.
3. **fuzz**: each target in the matrix for 30 seconds.
4. **cross**: `GOOS=windows GOARCH=amd64 go build ./...`,
   `GOOS=linux GOARCH=arm64 go build ./...`, `GOOS=linux GOARCH=386 go test ./...`.

The 386 job is not ceremonial. `sectionLen` originally took an `int`, and the
test constants `0xFFFFFFFF` and `1<<32` do not fit a 32-bit `int`, so the test
package failed to compile on 386 while passing on amd64. The fix was to widen
the parameter to `uint64` and cast the real lengths; the guard itself can only
trigger on 64-bit platforms, which is why it also has a direct unit test.

## 8.6 Adding a feature: checklist

1. Write the table/round-trip test first; include boundaries and one panic.
2. Implement, keeping the panic messages in the documented format.
3. Add a benchmark if it is on a hot path; compare with `benchstat`.
4. Add an allocation test if it claims zero allocations.
5. Add or extend a fuzz target if it decodes bytes.
6. Keep library coverage at 100%; `go tool cover -func` lists offenders.
7. Run `gofmt`, `go vet`, `staticcheck`, `go test -race -cover ./...`, and the
   cross builds before pushing.
8. Update `README.md`/`CHANGELOG.md` and the relevant `docs/` chapter.
