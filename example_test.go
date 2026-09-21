package bt_test

import (
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
