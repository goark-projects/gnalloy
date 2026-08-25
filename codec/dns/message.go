package dns

import (
	"encoding/binary"
	"net"
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const (
	TypeA     uint16 = 1
	TypeNS    uint16 = 2
	TypeCNAME uint16 = 5
	TypeSOA   uint16 = 6
	TypePTR   uint16 = 12
	TypeMX    uint16 = 15
	TypeTXT   uint16 = 16
	TypeAAAA  uint16 = 28
	TypeSRV   uint16 = 33
	TypeOPT   uint16 = 41

	ClassIN uint16 = 1

	OpCodeQuery uint8 = 0

	RCodeNoError uint8 = 0
)

const (
	headerSize      = 12
	maxNameLength   = 255
	maxLabelLength  = 63
	namePointerMask = 0xc0
	maxPointerJumps = 128
)

// Message 是 DNS 线协议报文的 Go 化结构。
type Message struct {
	ID uint16

	Response           bool
	OpCode             uint8
	Authoritative      bool
	Truncated          bool
	RecursionDesired   bool
	RecursionAvailable bool
	ResponseCode       uint8
	Questions          []Question
	Answers            []Resource
	Authorities        []Resource
	Additionals        []Resource
}

type Question struct {
	Name  string
	Type  uint16
	Class uint16
}

type Resource struct {
	Name  string
	Type  uint16
	Class uint16
	TTL   uint32
	Data  []byte
}

type MXData struct {
	Preference uint16
	Exchange   string
}

type SRVData struct {
	Priority uint16
	Weight   uint16
	Port     uint16
	Target   string
}

type SOAData struct {
	MName   string
	RName   string
	Serial  uint32
	Refresh uint32
	Retry   uint32
	Expire  uint32
	Minimum uint32
}

func NewQuery(id uint16, name string, qtype uint16) Message {
	return Message{
		ID:               id,
		RecursionDesired: true,
		Questions: []Question{{
			Name:  name,
			Type:  qtype,
			Class: ClassIN,
		}},
	}
}

func (r Resource) IP() net.IP {
	switch r.Type {
	case TypeA:
		if len(r.Data) != net.IPv4len {
			return nil
		}
		return net.IPv4(r.Data[0], r.Data[1], r.Data[2], r.Data[3])
	case TypeAAAA:
		if len(r.Data) != net.IPv6len {
			return nil
		}
		ip := make(net.IP, net.IPv6len)
		copy(ip, r.Data)
		return ip
	default:
		return nil
	}
}

func (r Resource) Target() (string, bool) {
	switch r.Type {
	case TypeNS, TypeCNAME, TypePTR:
		name, n, err := readName(r.Data, 0)
		return name, err == nil && n == len(r.Data)
	default:
		return "", false
	}
}

func (r Resource) MX() (MXData, bool) {
	if r.Type != TypeMX || len(r.Data) < 3 {
		return MXData{}, false
	}
	name, n, err := readName(r.Data, 2)
	if err != nil || n+2 != len(r.Data) {
		return MXData{}, false
	}
	return MXData{Preference: binary.BigEndian.Uint16(r.Data[:2]), Exchange: name}, true
}

func (r Resource) TXT() ([]string, bool) {
	if r.Type != TypeTXT {
		return nil, false
	}
	var out []string
	for idx := 0; idx < len(r.Data); {
		n := int(r.Data[idx])
		idx++
		if n > len(r.Data)-idx {
			return nil, false
		}
		out = append(out, string(r.Data[idx:idx+n]))
		idx += n
	}
	return out, true
}

func (r Resource) SRV() (SRVData, bool) {
	if r.Type != TypeSRV || len(r.Data) < 7 {
		return SRVData{}, false
	}
	target, n, err := readName(r.Data, 6)
	if err != nil || n+6 != len(r.Data) {
		return SRVData{}, false
	}
	return SRVData{
		Priority: binary.BigEndian.Uint16(r.Data[0:2]),
		Weight:   binary.BigEndian.Uint16(r.Data[2:4]),
		Port:     binary.BigEndian.Uint16(r.Data[4:6]),
		Target:   target,
	}, true
}

func (r Resource) SOA() (SOAData, bool) {
	if r.Type != TypeSOA {
		return SOAData{}, false
	}
	mName, n, err := readName(r.Data, 0)
	if err != nil {
		return SOAData{}, false
	}
	rName, m, err := readName(r.Data, n)
	if err != nil {
		return SOAData{}, false
	}
	idx := n + m
	if len(r.Data)-idx != 20 {
		return SOAData{}, false
	}
	return SOAData{
		MName:   mName,
		RName:   rName,
		Serial:  binary.BigEndian.Uint32(r.Data[idx : idx+4]),
		Refresh: binary.BigEndian.Uint32(r.Data[idx+4 : idx+8]),
		Retry:   binary.BigEndian.Uint32(r.Data[idx+8 : idx+12]),
		Expire:  binary.BigEndian.Uint32(r.Data[idx+12 : idx+16]),
		Minimum: binary.BigEndian.Uint32(r.Data[idx+16 : idx+20]),
	}, true
}

func NewAResource(name string, ttl uint32, ip net.IP) (Resource, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return Resource{}, ErrInvalidResource
	}
	return Resource{Name: name, Type: TypeA, Class: ClassIN, TTL: ttl, Data: append([]byte(nil), ip4...)}, nil
}

