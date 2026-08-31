package ascii

import "goark.dev/gnalloy/buffer"

// Command 表示 Memcached ASCII command。
type Command string

const (
	CommandGet      Command = "get"
	CommandGets     Command = "gets"
	CommandSet      Command = "set"
	CommandAdd      Command = "add"
	CommandReplace  Command = "replace"
	CommandAppend   Command = "append"
	CommandPrepend  Command = "prepend"
	CommandCAS      Command = "cas"
	CommandDelete   Command = "delete"
	CommandIncr     Command = "incr"
	CommandDecr     Command = "decr"
	CommandTouch    Command = "touch"
	CommandGAT      Command = "gat"
	CommandGATS     Command = "gats"
	CommandStats    Command = "stats"
	CommandVersion  Command = "version"
	CommandFlushAll Command = "flush_all"
	CommandQuit     Command = "quit"
)

// Status 表示 Memcached ASCII response status。
type Status string

const (
	StatusStored      Status = "STORED"
	StatusNotStored   Status = "NOT_STORED"
	StatusExists      Status = "EXISTS"
	StatusNotFound    Status = "NOT_FOUND"
	StatusDeleted     Status = "DELETED"
	StatusTouched     Status = "TOUCHED"
	StatusOK          Status = "OK"
	StatusEnd         Status = "END"
	StatusError       Status = "ERROR"
	StatusClientError Status = "CLIENT_ERROR"
	StatusServerError Status = "SERVER_ERROR"
	StatusVersion     Status = "VERSION"
)

// Request 表示一条 Memcached ASCII 请求。
type Request struct {
	Command   Command
	Key       string
	Keys      []string
	Flags     uint32
	Exptime   int64
	Bytes     int
	CAS       uint64
	Delta     uint64
	NoReply   bool
	Value     buffer.ByteBuf
	Arguments []string
}

// Release 释放请求 body。
func (r Request) Release() {
	if r.Value != nil {
		r.Value.Release()
	}
}

// Value 表示 VALUE 响应中的一项。
type Value struct {
	Key   string
	Flags uint32
	CAS   uint64
	Data  buffer.ByteBuf
}

// Release 释放 value body。
func (v Value) Release() {
	if v.Data != nil {
		v.Data.Release()
	}
}

// Response 表示一条 Memcached ASCII 响应。
type Response struct {
	Status  Status
	Message string
	Values  []Value
}

// Release 释放响应中所有 value body。
func (r Response) Release() {
	for i := range r.Values {
		r.Values[i].Release()
	}
}
