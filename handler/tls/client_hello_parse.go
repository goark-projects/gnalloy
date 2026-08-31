package tls

const (
	tlsRecordHeaderLen               = 5
	tlsRecordTypeHandshake      byte = 22
	tlsHandshakeTypeClientHello byte = 1
	maxTLSRecordPayload              = 1 << 14
	maxClientHelloHandshake          = 1 << 20
)

const (
	tlsExtensionServerName        uint16 = 0
	tlsExtensionALPN              uint16 = 16
	tlsExtensionSupportedVersions uint16 = 43
)

// InspectClientHello 从 TLS 明文记录中解析 ClientHello 元数据。
func InspectClientHello(raw []byte) (ClientHello, bool, error) {
	handshake, complete, err := collectClientHelloHandshake(raw)
	if err != nil || !complete {
		return ClientHello{}, complete, err
	}
	hello, err := parseClientHelloHandshake(handshake)
	return hello, err == nil, err
}

func collectClientHelloHandshake(raw []byte) ([]byte, bool, error) {
	var (
		joined   []byte
		expected int
		pos      int
	)
	expected = -1
	for {
		if len(raw)-pos < tlsRecordHeaderLen {
			return nil, false, nil
		}
		if raw[pos] != tlsRecordTypeHandshake || !isTLSRecordVersion(raw[pos+1], raw[pos+2]) {
			return nil, false, ErrInvalidClientHello
		}
		recordLen := int(raw[pos+3])<<8 | int(raw[pos+4])
		if recordLen <= 0 || recordLen > maxTLSRecordPayload {
			return nil, false, ErrInvalidClientHello
		}
		pos += tlsRecordHeaderLen
		if len(raw)-pos < recordLen {
			return nil, false, nil
		}
		payload := raw[pos : pos+recordLen]
		pos += recordLen

		if joined == nil && len(payload) >= 4 {
			size, err := clientHelloHandshakeSize(payload[:4])
			if err != nil {
				return nil, false, err
			}
			if size <= len(payload) {
				return payload[:size], true, nil
			}
			joined = make([]byte, 0, min(size, len(payload)*2))
			expected = size
		}
		joined = append(joined, payload...)
		if expected < 0 && len(joined) >= 4 {
			size, err := clientHelloHandshakeSize(joined[:4])
			if err != nil {
				return nil, false, err
			}
			expected = size
			if cap(joined) < expected {
				next := make([]byte, len(joined), expected)
				copy(next, joined)
				joined = next
			}
		}
		if expected >= 0 && len(joined) >= expected {
			return joined[:expected], true, nil
		}
	}
}

func clientHelloHandshakeSize(header []byte) (int, error) {
	if len(header) < 4 || header[0] != tlsHandshakeTypeClientHello {
		return 0, ErrInvalidClientHello
	}
	bodyLen := int(header[1])<<16 | int(header[2])<<8 | int(header[3])
	size := 4 + bodyLen
	if size <= 4 || size > maxClientHelloHandshake {
		return 0, ErrInvalidClientHello
	}
	return size, nil
}

func parseClientHelloHandshake(data []byte) (ClientHello, error) {
	size, err := clientHelloHandshakeSize(data)
	if err != nil {
		return ClientHello{}, err
	}
	if len(data) < size {
		return ClientHello{}, ErrInvalidClientHello
	}
	return parseClientHelloBody(data[4:size])
}

