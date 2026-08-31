package http1

import (
	"strconv"
	"strings"
)

func cloneHeaders(headers Headers) Headers {
	out := make(Headers, len(headers))
	for k, v := range headers {
		out[k] = v
	}
	return out
}

func setHeaderToken(headers Headers, name string, token string) Headers {
	headers = cloneHeaders(headers)
	value := headers.Get(name)
	if value == "" {
		headers.Set(name, token)
		return headers
	}
	if strings.TrimSpace(value) == "*" || headers.ContainsToken(name, token) {
		return headers
	}
	headers.Set(name, value+", "+token)
	return headers
}

func setKnownContentLength(headers Headers, size int) Headers {
	headers = cloneHeaders(headers)
	headers.Del("Transfer-Encoding")
	headers.Set("Content-Length", strconv.Itoa(size))
	return headers
}
