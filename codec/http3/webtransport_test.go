package http3

import (
	"errors"
	"reflect"
	"testing"
)

func TestWebTransportRequiredSettingsByRole(t *testing.T) {
	server := RequiredWebTransportSettings(WebTransportRoleServer, WebTransportSettings{
		InitialMaxData:        65536,
		InitialMaxStreamsBidi: 16,
		InitialMaxStreamsUni:  8,
	})
	wantServer := []Setting{
		{ID: SettingWTEnabled, Value: 1},
		{ID: SettingEnableConnectProtocol, Value: 1},
		{ID: SettingH3Datagram, Value: 1},
		{ID: SettingWTInitialMaxData, Value: 65536},
		{ID: SettingWTInitialMaxStreamsBidi, Value: 16},
		{ID: SettingWTInitialMaxStreamsUni, Value: 8},
	}
	if !reflect.DeepEqual(server, wantServer) {
		t.Fatalf("server settings=%+v, want %+v", server, wantServer)
	}

	client := RequiredWebTransportSettings(WebTransportRoleClient, WebTransportSettings{})
	wantClient := []Setting{
		{ID: SettingWTEnabled, Value: 1},
		{ID: SettingH3Datagram, Value: 1},
	}
	if !reflect.DeepEqual(client, wantClient) {
		t.Fatalf("client settings=%+v, want %+v", client, wantClient)
	}
}

func TestValidateWebTransportPeerSettings(t *testing.T) {
	serverSettings := []Setting{
		{ID: SettingWTEnabled, Value: 1},
		{ID: SettingEnableConnectProtocol, Value: 1},
		{ID: SettingH3Datagram, Value: 1},
	}
	if err := ValidateWebTransportPeerSettings(WebTransportRoleClient, serverSettings); err != nil {
		t.Fatal(err)
	}

	err := ValidateWebTransportPeerSettings(WebTransportRoleClient, []Setting{
		{ID: SettingWTEnabled, Value: 1},
		{ID: SettingEnableConnectProtocol, Value: 1},
	})
	if !errors.Is(err, ErrMissingWebTransportSetting) {
		t.Fatalf("err=%v, want ErrMissingWebTransportSetting", err)
	}

	err = ValidateWebTransportPeerSettings(WebTransportRoleClient, []Setting{
		{ID: SettingWTEnabled, Value: 2},
		{ID: SettingEnableConnectProtocol, Value: 1},
		{ID: SettingH3Datagram, Value: 1},
	})
	if !errors.Is(err, ErrInvalidWebTransportSetting) {
		t.Fatalf("err=%v, want ErrInvalidWebTransportSetting", err)
	}
}

func TestWebTransportConnectRequestRoundTrip(t *testing.T) {
	req := WebTransportConnectRequest{
		Scheme:    "https",
		Authority: "example.com",
		Path:      "/wt",
		Origin:    "https://app.example.com",
		Headers:   []HeaderField{{Name: "x-trace", Value: "abc"}},
	}
	block, err := NewWebTransportConnectRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseWebTransportConnectRequest(block)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != req.Scheme || parsed.Authority != req.Authority || parsed.Path != req.Path || parsed.Origin != req.Origin {
		t.Fatalf("parsed=%+v, want %+v", parsed, req)
	}
	if len(parsed.Headers) != 1 || parsed.Headers[0].Name != "x-trace" || parsed.Headers[0].Value != "abc" {
		t.Fatalf("headers=%+v", parsed.Headers)
	}
}

func TestParseWebTransportConnectRequestRejectsInvalidHeaders(t *testing.T) {
	_, err := ParseWebTransportConnectRequest(HeadersBlock{Fields: []HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":protocol", Value: WebTransportProtocolH3},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.com"},
		{Name: ":path", Value: "/wt"},
	}})
	if !errors.Is(err, ErrInvalidWebTransportConnect) {
		t.Fatalf("err=%v, want ErrInvalidWebTransportConnect", err)
	}
}

func TestWebTransportResponseHelpers(t *testing.T) {
	resp := NewWebTransportConnectResponse(200, []HeaderField{{Name: "x-session", Value: "ok"}})
	if !IsWebTransportConnectSuccess(resp) {
		t.Fatalf("response should be successful: %+v", resp)
	}
	if len(resp.Fields) != 2 || resp.Fields[1].Name != "x-session" {
		t.Fatalf("response fields=%+v", resp.Fields)
	}
}
