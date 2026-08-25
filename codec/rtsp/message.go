package rtsp

import (
	"strings"

	"goark.dev/gnalloy/buffer"
)

const Version10 = "RTSP/1.0"

type Method string

const (
	MethodOptions      Method = "OPTIONS"
	MethodDescribe     Method = "DESCRIBE"
	MethodAnnounce     Method = "ANNOUNCE"
	MethodSetup        Method = "SETUP"
	MethodPlay         Method = "PLAY"
	MethodPause        Method = "PAUSE"
	MethodTeardown     Method = "TEARDOWN"
	MethodGetParameter Method = "GET_PARAMETER"
	MethodSetParameter Method = "SET_PARAMETER"
	MethodRedirect     Method = "REDIRECT"
	MethodRecord       Method = "RECORD"
)

type Headers map[string]string

func (h Headers) Get(name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func (h Headers) Set(name string, value string) {
	h[name] = value
}

type Request struct {
	Method  Method
	URI     string
	Version string
	Headers Headers
	Body    buffer.ByteBuf
}

func (r Request) Release() {
	if r.Body != nil {
		r.Body.Release()
	}
}

type Response struct {
	Version    string
	StatusCode int
	Reason     string
	Headers    Headers
	Body       buffer.ByteBuf
}

func (r Response) Release() {
	if r.Body != nil {
		r.Body.Release()
	}
}
