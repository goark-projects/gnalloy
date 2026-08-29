package benchhttp

const (
	// ProtocolHTTP1 是外部对标 harness 使用的 HTTP/1.1 明文协议名。
	ProtocolHTTP1 = "http1"
)

var headerTerminator = []byte("\r\n\r\n")
