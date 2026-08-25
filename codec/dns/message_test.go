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
	if target, ok := msg.Answers[0].Target(); !ok || target != "alias.example.com" {
		t.Fatalf("target=%q ok=%v", target, ok)
	}
	if ip := msg.Answers[1].IP(); !ip.Equal(net.IPv4(1, 2, 3, 4)) {
		t.Fatalf("ip=%v", ip)
	}
}

func TestResourceHelpersRoundTripCommonRecords(t *testing.T) {
	mx, err := NewMXResource("example.com", 60, MXData{Preference: 10, Exchange: "mail.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	txt, err := NewTXTResource("example.com", 60, "v=spf1", "mx")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := NewSRVResource("_mqtt._tcp.example.com", 60, SRVData{Priority: 1, Weight: 2, Port: 1883, Target: "broker.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	msg := Message{ID: 1, Response: true, Answers: []Resource{mx, txt, srv}}
	wire, err := AppendMessage(nil, msg)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseMessage(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := decoded.Answers[0].MX(); !ok || got.Preference != 10 || got.Exchange != "mail.example.com" {
		t.Fatalf("mx=%+v ok=%v", got, ok)
	}
	if got, ok := decoded.Answers[1].TXT(); !ok || len(got) != 2 || got[0] != "v=spf1" || got[1] != "mx" {
		t.Fatalf("txt=%+v ok=%v", got, ok)
	}
	if got, ok := decoded.Answers[2].SRV(); !ok || got.Port != 1883 || got.Target != "broker.example.com" {
		t.Fatalf("srv=%+v ok=%v", got, ok)
	}
}

func TestDecodeMessageRejectsTrailingNameResourceData(t *testing.T) {
	tests := []struct {
		name  string
		rtype uint16
		rdata []byte
	}{
		{
			name:  "cname",
			rtype: TypeCNAME,
			rdata: []byte{0x00, 0xff},
		},
		{
			name:  "mx",
			rtype: TypeMX,
			rdata: []byte{0x00, 0x0a, 0x00, 0xff},
		},
		{
			name:  "srv",
			rtype: TypeSRV,
			rdata: []byte{0x00, 0x01, 0x00, 0x02, 0x07, 0x5b, 0x00, 0xff},
		},
		{
			name:  "soa",
			rtype: TypeSOA,
			rdata: []byte{
				0x00, 0x00,
				0x00, 0x00, 0x00, 0x01,
				0x00, 0x00, 0x00, 0x02,
				0x00, 0x00, 0x00, 0x03,
				0x00, 0x00, 0x00, 0x04,
				0x00, 0x00, 0x00, 0x05,
				0xff,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte{
				0x00, 0x01, 0x81, 0x80, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
				0x00,
				byte(tt.rtype >> 8), byte(tt.rtype), 0x00, 0x01,
				0x00, 0x00, 0x00, 0x3c,
				byte(len(tt.rdata) >> 8), byte(len(tt.rdata)),
			}
			data = append(data, tt.rdata...)
			_, err := ParseMessage(data)
			if !errors.Is(err, ErrInvalidResource) {
				t.Fatalf("err=%v, want %v", err, ErrInvalidResource)
			}
		})
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
