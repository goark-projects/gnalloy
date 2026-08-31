package http1

import "testing"

func TestUpgradeHelpersBuildClientRequestAndServerResponse(t *testing.T) {
	req := NewClientUpgradeRequest("GET", "/chat", "websocket", Headers{"Host": "example.com"})
	if req.Method != "GET" || req.URI != "/chat" || req.Version != "HTTP/1.1" {
		t.Fatalf("request=%+v", req)
	}
	if req.Headers.Get("Upgrade") != "websocket" || !req.Headers.ContainsToken("Connection", "Upgrade") {
		t.Fatalf("request headers=%+v", req.Headers)
	}
	if !IsUpgradeRequest(req, "websocket") {
		t.Fatalf("request should be websocket upgrade: %+v", req)
	}

	resp := NewSwitchingProtocolsResponse("websocket", Headers{"Sec-WebSocket-Accept": "token"})
	if resp.StatusCode != 101 || resp.Reason != "Switching Protocols" {
		t.Fatalf("response=%+v", resp)
	}
	if resp.Headers.Get("Upgrade") != "websocket" || !resp.Headers.ContainsToken("Connection", "Upgrade") {
		t.Fatalf("response headers=%+v", resp.Headers)
	}
	if !IsSwitchingProtocolsResponse(resp, "websocket") {
		t.Fatalf("response should be websocket upgrade: %+v", resp)
	}
}

func TestUpgradeHelpersKeepExistingConnectionTokens(t *testing.T) {
	req := NewClientUpgradeRequest("", "/h2c", "h2c", Headers{"Connection": "keep-alive"})
	if !req.Headers.ContainsToken("Connection", "keep-alive") || !req.Headers.ContainsToken("Connection", "Upgrade") {
		t.Fatalf("connection=%q", req.Headers.Get("Connection"))
	}
}
