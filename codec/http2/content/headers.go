package content

import (
	"strconv"
	"strings"

	"goark.dev/gnalloy/codec/http2"
)

func getHeader(fields []http2.HeaderField, name string) string {
	for i := range fields {
		if strings.EqualFold(fields[i].Name, name) {
			return fields[i].Value
		}
	}
	return ""
}

func removeHeaders(fields []http2.HeaderField, names ...string) []http2.HeaderField {
	if len(fields) == 0 || len(names) == 0 {
		return fields
	}
	out := fields[:0]
	for _, field := range fields {
		if !matchesAnyHeader(field.Name, names) {
			out = append(out, field)
		}
	}
	clear(fields[len(out):])
	return out
}

func setHeader(fields []http2.HeaderField, name string, value string) []http2.HeaderField {
	name = strings.ToLower(name)
	for i := range fields {
		if strings.EqualFold(fields[i].Name, name) {
			fields[i].Name = name
			fields[i].Value = value
			return fields
		}
	}
	return append(fields, http2.HeaderField{Name: name, Value: value})
}

func addHeaderToken(fields []http2.HeaderField, name string, token string) []http2.HeaderField {
	name = strings.ToLower(name)
	token = strings.ToLower(strings.TrimSpace(token))
	for i := range fields {
		if !strings.EqualFold(fields[i].Name, name) {
			continue
		}
		if containsToken(fields[i].Value, token) {
			fields[i].Name = name
			return fields
		}
		if strings.TrimSpace(fields[i].Value) == "" {
			fields[i].Value = token
		} else {
			fields[i].Value += ", " + token
		}
		fields[i].Name = name
		return fields
	}
	return append(fields, http2.HeaderField{Name: name, Value: token})
}

func cloneFields(fields []http2.HeaderField) []http2.HeaderField {
	if len(fields) == 0 {
		return nil
	}
	out := make([]http2.HeaderField, len(fields))
	copy(out, fields)
	return out
}

func contentLength(body bufferReadable) string {
	if body == nil {
		return "0"
	}
	return strconv.Itoa(body.ReadableBytes())
}

func statusCode(fields []http2.HeaderField) int {
	status := getHeader(fields, ":status")
	if status == "" {
		return 0
	}
	code, err := strconv.Atoi(status)
	if err != nil {
		return 0
	}
	return code
}

func responseCanHaveBody(fields []http2.HeaderField) bool {
	code := statusCode(fields)
	return code < 100 || (code >= 200 && code != 204 && code != 304)
}

func matchesAnyHeader(name string, names []string) bool {
	for _, candidate := range names {
		if strings.EqualFold(name, candidate) {
			return true
		}
	}
	return false
}

func containsToken(value string, token string) bool {
	for part := range strings.SplitSeq(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

type bufferReadable interface {
	ReadableBytes() int
}
