package quic

import (
	"fmt"

	nativequic "github.com/quic-go/quic-go"
)

// ClientToken 是客户端收到的 QUIC NEW_TOKEN。
type ClientToken = nativequic.ClientToken

// ClientTokenStore 保存客户端 NEW_TOKEN，供后续连接跳过地址验证。
type ClientTokenStore = nativequic.TokenStore

// NewClientTokenStore 创建并发安全的 LRU 客户端 token store。
func NewClientTokenStore(maxOrigins int, tokensPerOrigin int) (ClientTokenStore, error) {
	if maxOrigins <= 0 {
		return nil, fmt.Errorf("%w: client token store origins must be positive", ErrInvalidConfig)
	}
	if tokensPerOrigin <= 0 {
		return nil, fmt.Errorf("%w: client token store tokens must be positive", ErrInvalidConfig)
	}
	return nativequic.NewLRUTokenStore(maxOrigins, tokensPerOrigin), nil
}
