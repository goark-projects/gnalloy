package zerocopy

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

func TestCopyWritesRegion(t *testing.T) {
	region, err := channel.NewFileRegion(strings.NewReader("0123456789"), 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	result, err := Copy(&out, region, 2)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "23456" || result.Bytes != 5 || result.ZeroCopy || region.Transferred() != 5 {
		t.Fatalf("out=%q result=%+v transferred=%d", out.String(), result, region.Transferred())
	}
}

func TestSendFileRejectsInvalidFD(t *testing.T) {
	region, err := channel.NewFileRegion(strings.NewReader("abc"), 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := NewSender(0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := sender.SendFile(transport.FDRef{FD: -1}, region); !errors.Is(err, transport.ErrInvalidFD) {
		t.Fatalf("err=%v, want invalid fd", err)
	}
}

func TestSendFileRequiresNativeFileRegion(t *testing.T) {
	region, err := channel.NewFileRegion(strings.NewReader("abc"), 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := NewSender(1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := sender.SendFile(transport.FDRef{FD: 1}, region); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err=%v, want unsupported", err)
	}
}

func TestNewSenderRejectsNegativeChunk(t *testing.T) {
	if _, err := NewSender(-1); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v, want invalid config", err)
	}
}
