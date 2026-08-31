package http1

import (
	"net/url"
	"strings"

	"goark.dev/gnalloy/codec"
)

const defaultMaxQueryParams = 1024

// QueryParam 保留 query 参数的线协议顺序和重复 key。
type QueryParam struct {
	Name  string
	Value string
}

// QueryString 是 HTTP/1 URI 的 path、query 和 fragment 拆分结果。
type QueryString struct {
	Path     string
	Params   []QueryParam
	Fragment string
}

func DecodeQueryString(uri string, maxParams int) (QueryString, error) {
	if maxParams <= 0 {
		maxParams = defaultMaxQueryParams
	}
	withoutFragment, fragment, _ := strings.Cut(uri, "#")
	rawPath, rawQuery, hasQuery := strings.Cut(withoutFragment, "?")
	path, err := url.PathUnescape(rawPath)
	if err != nil {
		return QueryString{}, codec.ErrInvalidFrameLength
	}
	query := QueryString{Path: path, Fragment: fragment}
	if !hasQuery || rawQuery == "" {
		return query, nil
	}
	for rawQuery != "" {
		if len(query.Params) >= maxParams {
			return QueryString{}, codec.ErrFrameTooLong
		}
		var part string
		part, rawQuery, _ = strings.Cut(rawQuery, "&")
		name, value, hasValue := strings.Cut(part, "=")
		decodedName, err := url.QueryUnescape(name)
		if err != nil {
			return QueryString{}, codec.ErrInvalidFrameLength
		}
		decodedValue := ""
		if hasValue {
			decodedValue, err = url.QueryUnescape(value)
			if err != nil {
				return QueryString{}, codec.ErrInvalidFrameLength
			}
		}
		query.Params = append(query.Params, QueryParam{Name: decodedName, Value: decodedValue})
	}
	return query, nil
}

func (q QueryString) Values() url.Values {
	values := make(url.Values, len(q.Params))
	for _, param := range q.Params {
		values.Add(param.Name, param.Value)
	}
	return values
}

func AppendQueryString(dst []byte, path string, params []QueryParam) ([]byte, error) {
	if path == "" {
		path = "/"
	}
	dst = append(dst, path...)
	if len(params) == 0 {
		return dst, nil
	}
	dst = append(dst, '?')
	for i, param := range params {
		if i > 0 {
			dst = append(dst, '&')
		}
		dst = append(dst, url.QueryEscape(param.Name)...)
		dst = append(dst, '=')
		dst = append(dst, url.QueryEscape(param.Value)...)
	}
	return dst, nil
}
