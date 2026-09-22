package bt_test

import (
	"bytes"
	"errors"
	"fmt"

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
