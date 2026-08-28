package multipart

import "errors"

var (
	// ErrInvalidContentType 表示 Content-Type 不是合法 multipart 类型。
	ErrInvalidContentType = errors.New("gnalloy/codec/http1/multipart: invalid content type")
	// ErrMissingBoundary 表示 multipart Content-Type 缺少 boundary 参数。
	ErrMissingBoundary = errors.New("gnalloy/codec/http1/multipart: missing boundary")
	// ErrMissingBody 表示请求或响应没有可解析的 multipart body。
	ErrMissingBody = errors.New("gnalloy/codec/http1/multipart: missing body")
	// ErrInvalidConfig 表示编码器或解码器配置非法。
	ErrInvalidConfig = errors.New("gnalloy/codec/http1/multipart: invalid config")
	// ErrLimitExceeded 表示 multipart part 数量、头部或正文超过配置预算。
	ErrLimitExceeded = errors.New("gnalloy/codec/http1/multipart: limit exceeded")
)
