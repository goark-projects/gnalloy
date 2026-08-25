package dns

import (
	"errors"
	"net"
	"strings"
	"testing"
)

func TestMessageRoundTripQuery(t *testing.T) {
	query := NewQuery(0x1234, "example.com.", TypeA)
	data, err := AppendMessage(nil, query)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ID != query.ID || !decoded.RecursionDesired || len(decoded.Questions) != 1 {
		t.Fatalf("decoded=%+v", decoded)
	}
	question := decoded.Questions[0]
	if question.Name != "example.com" || question.Type != TypeA || question.Class != ClassIN {
		t.Fatalf("question=%+v", question)
	}
}

func TestDecodeMessageParsesCompressedResponse(t *testing.T) {
	data := []byte{
		0x12, 0x34, 0x81, 0x80, 0x00, 0x01, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00,
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0x03, 'c', 'o', 'm', 0x00,
		0x00, 0x01, 0x00, 0x01,
		0xc0, 0x0c, 0x00, 0x05, 0x00, 0x01, 0x00, 0x00, 0x00, 0x3c, 0x00, 0x08,
		0x05, 'a', 'l', 'i', 'a', 's', 0xc0, 0x0c,
		0xc0, 0x0c, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x3c, 0x00, 0x04,
		0x01, 0x02, 0x03, 0x04,
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	if !msg.Response || msg.ResponseCode != RCodeNoError || len(msg.Answers) != 2 {
		t.Fatalf("msg=%+v", msg)
	}
	if msg.Answers[0].Name != "example.com" || msg.Answers[0].Type != TypeCNAME {
		t.Fatalf("cname=%+v", msg.Answers[0])
	}
	if ip := msg.Answers[1].IP(); !ip.Equal(net.IPv4(1, 2, 3, 4)) {
		t.Fatalf("ip=%v", ip)
	}
}

func TestDecodeMessageRejectsPointerLoop(t *testing.T) {
	data := []byte{
		0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xc0, 0x0c, 0x00, 0x01, 0x00, 0x01,
	}
	_, err := ParseMessage(data)
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidName)
	}
}

func TestAppendMessageRejectsTooLongName(t *testing.T) {
	name := strings.Repeat("a.", 128) + "a"
	_, err := AppendMessage(nil, NewQuery(1, name, TypeA))
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidName)
	}
}
