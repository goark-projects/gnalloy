package multipart

import (
	"fmt"
	"mime"
	"strings"
)

// ParseBoundary 从 multipart Content-Type 中提取 boundary。
func ParseBoundary(contentType string) (string, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidContentType, err)
	}
	if !strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		return "", ErrInvalidContentType
	}
	boundary := params["boundary"]
	if boundary == "" {
		return "", ErrMissingBoundary
	}
	return boundary, nil
}

// ContentType 返回带 boundary 的 multipart/form-data Content-Type。
func ContentType(boundary string) (string, error) {
	if boundary == "" {
		return "", ErrMissingBoundary
	}
	return mime.FormatMediaType("multipart/form-data", map[string]string{"boundary": boundary}), nil
}
