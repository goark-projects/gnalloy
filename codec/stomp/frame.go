package stomp

import (
	"strconv"
	"strings"

	"goark.dev/gnalloy/buffer"
)

type Command string

const (
	CommandConnect     Command = "CONNECT"
	CommandStomp       Command = "STOMP"
	CommandConnected   Command = "CONNECTED"
	CommandSend        Command = "SEND"
	CommandSubscribe   Command = "SUBSCRIBE"
	CommandUnsubscribe Command = "UNSUBSCRIBE"
	CommandAck         Command = "ACK"
	CommandNack        Command = "NACK"
	CommandBegin       Command = "BEGIN"
	CommandCommit      Command = "COMMIT"
	CommandAbort       Command = "ABORT"
	CommandDisconnect  Command = "DISCONNECT"
	CommandMessage     Command = "MESSAGE"
	CommandReceipt     Command = "RECEIPT"
	CommandError       Command = "ERROR"
)

type Header struct {
	Name  string
	Value string
}

// Headers 保留线缆顺序，并允许同名 header 重复出现。
type Headers []Header

func (h Headers) Get(name string) string {
	for _, header := range h {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}
	return ""
}

func (h Headers) Values(name string) []string {
	var values []string
	for _, header := range h {
		if strings.EqualFold(header.Name, name) {
			values = append(values, header.Value)
		}
	}
	return values
}

func (h *Headers) Add(name string, value string) {
	*h = append(*h, Header{Name: name, Value: value})
}

func (h *Headers) Set(name string, value string) {
	for i := range *h {
		if strings.EqualFold((*h)[i].Name, name) {
			(*h)[i].Value = value
			return
		}
	}
	h.Add(name, value)
}

func (h Headers) Has(name string) bool {
	for _, header := range h {
		if strings.EqualFold(header.Name, name) {
			return true
		}
	}
	return false
}

func (h Headers) ContentLength() (int, bool, error) {
	value := h.Get("content-length")
	if value == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return 0, false, ErrInvalidHeader
	}
	return n, true, nil
}

type Frame struct {
	Command   Command
	Headers   Headers
	Body      buffer.ByteBuf
	Heartbeat bool
}

func Heartbeat() Frame {
	return Frame{Heartbeat: true}
}

func NewFrame(command Command, headers Headers, body buffer.ByteBuf) Frame {
	return Frame{Command: command, Headers: headers, Body: body}
}

func (f Frame) Release() {
	if f.Body != nil {
		f.Body.Release()
	}
}
