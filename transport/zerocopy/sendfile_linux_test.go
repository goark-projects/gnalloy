//go:build linux

package zerocopy

import (
	"io"
	"os"
	"testing"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

func TestSendFileToPipe(t *testing.T) {
	src, err := os.CreateTemp(t.TempDir(), "gnalloy-sendfile-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.WriteString("0123456789"); err != nil {
		t.Fatal(err)
	}
	region, err := channel.NewFileRegion(src, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()

	sender, err := NewSender(1024)
	if err != nil {
		t.Fatal(err)
	}
	result, again, err := sender.SendFile(transport.FDRef{FD: int(writer.Fd())}, region)
	if err != nil {
		t.Fatal(err)
	}
	if again || !result.ZeroCopy || result.Bytes != 5 || region.Transferred() != 5 {
		t.Fatalf("result=%+v again=%t transferred=%d", result, again, region.Transferred())
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "23456" {
		t.Fatalf("data=%q", data)
	}
}
