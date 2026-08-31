package http1bridge

import (
	"testing"

	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/http2"
)

func TestRequestFromHeadersBlockMapsPseudoHeaders(t *testing.T) {
	req, err := RequestFromHeadersBlock(http2.HeadersBlock{
		StreamID: 1,
		Fields: []http2.HeaderField{
			{Name: ":method", Value: "POST"},
			{Name: ":scheme", Value: "https"},
			{Name: ":authority", Value: "example.test"},
			{Name: ":path", Value: "/items?a=1"},
			{Name: "content-type", Value: "text/plain"},
		},
		EndStream: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if req.Method != "POST" || req.URI != "/items?a=1" || req.Version != "HTTP/1.1" {
		t.Fatalf("request=%+v", req)
	}
	if req.Headers.Get("Host") != "example.test" || req.Headers.Get("Content-Type") != "text/plain" {
		t.Fatalf("headers=%+v", req.Headers)
	}
}

func TestHeadersBlockFromRequestBuildsPseudoHeadersFirst(t *testing.T) {
	block := HeadersBlockFromRequest(3, http1.Request{
		Method: "GET",
		URI:    "/q",
		Headers: http1.Headers{
			"Host":       "example.test",
			"User-Agent": "gnalloy",
			"Connection": "keep-alive",
		},
	}, true)

	want := []http2.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.test"},
		{Name: ":path", Value: "/q"},
		{Name: "user-agent", Value: "gnalloy"},
	}
	if !equalFields(block.Fields, want) || block.StreamID != 3 || !block.EndStream {
		t.Fatalf("block=%+v want fields=%+v", block, want)
	}
}

func TestResponseBridgeMapsStatus(t *testing.T) {
	resp, err := ResponseFromHeadersBlock(http2.HeadersBlock{
		StreamID: 1,
		Fields: []http2.HeaderField{
			{Name: ":status", Value: "204"},
			{Name: "server", Value: "gnalloy"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 204 || resp.Version != "HTTP/1.1" || resp.Headers.Get("Server") != "gnalloy" {
		t.Fatalf("response=%+v", resp)
	}

	block := HeadersBlockFromResponse(1, http1.Response{
		StatusCode: 200,
		Headers:    http1.Headers{"Server": "gnalloy"},
	}, false)
	want := []http2.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "server", Value: "gnalloy"},
	}
	if !equalFields(block.Fields, want) {
		t.Fatalf("fields=%+v, want %+v", block.Fields, want)
	}
}

func equalFields(got []http2.HeaderField, want []http2.HeaderField) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i].Name != want[i].Name || got[i].Value != want[i].Value || got[i].Sensitive != want[i].Sensitive {
			return false
		}
	}
	return true
}
