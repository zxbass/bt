//go:build amd64 || arm64

package bt

import (
	"testing"
	"unsafe"
)

// BenchmarkUnsafeCast measures the theoretical floor: an unaligned-capable
// native-endian load with no bounds check, cursor bookkeeping or portability.
// The value is native-endian and only used to keep the load alive.
func BenchmarkUnsafeCast(b *testing.B) {
	buf := benchBuf()
	base := unsafe.Pointer(unsafe.SliceData(buf))
	b.SetBytes(4)
	b.ReportAllocs()

	off := 0
	for i := 0; i < b.N; i++ {
		if off > len(buf)-4 {
			off = 0
		}
		sinkU32 = *(*uint32)(unsafe.Add(base, off))
		off += 4
	}
}