func NewAAAAResource(name string, ttl uint32, ip net.IP) (Resource, error) {
	ip16 := ip.To16()
	if ip16 == nil || ip.To4() != nil {
		return Resource{}, ErrInvalidResource
	}
	return Resource{Name: name, Type: TypeAAAA, Class: ClassIN, TTL: ttl, Data: append([]byte(nil), ip16...)}, nil
}

func NewNameResource(name string, rtype uint16, ttl uint32, target string) (Resource, error) {
	if rtype != TypeNS && rtype != TypeCNAME && rtype != TypePTR {
		return Resource{}, ErrInvalidResource
	}
	data, err := appendName(nil, target)
	if err != nil {
		return Resource{}, err
	}
	return Resource{Name: name, Type: rtype, Class: ClassIN, TTL: ttl, Data: data}, nil
}

func NewMXResource(name string, ttl uint32, mx MXData) (Resource, error) {
	data := binary.BigEndian.AppendUint16(nil, mx.Preference)
	out, err := appendName(data, mx.Exchange)
	if err != nil {
		return Resource{}, err
	}
	return Resource{Name: name, Type: TypeMX, Class: ClassIN, TTL: ttl, Data: out}, nil
}

func NewTXTResource(name string, ttl uint32, values ...string) (Resource, error) {
	var data []byte
	for _, value := range values {
		if len(value) > 255 {
			return Resource{}, ErrInvalidResource
		}
		data = append(data, byte(len(value)))
		data = append(data, value...)
	}
	return Resource{Name: name, Type: TypeTXT, Class: ClassIN, TTL: ttl, Data: data}, nil
}

func NewSRVResource(name string, ttl uint32, srv SRVData) (Resource, error) {
	var data []byte
	data = binary.BigEndian.AppendUint16(data, srv.Priority)
	data = binary.BigEndian.AppendUint16(data, srv.Weight)
	data = binary.BigEndian.AppendUint16(data, srv.Port)
	out, err := appendName(data, srv.Target)
	if err != nil {
		return Resource{}, err
	}
	return Resource{Name: name, Type: TypeSRV, Class: ClassIN, TTL: ttl, Data: out}, nil
}

func ParseMessage(data []byte) (Message, error) {
	if len(data) < headerSize {
		return Message{}, ErrInvalidMessage
	}
	msg := Message{ID: binary.BigEndian.Uint16(data[0:2])}
	flags := binary.BigEndian.Uint16(data[2:4])
	msg.Response = flags&0x8000 != 0
	msg.OpCode = uint8((flags >> 11) & 0x0f)
	msg.Authoritative = flags&0x0400 != 0
	msg.Truncated = flags&0x0200 != 0
	msg.RecursionDesired = flags&0x0100 != 0
	msg.RecursionAvailable = flags&0x0080 != 0
	msg.ResponseCode = uint8(flags & 0x000f)

	qd := int(binary.BigEndian.Uint16(data[4:6]))
	an := int(binary.BigEndian.Uint16(data[6:8]))
	ns := int(binary.BigEndian.Uint16(data[8:10]))
	ar := int(binary.BigEndian.Uint16(data[10:12]))
	idx := headerSize

	var err error
	msg.Questions, idx, err = parseQuestions(data, idx, qd)
	if err != nil {
		return Message{}, err
	}
	msg.Answers, idx, err = parseResources(data, idx, an)
	if err != nil {
		return Message{}, err
	}
	msg.Authorities, idx, err = parseResources(data, idx, ns)
	if err != nil {
		return Message{}, err
	}
	msg.Additionals, _, err = parseResources(data, idx, ar)
	if err != nil {
		return Message{}, err
	}
	return msg, nil
}

func DecodeMessage(buf buffer.ByteBuf) (Message, error) {
	if buf == nil {
		return Message{}, ErrInvalidMessage
	}
	return ParseMessage(buf.Bytes())
}

