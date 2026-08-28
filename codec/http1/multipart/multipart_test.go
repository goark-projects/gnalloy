package multipart

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/codec/http1"
)

func TestParseBoundary(t *testing.T) {
	boundary, err := ParseBoundary(`multipart/form-data; boundary="goark-boundary"`)
	if err != nil {
		t.Fatal(err)
	}
	if boundary != "goark-boundary" {
		t.Fatalf("boundary=%q", boundary)
	}
	if _, err := ParseBoundary("text/plain"); !errors.Is(err, ErrInvalidContentType) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidContentType)
	}
	if _, err := ParseBoundary("multipart/form-data"); !errors.Is(err, ErrMissingBoundary) {
		t.Fatalf("err=%v, want %v", err, ErrMissingBoundary)
	}
}

func TestEncoderDecoderRoundTrip(t *testing.T) {
	var body bytes.Buffer
	encoder, err := NewEncoderWithBoundary(&body, "goark-boundary")
	if err != nil {
		t.Fatal(err)
	}
	if err := encoder.WriteField("name", "gnalloy"); err != nil {
		t.Fatal(err)
	}
	n, err := encoder.WritePart(PartInfo{
		Headers: FormDataHeader("file", "hello.txt", "text/plain"),
	}, strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("n=%d, want 5", n)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}

	decoder, err := NewDecoderFromContentType(encoder.ContentType(), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	parts, err := decoder.DecodeBytes(body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 {
		t.Fatalf("parts=%d, want 2", len(parts))
	}
	if parts[0].Name != "name" || parts[0].FileName != "" || string(parts[0].Data) != "gnalloy" {
		t.Fatalf("field=%+v", parts[0])
	}
	if parts[1].Name != "file" || parts[1].FileName != "hello.txt" || string(parts[1].Data) != "hello" || !parts[1].IsFile() {
		t.Fatalf("file=%+v", parts[1])
	}
}

func TestDecodeRequestUsesHTTPContentType(t *testing.T) {
	var body bytes.Buffer
	encoder, err := NewEncoderWithBoundary(&body, "req-boundary")
	if err != nil {
		t.Fatal(err)
	}
	if err := encoder.WriteField("k", "v"); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	buf := buffer.NewHeapBuffer(body.Len())
	defer buf.Release()
	if _, err := buf.WriteBytes(body.Bytes()); err != nil {
		t.Fatal(err)
	}
	parts, err := DecodeRequest(http1.Request{
		Headers: http1.Headers{"Content-Type": encoder.ContentType()},
		Body:    buf,
	}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 || parts[0].Name != "k" || string(parts[0].Data) != "v" {
		t.Fatalf("parts=%+v", parts)
	}
}

func TestStreamDecoderDrainsUnreadPart(t *testing.T) {
	var body bytes.Buffer
	encoder, err := NewEncoderWithBoundary(&body, "stream-boundary")
	if err != nil {
		t.Fatal(err)
	}
	if err := encoder.WriteField("first", "unread"); err != nil {
		t.Fatal(err)
	}
	if err := encoder.WriteField("second", "read"); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder("stream-boundary", Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	var second []byte
	err = decoder.Decode(bytes.NewReader(body.Bytes()), PartHandlerFunc(func(info PartInfo, reader io.Reader) error {
		names = append(names, info.Name)
		if info.Name != "second" {
			return nil
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		second = data
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "first,second" || string(second) != "read" {
		t.Fatalf("names=%v second=%q", names, second)
	}
}

func TestDecodeRejectsPartLimit(t *testing.T) {
	var body bytes.Buffer
	encoder, err := NewEncoderWithBoundary(&body, "limit-boundary")
	if err != nil {
		t.Fatal(err)
	}
	if err := encoder.WriteField("a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := encoder.WriteField("b", "2"); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder("limit-boundary", Limits{MaxParts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.DecodeBytes(body.Bytes()); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err=%v, want %v", err, ErrLimitExceeded)
	}
}

func TestDecodeRejectsBodyLimits(t *testing.T) {
	var body bytes.Buffer
	encoder, err := NewEncoderWithBoundary(&body, "body-limit-boundary")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.WritePart(PartInfo{Headers: FormDataHeader("file", "a.bin", "")}, strings.NewReader("12345")); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder("body-limit-boundary", Limits{MaxPartBytes: 4, MaxTotalBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(bytes.NewReader(body.Bytes()), PartHandlerFunc(func(_ PartInfo, reader io.Reader) error {
		_, err := io.Copy(io.Discard, reader)
		return err
	})); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err=%v, want %v", err, ErrLimitExceeded)
	}
}

func TestDecodeRejectsHeaderLimit(t *testing.T) {
	var body bytes.Buffer
	encoder, err := NewEncoderWithBoundary(&body, "header-limit-boundary")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.WritePart(PartInfo{Headers: FormDataHeader("very-long-name", "", "")}, strings.NewReader("ok")); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder("header-limit-boundary", Limits{MaxHeaderBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.DecodeBytes(body.Bytes()); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err=%v, want %v", err, ErrLimitExceeded)
	}
}
