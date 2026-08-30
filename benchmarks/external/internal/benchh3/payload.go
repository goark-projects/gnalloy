package benchh3

import codechttp3 "goark.dev/gnalloy/codec/http3"

// ResponseBody 返回固定响应体，确保所有框架返回同样字节数。
func ResponseBody(size int) []byte {
	body := make([]byte, size)
	for i := range body {
		body[i] = byte(i)
	}
	return body
}

func requestHeaders(host string) codechttp3.HeadersBlock {
	if host == "" {
		host = "127.0.0.1"
	}
	return codechttp3.HeadersBlock{Fields: []codechttp3.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: host},
		{Name: ":path", Value: "/bench"},
	}}
}