func AppendMessage(dst []byte, msg Message) ([]byte, error) {
	if len(msg.Questions) > 0xffff || len(msg.Answers) > 0xffff || len(msg.Authorities) > 0xffff || len(msg.Additionals) > 0xffff {
		return nil, ErrInvalidMessage
	}
	var header [headerSize]byte
	binary.BigEndian.PutUint16(header[0:2], msg.ID)
	binary.BigEndian.PutUint16(header[2:4], messageFlags(msg))
	binary.BigEndian.PutUint16(header[4:6], uint16(len(msg.Questions)))
	binary.BigEndian.PutUint16(header[6:8], uint16(len(msg.Answers)))
	binary.BigEndian.PutUint16(header[8:10], uint16(len(msg.Authorities)))
	binary.BigEndian.PutUint16(header[10:12], uint16(len(msg.Additionals)))
	dst = append(dst, header[:]...)
	var err error
	for _, question := range msg.Questions {
		dst, err = appendQuestion(dst, question)
		if err != nil {
			return nil, err
		}
	}
	for _, resource := range msg.Answers {
		dst, err = appendResource(dst, resource)
		if err != nil {
			return nil, err
		}
	}
	for _, resource := range msg.Authorities {
		dst, err = appendResource(dst, resource)
		if err != nil {
			return nil, err
		}
	}
	for _, resource := range msg.Additionals {
		dst, err = appendResource(dst, resource)
		if err != nil {
			return nil, err
		}
	}
	return dst, nil
}

