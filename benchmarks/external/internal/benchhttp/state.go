package benchhttp

import "bytes"

// ServerState 保存一个服务端连接上的半包 HTTP 请求头。
type ServerState struct {
	pending []byte
}

// AppendAndCountRequests 追加新字节并返回完整 HTTP/1 请求数。
func (s *ServerState) AppendAndCountRequests(data []byte) int {
	if len(data) > 0 {
		s.pending = append(s.pending, data...)
	}
	count := 0
	for {
		idx := bytes.Index(s.pending, headerTerminator)
		if idx < 0 {
			return count
		}
		count++
		consumed := idx + len(headerTerminator)
		copy(s.pending, s.pending[consumed:])
		s.pending = s.pending[:len(s.pending)-consumed]
	}
}
