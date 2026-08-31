package http3

import (
	"context"
	"io"
	"testing"

	"goark.dev/gnalloy/buffer"
	codechttp3 "goark.dev/gnalloy/codec/http3"
)

func TestSessionStatsTrackStreamsAndBytes(t *testing.T) {
	conn := newFakeConnection()
	session, err := NewSession(conn, Config{})
	if err != nil {
		t.Fatal(err)
	}
	streamCh, err := session.OpenRequestStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := streamCh.Channel().Write(codechttp3.DataFrame{Data: testHTTP3TransportBuf("abc")}); err != nil {
		t.Fatal(err)
	}
	conn.openedBidi.feed([]byte{byte(codechttp3.FrameData), 3, 'x', 'y', 'z'})
	if _, err := streamCh.ReadOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := streamCh.Close(); err != nil {
		t.Fatal(err)
	}

	stats := session.Stats()
	if stats.OpenedStreams != 1 || stats.AcceptedStreams != 0 || stats.ActiveStreams != 0 || stats.ClosedStreams != 1 {
		t.Fatalf("lifecycle stats=%+v", stats)
	}
	if stats.StreamsByKind[StreamKindRequest] != 1 {
		t.Fatalf("kind stats=%+v", stats.StreamsByKind)
	}
	if stats.BytesRead != 5 || stats.BytesWritten == 0 {
		t.Fatalf("byte stats=%+v", stats)
	}
}

func TestSessionStatsTrackAcceptedStreams(t *testing.T) {
	conn := newFakeConnection()
	conn.acceptedUni.feed([]byte{byte(codechttp3.StreamTypeControl), byte(codechttp3.FrameSettings), 0})
	session, err := NewSession(conn, Config{})
	if err != nil {
		t.Fatal(err)
	}
	streamCh, err := session.AcceptRemoteControlStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := streamCh.ReadOnce(context.Background()); err != nil && err != io.EOF {
		t.Fatal(err)
	}

	stats := session.Stats()
	if stats.AcceptedStreams != 1 || stats.StreamsByKind[StreamKindRemoteControl] != 1 {
		t.Fatalf("stats=%+v", stats)
	}
}

func testHTTP3TransportBuf(data string) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes([]byte(data))
	return buf
}
