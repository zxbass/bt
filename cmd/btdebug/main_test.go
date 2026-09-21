package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/zxbass/bt"
)

func TestParseSeq(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []op
	}{
		{"simple", "u8,u16le", []op{{raw: "u8", name: "u8"}, {raw: "u16le", name: "u16le"}}},
		{"spaces and empties", " u8 , ,str:4 ", []op{{raw: "u8", name: "u8"}, {raw: "str:4", name: "str", arg: 4}}},
		{"uppercase", "U32BE", []op{{raw: "U32BE", name: "u32be"}}},
		{"size ops", "sub:2,skip:0", []op{{raw: "sub:2", name: "sub", arg: 2}, {raw: "skip:0", name: "skip"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSeq(tt.in)
			if err != nil {
				t.Fatalf("parseSeq(%q) error: %v", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseSeq(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseSeqErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"unknown op", "nope"},
		{"arg on no-arg op", "u8:1"},
		{"missing arg", "str"},
		{"non-numeric arg", "str:x"},
		{"negative arg", "str:-1"},
		{"missing sub arg", "sub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseSeq(tt.in); err == nil {
				t.Fatalf("parseSeq(%q) succeeded, want error", tt.in)
			}
		})
	}
}

func TestLoadHex(t *testing.T) {
	got, err := load("01 02 0x03,04", false)
	if err != nil {
		t.Fatalf("load hex: %v", err)
	}
	if want := []byte{1, 2, 3, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("load hex = %v, want %v", got, want)
	}

	if _, err := load("zz", false); err == nil {
		t.Fatal("load invalid hex succeeded, want error")
	}
}

func TestLoadDemo(t *testing.T) {
	got, err := load("", true)
	if err != nil {
		t.Fatalf("load demo: %v", err)
	}
	if !reflect.DeepEqual(got, demoData()) {
		t.Fatal("load demo did not return demoData")
	}
}

func TestLoadEmptyStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()

	old := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = old
		r.Close()
	}()

	_, err = load("", false)
	if err == nil {
		t.Fatal("load with empty stdin succeeded, want error")
	}
	if !strings.Contains(err.Error(), "stdin is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExec(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		op   op
		want string
	}{
		{"u8", []byte{0x42}, op{name: "u8"}, "66 (0x42)"},
		{"u16le", []byte{0x02, 0x01}, op{name: "u16le"}, "258 (0x0102)"},
		{"sleb", []byte{0x7F}, op{name: "sleb"}, "-1"},
		{"uleb", []byte{0xAC, 0x02}, op{name: "uleb"}, "300 (0x12c)"},
		{"pos", []byte{1}, op{name: "pos"}, "0"},
		{"canread true", []byte{1, 2}, op{name: "canread", arg: 2}, "true"},
		{"canread false", []byte{1}, op{name: "canread", arg: 2}, "false"},
		{"hex", []byte{0xde, 0xad}, op{name: "hex"}, "dead"},
		{"sub", []byte{1, 2, 3}, op{name: "sub", arg: 2}, "sub-cursor 2 bytes @0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := bt.NewCursor(tt.data)
			got, err := exec(c, tt.op)
			if err != nil {
				t.Fatalf("exec(%s) error: %v", tt.op.raw, err)
			}
			if got != tt.want {
				t.Fatalf("exec(%s) = %q, want %q", tt.op.raw, got, tt.want)
			}
		})
	}
}

func TestExecError(t *testing.T) {
	c := bt.NewCursor([]byte{1, 2})
	if _, err := exec(c, op{name: "u32le"}); err == nil {
		t.Fatal("exec(u32le) on 2 bytes succeeded, want panic-derived error")
	}
	if got := c.Pos(); got != 0 {
		t.Fatalf("Pos() after failed exec = %d, want 0", got)
	}
}
