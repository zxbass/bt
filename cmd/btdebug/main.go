// Command btdebug is a small playground for poking at bt.Cursor.
//
// Usage:
//
//	go run ./cmd/btdebug -demo
//	go run ./cmd/btdebug -hex "42 02 01 01 02" -seq "u8,u16le,u16be"
//	printf '\x01\x02\x03\x04' | go run ./cmd/btdebug -seq "u32le"
//
// Supported ops:
//
//	u8, u16le, u16be, u32le, u32be, u64le, u64be
//	i8, i16le, i16be, i24le, i24be
//	u24le, u24be, f32le, f32be, f64le, f64be, uleb, sleb
//	str:N, rawstr:N, strunsafe:N, cstr:N
//	bytes:N, peek:N, skip:N, ensure:N, canread:N, pos, sub:N
//	hex
//
// The hex op dumps the remaining bytes without advancing the cursor.
package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/zxbass/bt"
)

type op struct {
	raw  string
	name string
	arg  int
}

var (
	noArgOps = map[string]bool{
		"u8": true, "u16le": true, "u16be": true, "u32le": true, "u32be": true,
		"u64le": true, "u64be": true,
		"i8": true, "i16le": true, "i16be": true, "i24le": true, "i24be": true,
		"u24le": true, "u24be": true, "f32le": true, "f32be": true,
		"f64le": true, "f64be": true, "uleb": true, "sleb": true, "pos": true, "hex": true,
	}
	sizeArgOps = map[string]bool{
		"str": true, "rawstr": true, "strunsafe": true, "cstr": true,
		"bytes": true, "peek": true, "skip": true, "ensure": true, "canread": true,
		"sub": true,
	}
)

func main() {
	var (
		hexInput = flag.String("hex", "", "input buffer as hex (spaces and 0x prefixes are ignored)")
		seq      = flag.String("seq", "u8,u16le,u16be,u32le,u32be,u64le,u24le,f32le,uleb,sleb,str:5,hex", "comma-separated reads")
		demo     = flag.Bool("demo", false, "use the built-in demo buffer")
	)
	flag.Parse()

	data, err := load(*hexInput, *demo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "btdebug:", err)
		os.Exit(1)
	}

	ops, err := parseSeq(*seq)
	if err != nil {
		fmt.Fprintln(os.Stderr, "btdebug:", err)
		os.Exit(1)
	}

	run(data, ops)
}

func load(hexInput string, demo bool) ([]byte, error) {
	switch {
	case hexInput != "":
		clean := strings.NewReplacer(" ", "", "\t", "", "\n", "", "0x", "", "0X", "", ",", "").Replace(hexInput)
		return hex.DecodeString(clean)
	case demo:
		return demoData(), nil
	}

	stat, err := os.Stdin.Stat()
	if err != nil {
		return nil, err
	}
	if stat.Mode()&os.ModeCharDevice == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		if len(data) == 0 {
			return nil, errors.New("stdin is empty (use -hex, -demo or pipe bytes)")
		}
		return data, nil
	}

	return demoData(), nil
}

func demoData() []byte {
	return []byte{
		0x42,       // u8
		0x02, 0x01, // u16le
		0x01, 0x02, // u16be
		0x78, 0x56, 0x34, 0x12, // u32le
		0x12, 0x34, 0x56, 0x78, // u32be
		0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11, // u64le
		0xFF, 0xFF, 0xFF, // u24le
		0xdb, 0x0f, 0x49, 0x40, // f32le (pi)
		0xAC, 0x02, // uleb (300)
		0x7F,                          // sleb (-1)
		'h', 'e', 'l', 'l', 'o', 0x00, // str:5
		0xde, 0xad, 0xbe, 0xef, // hex
	}
}

func parseSeq(s string) ([]op, error) {
	var ops []op
	for _, raw := range strings.Split(s, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		name, argStr, hasArg := strings.Cut(raw, ":")
		o := op{raw: raw, name: strings.ToLower(name)}

		switch {
		case sizeArgOps[o.name]:
			arg, err := strconv.Atoi(argStr)
			if !hasArg || err != nil || arg < 0 {
				return nil, fmt.Errorf("bad op %q: want %s:N with N >= 0", raw, o.name)
			}
			o.arg = arg
		case noArgOps[o.name]:
			if hasArg {
				return nil, fmt.Errorf("bad op %q: takes no argument", raw)
			}
		default:
			return nil, fmt.Errorf("unknown op %q", raw)
		}

		ops = append(ops, o)
	}
	return ops, nil
}