func parseClientHelloBody(body []byte) (ClientHello, error) {
	r := byteReader{data: body}
	if !r.skip(2 + 32) {
		return ClientHello{}, ErrInvalidClientHello
	}
	sessionLen, ok := r.u8()
	if !ok || !r.skip(int(sessionLen)) {
		return ClientHello{}, ErrInvalidClientHello
	}
	cipherBytes, ok := r.u16()
	if !ok || cipherBytes == 0 || cipherBytes%2 != 0 || !r.has(int(cipherBytes)) {
		return ClientHello{}, ErrInvalidClientHello
	}
	hello := ClientHello{CipherSuites: make([]uint16, 0, int(cipherBytes)/2)}
	for i := 0; i < int(cipherBytes)/2; i++ {
		suite, _ := r.u16()
		hello.CipherSuites = append(hello.CipherSuites, suite)
	}
	compressionLen, ok := r.u8()
	if !ok || compressionLen == 0 || !r.skip(int(compressionLen)) {
		return ClientHello{}, ErrInvalidClientHello
	}
	if r.remaining() == 0 {
		return hello, nil
	}
	extensionsLen, ok := r.u16()
	if !ok || int(extensionsLen) != r.remaining() {
		return ClientHello{}, ErrInvalidClientHello
	}
	return parseClientHelloExtensions(r.tail(), hello)
}

func parseClientHelloExtensions(data []byte, hello ClientHello) (ClientHello, error) {
	r := byteReader{data: data}
	for r.remaining() > 0 {
		extType, ok := r.u16()
		if !ok {
			return ClientHello{}, ErrInvalidClientHello
		}
		extLen, ok := r.u16()
		if !ok {
			return ClientHello{}, ErrInvalidClientHello
		}
		extData, ok := r.take(int(extLen))
		if !ok {
			return ClientHello{}, ErrInvalidClientHello
		}
		var err error
		hello, err = parseClientHelloExtension(extType, extData, hello)
		if err != nil {
			return ClientHello{}, err
		}
	}
	return hello, nil
}

func parseClientHelloExtension(extType uint16, data []byte, hello ClientHello) (ClientHello, error) {
	switch extType {
	case tlsExtensionServerName:
		return parseServerNameExtension(data, hello)
	case tlsExtensionALPN:
		return parseALPNExtension(data, hello)
	case tlsExtensionSupportedVersions:
		return parseSupportedVersionsExtension(data, hello)
	default:
		return hello, nil
	}
}

func parseServerNameExtension(data []byte, hello ClientHello) (ClientHello, error) {
	r := byteReader{data: data}
	listLen, ok := r.u16()
	if !ok || int(listLen) != r.remaining() {
		return ClientHello{}, ErrInvalidClientHello
	}
	list, _ := r.take(int(listLen))
	names := byteReader{data: list}
	for names.remaining() > 0 {
		nameType, ok := names.u8()
		if !ok {
			return ClientHello{}, ErrInvalidClientHello
		}
		nameLen, ok := names.u16()
		if !ok {
			return ClientHello{}, ErrInvalidClientHello
		}
		name, ok := names.take(int(nameLen))
		if !ok || len(name) == 0 {
			return ClientHello{}, ErrInvalidClientHello
		}
		if nameType == 0 && hello.ServerName == "" {
			hello.ServerName = string(name)
		}
	}
	return hello, nil
}

func parseALPNExtension(data []byte, hello ClientHello) (ClientHello, error) {
	r := byteReader{data: data}
	listLen, ok := r.u16()
	if !ok || int(listLen) != r.remaining() {
		return ClientHello{}, ErrInvalidClientHello
	}
	list, _ := r.take(int(listLen))
	protocols := byteReader{data: list}
	for protocols.remaining() > 0 {
		size, ok := protocols.u8()
		if !ok || size == 0 {
			return ClientHello{}, ErrInvalidClientHello
		}
		protocol, ok := protocols.take(int(size))
		if !ok {
			return ClientHello{}, ErrInvalidClientHello
		}
		hello.ALPNProtocols = append(hello.ALPNProtocols, string(protocol))
	}
	return hello, nil
}

func parseSupportedVersionsExtension(data []byte, hello ClientHello) (ClientHello, error) {
	r := byteReader{data: data}
	listLen, ok := r.u8()
	if !ok || int(listLen) != r.remaining() || listLen%2 != 0 {
		return ClientHello{}, ErrInvalidClientHello
	}
	for r.remaining() > 0 {
		version, _ := r.u16()
		hello.SupportedVersions = append(hello.SupportedVersions, version)
	}
	return hello, nil
}

func isTLSRecordVersion(major byte, minor byte) bool {
	return major == 0x03 && minor <= 0x04
}
