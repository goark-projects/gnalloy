package smtp

import (
	"strconv"
	"strings"

	"goark.dev/gnalloy/buffer"
)

type Command string

const (
	CommandHELO     Command = "HELO"
	CommandEHLO     Command = "EHLO"
	CommandMAIL     Command = "MAIL"
	CommandRCPT     Command = "RCPT"
	CommandDATA     Command = "DATA"
	CommandRSET     Command = "RSET"
	CommandVRFY     Command = "VRFY"
	CommandEXPN     Command = "EXPN"
	CommandHELP     Command = "HELP"
	CommandNOOP     Command = "NOOP"
	CommandQUIT     Command = "QUIT"
	CommandSTARTTLS Command = "STARTTLS"
	CommandAUTH     Command = "AUTH"
)

type Request struct {
	Command Command
	Params  []string
}

func NewRequest(command Command, params ...string) Request {
	return Request{Command: command, Params: params}
}

type Response struct {
	Code    int
	Details []string
}

func NewResponse(code int, details ...string) Response {
	return Response{Code: code, Details: details}
}

func (r Response) Valid() bool {
	return r.Code >= 100 && r.Code <= 999
}

type Data struct {
	Payload buffer.ByteBuf
	Last    bool
}

func NewData(payload buffer.ByteBuf) Data {
	return Data{Payload: payload}
}

func LastData(payload buffer.ByteBuf) Data {
	return Data{Payload: payload, Last: true}
}

func (d Data) Release() {
	if d.Payload != nil {
		d.Payload.Release()
	}
}

func commandLine(req Request) (string, error) {
	if req.Command == "" || strings.ContainsAny(string(req.Command), " \r\n") {
		return "", ErrInvalidRequest
	}
	var b strings.Builder
	b.WriteString(string(req.Command))
	for _, param := range req.Params {
		if strings.ContainsAny(param, "\r\n") {
			return "", ErrInvalidRequest
		}
		b.WriteByte(' ')
		b.WriteString(param)
	}
	b.WriteString("\r\n")
	return b.String(), nil
}

func responseLines(resp Response) ([]string, error) {
	if !resp.Valid() {
		return nil, ErrInvalidResponse
	}
	code := strconv.Itoa(resp.Code)
	details := resp.Details
	if len(details) == 0 {
		return []string{code + "\r\n"}, nil
	}
	lines := make([]string, 0, len(details))
	for i, detail := range details {
		if strings.ContainsAny(detail, "\r\n") {
			return nil, ErrInvalidResponse
		}
		sep := " "
		if i != len(details)-1 {
			sep = "-"
		}
		lines = append(lines, code+sep+detail+"\r\n")
	}
	return lines, nil
}