func run(data []byte, ops []op) {
	fmt.Printf("buffer: %d bytes %s\n", len(data), hex.EncodeToString(data))

	c := bt.NewCursor(data)
	for i, o := range ops {
		before := c.Pos()
		res, err := exec(c, o)

		if err != nil {
			fmt.Printf("%2d %-10s @%-3d ERROR: %v\n", i+1, o.raw, before, err)
			return
		}
		fmt.Printf("%2d %-10s @%-3d %-24s (left %d)\n", i+1, o.raw, c.Pos(), res, c.BytesLeft())
	}
}

func exec(c *bt.Cursor, o op) (res string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	switch o.name {
	case "u8":
		v := c.U8()
		res = fmt.Sprintf("%d (0x%02x)", v, v)
	case "u16le":
		v := c.U16LE()
		res = fmt.Sprintf("%d (0x%04x)", v, v)
	case "u16be":
		v := c.U16BE()
		res = fmt.Sprintf("%d (0x%04x)", v, v)
	case "u32le":
		v := c.U32LE()
		res = fmt.Sprintf("%d (0x%08x)", v, v)
	case "u32be":
		v := c.U32BE()
		res = fmt.Sprintf("%d (0x%08x)", v, v)
	case "u64le":
		v := c.U64LE()
		res = fmt.Sprintf("%d (0x%016x)", v, v)
	case "u64be":
		v := c.U64BE()
		res = fmt.Sprintf("%d (0x%016x)", v, v)
	case "i8":
		res = strconv.Itoa(int(c.I8()))
	case "i16le":
		res = strconv.Itoa(int(c.I16LE()))
	case "i16be":
		res = strconv.Itoa(int(c.I16BE()))
	case "i24le":
		res = strconv.Itoa(int(c.I24LE()))
	case "i24be":
		res = strconv.Itoa(int(c.I24BE()))
	case "u24le":
		v := c.U24LE()
		res = fmt.Sprintf("%d (0x%06x)", v, v)
	case "u24be":
		v := c.U24BE()
		res = fmt.Sprintf("%d (0x%06x)", v, v)
	case "f32le":
		res = strconv.FormatFloat(float64(c.F32LE()), 'g', -1, 32)
	case "f32be":
		res = strconv.FormatFloat(float64(c.F32BE()), 'g', -1, 32)
	case "f64le":
		res = strconv.FormatFloat(c.F64LE(), 'g', -1, 64)
	case "f64be":
		res = strconv.FormatFloat(c.F64BE(), 'g', -1, 64)
	case "uleb":
		v := c.ULEB128()
		res = fmt.Sprintf("%d (0x%x)", v, v)
	case "sleb":
		res = strconv.FormatInt(c.SLEB128(), 10)
	case "sub":
		sub := c.Sub(o.arg)
		res = fmt.Sprintf("sub-cursor %d bytes @%d", sub.BytesLeft(), sub.Pos())
	case "str":
		res = strconv.Quote(c.StrOrRest(o.arg))
	case "rawstr":
		res = strconv.Quote(c.RawStr(o.arg))
	case "strunsafe":
		res = strconv.Quote(c.StrUnsafe(o.arg))
	case "cstr":
		v, cerr := bt.CStr(c.Peek(o.arg))
		if cerr != nil {
			return "", cerr
		}
		c.Skip(o.arg)
		res = strconv.Quote(v)
	case "bytes":
		res = hex.EncodeToString(c.Bytes(o.arg))
	case "peek":
		res = hex.EncodeToString(c.Peek(o.arg))
	case "skip":
		c.Skip(o.arg)
		res = fmt.Sprintf("skipped %d", o.arg)
	case "ensure":
		c.Ensure(o.arg)
		res = "ok"
	case "canread":
		res = strconv.FormatBool(c.CanRead(o.arg))
	case "pos":
		res = strconv.Itoa(c.Pos())
	case "hex":
		res = hex.EncodeToString(c.Peek(c.BytesLeft()))
	}
	return res, nil
}
