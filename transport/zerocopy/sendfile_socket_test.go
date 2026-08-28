//go:build darwin || windows

package zerocopy

import (
	"fmt"
	"io"
	"net"
	"os"
	"testing"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

func TestSendFileToTCPSocket(t *testing.T) {
	src, err := os.CreateTemp(t.TempDir(), "gnalloy-sendfile-socket-*")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.WriteString("0123456789"); err != nil {
		t.Fatal(err)
	}
	region, err := channel.NewFileRegion(src, 2, 5)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		tcpConn, ok := conn.(*net.TCPConn)
		if !ok {
			done <- fmt.Errorf("accepted conn=%T, want *net.TCPConn", conn)
			return
		}
		result, err := sendFileForTest(tcpConn, region)
		if err != nil {
			done <- err
			return
		}
		if !result.ZeroCopy || result.Bytes != 5 || region.Transferred() != 5 {
			done <- fmt.Errorf("result=%+v transferred=%d", result, region.Transferred())
			return
		}
		done <- nil
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(client)
	closeErr := client.Close()
	sendErr := <-done
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if sendErr != nil {
		t.Fatal(sendErr)
	}
	if string(data) != "23456" {
		t.Fatalf("data=%q", data)
	}
}

func sendFileForTest(conn *net.TCPConn, region channel.FileRegion) (Result, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return Result{}, err
	}
	sender, err := NewSender(1024)
	if err != nil {
		return Result{}, err
	}
	var total Result
	var sendErr error
	if err := raw.Control(func(fd uintptr) {
		for region.Transferred() < region.Count() {
			result, again, err := sender.SendFile(transport.FDRef{FD: int(fd)}, region)
			total.Bytes += result.Bytes
			total.ZeroCopy = total.ZeroCopy || result.ZeroCopy
			if err != nil {
				sendErr = err
				return
			}
			if again {
				sendErr = fmt.Errorf("sendfile returned again after %d bytes", total.Bytes)
				return
			}
		}
	}); err != nil {
		return total, err
	}
	return total, sendErr
}
