package unix

import (
	"errors"
	"strings"
	"testing"
)

func TestParseAddressAcceptsPathAndUnixScheme(t *testing.T) {
	addr, err := ParseAddress("unix:///tmp/gnalloy.sock")
	if err != nil {
		t.Fatal(err)
	}
	if addr.Path != "/tmp/gnalloy.sock" || addr.Abstract {
		t.Fatalf("addr=%+v", addr)
	}
	if addr.String() != "unix:///tmp/gnalloy.sock" {
		t.Fatalf("string=%q", addr.String())
	}
}

func TestParseAddressAcceptsAbstractAddress(t *testing.T) {
	addr, err := ParseAddress("@gnalloy")
	if err != nil {
		t.Fatal(err)
	}
	if !addr.Abstract || addr.Path != "gnalloy" || addr.sockaddrName() != "\x00gnalloy" {
		t.Fatalf("addr=%+v name=%q", addr, addr.sockaddrName())
	}
}

func TestParseAddressRejectsInvalidInput(t *testing.T) {
	if _, err := ParseAddress(""); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err=%v, want ErrInvalidAddress", err)
	}
	if _, err := ParseAddress("@"); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err=%v, want ErrInvalidAddress", err)
	}
	if _, err := ParseAddress(strings.Repeat("x", maxSocketPathLength+1)); !errors.Is(err, ErrPathTooLong) {
		t.Fatalf("err=%v, want ErrPathTooLong", err)
	}
}
