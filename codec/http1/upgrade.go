package http1

import "strings"

// NewClientUpgradeRequest 创建 HTTP/1.1 Upgrade 请求对象。
func NewClientUpgradeRequest(method string, uri string, protocol string, headers Headers) Request {
	if method == "" {
		method = "GET"
	}
	if uri == "" {
		uri = "/"
	}
	headers = setHeaderToken(headers, "Connection", "Upgrade")
	headers.Set("Upgrade", strings.TrimSpace(protocol))
	return Request{Method: method, URI: uri, Version: "HTTP/1.1", Headers: headers}
}

// NewSwitchingProtocolsResponse 创建 HTTP/1.1 101 Upgrade 响应对象。
func NewSwitchingProtocolsResponse(protocol string, headers Headers) Response {
	headers = setHeaderToken(headers, "Connection", "Upgrade")
	headers.Set("Upgrade", strings.TrimSpace(protocol))
	return Response{Version: "HTTP/1.1", StatusCode: 101, Reason: "Switching Protocols", Headers: headers}
}

func IsUpgradeRequest(req Request, protocol string) bool {
	return req.Headers.ContainsToken("Connection", "Upgrade") && upgradeProtocolMatches(req.Headers, protocol)
}

func IsSwitchingProtocolsResponse(resp Response, protocol string) bool {
	return resp.StatusCode == 101 && resp.Headers.ContainsToken("Connection", "Upgrade") && upgradeProtocolMatches(resp.Headers, protocol)
}

func upgradeProtocolMatches(headers Headers, protocol string) bool {
	actual := strings.TrimSpace(headers.Get("Upgrade"))
	if protocol == "" {
		return actual != ""
	}
	return strings.EqualFold(actual, strings.TrimSpace(protocol))
}