func EncodeMessage(alloc buffer.Allocator, msg Message) (buffer.ByteBuf, error) {
	if alloc == nil {
		return nil, ErrInvalidMessage
	}
	data, err := AppendMessage(nil, msg)
	if err != nil {
		return nil, err
	}
	out, err := alloc.Acquire(len(data))
	if err != nil {
		return nil, err
	}
	if _, err := out.WriteBytes(data); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

// MessageDecoder 把 ByteBuf 解码成 DNS Message。
type MessageDecoder struct{}

func NewMessageDecoder() *MessageDecoder {
	return &MessageDecoder{}
}

func (d *MessageDecoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	decoded, err := DecodeMessage(buf)
	buf.Release()
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(decoded)
}

// MessageEncoder 把 DNS Message 编码成 ByteBuf。
type MessageEncoder struct{}

func NewMessageEncoder() *MessageEncoder {
	return &MessageEncoder{}
}

func (e *MessageEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	dnsMsg, ok := msg.(Message)
	if !ok {
		return ctx.Write(msg)
	}
	out, err := EncodeMessage(ctx.Channel().Allocator(), dnsMsg)
	if err != nil {
		return err
	}
	if err := ctx.Write(out); err != nil {
		out.Release()
		return err
	}
	return nil
}

func parseQuestions(data []byte, idx int, count int) ([]Question, int, error) {
	questions := make([]Question, 0, count)
	for i := 0; i < count; i++ {
		name, n, err := readName(data, idx)
		if err != nil {
			return nil, 0, err
		}
		idx += n
		if len(data)-idx < 4 {
			return nil, 0, ErrInvalidQuestion
		}
		question := Question{
			Name:  name,
			Type:  binary.BigEndian.Uint16(data[idx : idx+2]),
			Class: binary.BigEndian.Uint16(data[idx+2 : idx+4]),
		}
		idx += 4
		questions = append(questions, question)
	}
	return questions, idx, nil
}

func parseResources(data []byte, idx int, count int) ([]Resource, int, error) {
	resources := make([]Resource, 0, count)
	for i := 0; i < count; i++ {
		name, n, err := readName(data, idx)
		if err != nil {
			return nil, 0, err
		}
		idx += n
		if len(data)-idx < 10 {
			return nil, 0, ErrInvalidResource
		}
		rdLen := int(binary.BigEndian.Uint16(data[idx+8 : idx+10]))
		if rdLen > len(data)-idx-10 {
			return nil, 0, ErrInvalidResource
		}
		resourceType := binary.BigEndian.Uint16(data[idx : idx+2])
		rdata, err := normalizeResourceData(data, resourceType, idx+10, rdLen)
		if err != nil {
			return nil, 0, err
		}
		resource := Resource{
			Name:  name,
			Type:  resourceType,
			Class: binary.BigEndian.Uint16(data[idx+2 : idx+4]),
			TTL:   binary.BigEndian.Uint32(data[idx+4 : idx+8]),
			Data:  rdata,
		}
		idx += 10 + rdLen
		resources = append(resources, resource)
	}
	return resources, idx, nil
}

func normalizeResourceData(packet []byte, rtype uint16, start int, length int) ([]byte, error) {
	switch rtype {
	case TypeNS, TypeCNAME, TypePTR:
		name, _, err := readName(packet, start)
		if err != nil {
			return nil, err
		}
		return appendName(nil, name)
	case TypeMX:
		if length < 3 {
			return nil, ErrInvalidResource
		}
		out := append([]byte(nil), packet[start:start+2]...)
		name, _, err := readName(packet, start+2)
		if err != nil {
			return nil, err
		}
		return appendName(out, name)
	case TypeSRV:
		if length < 7 {
			return nil, ErrInvalidResource
		}
		out := append([]byte(nil), packet[start:start+6]...)
		name, _, err := readName(packet, start+6)
		if err != nil {
			return nil, err
		}
		return appendName(out, name)
	case TypeSOA:
		mName, n, err := readName(packet, start)
		if err != nil {
			return nil, err
		}
		rName, m, err := readName(packet, start+n)
		if err != nil {
			return nil, err
		}
		numbers := start + n + m
		if numbers+20 > start+length {
			return nil, ErrInvalidResource
		}
		out, err := appendName(nil, mName)
		if err != nil {
			return nil, err
		}
		out, err = appendName(out, rName)
		if err != nil {
			return nil, err
		}
		return append(out, packet[numbers:numbers+20]...), nil
	default:
		return append([]byte(nil), packet[start:start+length]...), nil
	}
}

func appendQuestion(dst []byte, question Question) ([]byte, error) {
	if question.Type == 0 || question.Class == 0 {
		return nil, ErrInvalidQuestion
	}
	out, err := appendName(dst, question.Name)
	if err != nil {
		return nil, err
	}
	out = binary.BigEndian.AppendUint16(out, question.Type)
	out = binary.BigEndian.AppendUint16(out, question.Class)
	return out, nil
}

func appendResource(dst []byte, resource Resource) ([]byte, error) {
	if resource.Type == 0 || resource.Class == 0 || len(resource.Data) > 0xffff {
		return nil, ErrInvalidResource
	}
	out, err := appendName(dst, resource.Name)
	if err != nil {
		return nil, err
	}
	out = binary.BigEndian.AppendUint16(out, resource.Type)
	out = binary.BigEndian.AppendUint16(out, resource.Class)
	out = binary.BigEndian.AppendUint32(out, resource.TTL)
	out = binary.BigEndian.AppendUint16(out, uint16(len(resource.Data)))
	out = append(out, resource.Data...)
	return out, nil
}

func readName(data []byte, idx int) (string, int, error) {
	if idx < 0 || idx >= len(data) {
		return "", 0, ErrInvalidName
	}
	start := idx
	consumed := 0
	jumped := false
	labels := make([]string, 0, 4)
	visited := make(map[int]struct{}, 4)
	for jumps := 0; jumps <= maxPointerJumps; jumps++ {
		if idx >= len(data) {
			return "", 0, ErrInvalidName
		}
		length := int(data[idx])
		switch {
		case length&namePointerMask == namePointerMask:
			if idx+1 >= len(data) {
				return "", 0, ErrInvalidName
			}
			ptr := int(binary.BigEndian.Uint16(data[idx:idx+2]) & 0x3fff)
			if ptr >= len(data) {
				return "", 0, ErrInvalidName
			}
			if _, ok := visited[ptr]; ok {
				return "", 0, ErrInvalidName
			}
			visited[ptr] = struct{}{}
			if !jumped {
				consumed = idx + 2 - start
				jumped = true
			}
			idx = ptr
		case length&namePointerMask != 0:
			return "", 0, ErrInvalidName
		case length == 0:
			if !jumped {
				consumed = idx + 1 - start
			}
			if len(labels) == 0 {
				return ".", consumed, nil
			}
			return strings.Join(labels, "."), consumed, nil
		default:
			if length > maxLabelLength || idx+1+length > len(data) {
				return "", 0, ErrInvalidName
			}
			labels = append(labels, string(data[idx+1:idx+1+length]))
			idx += 1 + length
		}
	}
	return "", 0, ErrInvalidName
}

func appendName(dst []byte, name string) ([]byte, error) {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	if name == "" {
		return append(dst, 0), nil
	}
	total := 1
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > maxLabelLength {
			return nil, ErrInvalidName
		}
		total += 1 + len(label)
		if total > maxNameLength {
			return nil, ErrInvalidName
		}
		dst = append(dst, byte(len(label)))
		dst = append(dst, label...)
	}
	return append(dst, 0), nil
}

func messageFlags(msg Message) uint16 {
	var flags uint16
	if msg.Response {
		flags |= 0x8000
	}
	flags |= uint16(msg.OpCode&0x0f) << 11
	if msg.Authoritative {
		flags |= 0x0400
	}
	if msg.Truncated {
		flags |= 0x0200
	}
	if msg.RecursionDesired {
		flags |= 0x0100
	}
	if msg.RecursionAvailable {
		flags |= 0x0080
	}
	flags |= uint16(msg.ResponseCode & 0x0f)
	return flags
}
