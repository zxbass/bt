package bt_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/zxbass/bt"
)

func ExampleCursor() {
	data := []byte{
		0x42,       // u8
		0x02, 0x01, // u16le
		'd', 'e', 'v', 0x00,
	}
	c := bt.NewCursor(data)

	fmt.Println(c.U8())
	fmt.Println(c.U16LE())
	fmt.Println(c.StrOrRest(4))
	fmt.Println(c.BytesLeft())
	// Output:
	// 66
	// 258
	// dev
	// 0
}

func ExampleCursor_Ensure() {
	c := bt.NewCursor([]byte{0x01, 0x02})
	if c.CanRead(2) {
		fmt.Println(c.U16BE())
	}

	defer func() {
		fmt.Println(recover())
	}()

	c.Ensure(1)
	// Output:
	// 258
	// bt: need 1 bytes at offset 2, have 0
}

func ExampleCStr() {
	s, err := bt.CStr([]byte("hello\x00world"))
	fmt.Println(s, err)

	_, err = bt.CStr([]byte("no terminator"))
	fmt.Println(errors.Is(err, bt.ErrNoNul))
	// Output:
	// hello <nil>
	// true
}

func ExampleWriter() {
	w := bt.NewWriter()
	w.U16LE(0x1234)
	w.CStr("dev")
	w.SLEB128(-1)
	fmt.Printf("% x\n", w.Bytes())
	// Output:
	// 34 12 64 65 76 00 7f
}

func ExampleWriter_LenU16LE() {
	w := bt.NewWriter()
	w.LenU16LE(func(w *bt.Writer) {
		w.RawStr("hello")
		w.U8(7)
	})

	c := bt.NewCursor(w.Bytes())
	fmt.Println(c.U16LE())
	fmt.Println(c.StrOrRest(5))
	fmt.Println(c.U8())
	// Output:
	// 6
	// hello
	// 7
}

func ExampleWriter_Reserve() {
	w := bt.NewWriter()
	off := w.Reserve(2)
	w.RawStr("payload")
	w.PatchU16LE(off, uint16(w.Len()-2))

	c := bt.NewCursor(w.Bytes())
	fmt.Println(c.U16LE())
	fmt.Println(c.StrOrRest(7))
	// Output:
	// 7
	// payload
}

func ExampleStreamWriter() {
	var buf bytes.Buffer
	sw := bt.NewStreamWriter(&buf, bt.WithFlushThreshold(0))
	sw.U32BE(1)
	sw.RawStr("done")
	fmt.Println(buf.Len())
	if err := sw.Flush(); err != nil {
		fmt.Println(err)
	}
	fmt.Printf("% x\n", buf.Bytes())
	// Output:
	// 0
	// 00 00 00 01 64 6f 6e 65
}

func ExampleCursor_Sub() {
	data := []byte{
		3, 'o', 'n', 'e',
		3, 't', 'w', 'o',
	}
	c := bt.NewCursor(data)

	for c.BytesLeft() > 0 {
		rec := c.Sub(int(c.U8()))
		fmt.Println(rec.RawStr(rec.BytesLeft()))
	}
	// Output:
	// one
	// two
}

func ExampleCursor_IndexedRecords() {
	data := []byte{1, 2, 3, 4, 5, 6}
	c := bt.NewCursor(data)

	for i, rec := range c.IndexedRecords(3) {
		fmt.Println(i, rec.U8(), rec.U16LE())
	}
	// Output:
	// 0 1 770
	// 1 4 1541
}

func ExampleCursor_Chunks() {
	data := []byte("abcdabcdabcdabcd")
	c := bt.NewCursor(data)

	total := 0
	for chunk := range c.Chunks(4) {
		for _, b := range chunk {
			total += int(b)
		}
	}
	fmt.Println(total)
	// Output:
	// 1576
}

func ExampleCursor_ULEB128() {
	w := bt.NewWriter()
	w.ULEB128(300)
	w.SLEB128(-5)

	c := bt.NewCursor(w.Bytes())
	fmt.Println(c.ULEB128(), c.SLEB128())
	// Output:
	// 300 -5
}

func ExampleCursor_StrUnsafe() {
	data := []byte("zero-copy\x00rest")
	c := bt.NewCursor(data)

	s := c.StrUnsafe(9)
	fmt.Println(s, c.BytesLeft())
	// Output:
	// zero-copy 5
}

func ExampleStream() {
	r := bytes.NewReader([]byte{
		3, 'a', 'b', 'c',
		2, 'h', 'i',
	})
	st := bt.NewStream(r)

	for {
		if err := st.Fill(1); err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) && st.Buffered() == 0 {
				break
			}
			panic(err)
		}
		size := int(st.Cursor().U8())
		st.Advance(1)

		if err := st.Fill(size); err != nil {
			panic(err)
		}
		body := st.Cursor().RawStr(size)
		st.Advance(size)
		fmt.Println(body)
	}
	// Output:
	// abc
	// hi
}

func ExampleWithMaxBuffer() {
	st := bt.NewStream(bytes.NewReader([]byte{1, 2, 3}), bt.WithMaxBuffer(2))

	err := st.Fill(3)
	fmt.Println(errors.Is(err, bt.ErrBufferLimit))
	// Output:
	// true
}

func ExampleStreamWriter_LenU16BE() {
	var buf bytes.Buffer
	sw := bt.NewStreamWriter(&buf, bt.WithFlushThreshold(0))

	sw.LenU16BE(func(w *bt.Writer) {
		w.CStr("id")
		w.U32LE(7)
	})
	if err := sw.Flush(); err != nil {
		panic(err)
	}

	fmt.Printf("% x\n", buf.Bytes())
	// Output:
	// 00 07 69 64 00 07 00 00 00
}

func ExampleWriter_WriteTo() {
	w := bt.NewWriter()
	w.U32BE(1)
	w.CStr("ok")

	var buf bytes.Buffer
	n, err := w.WriteTo(&buf)
	fmt.Println(n, err, w.Len(), buf.Len())
	// Output:
	// 7 <nil> 0 7
}
