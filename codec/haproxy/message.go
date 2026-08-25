package haproxy

const (
	defaultMaxHeaderLength = 16 + 65535
	maxV1HeaderLength      = 108
)

var v2Signature = [...]byte{0x0d, 0x0a, 0x0d, 0x0a, 0x00, 0x0d, 0x0a, 'Q', 'U', 'I', 'T', 0x0a}

type Version uint8

const (
	Version1 Version = 1
	Version2 Version = 2
)

type Command uint8

const (
	CommandLocal Command = 0x00
	CommandProxy Command = 0x01
)

type Protocol byte

const (
	ProtocolUnknown    Protocol = 0x00
	ProtocolTCP4       Protocol = 0x11
	ProtocolUDP4       Protocol = 0x12
	ProtocolTCP6       Protocol = 0x21
	ProtocolUDP6       Protocol = 0x22
	ProtocolUnixStream Protocol = 0x31
	ProtocolUnixDgram  Protocol = 0x32
)

type TLVType byte

const (
	TLVTypeALPN           TLVType = 0x01
	TLVTypeAuthority      TLVType = 0x02
	TLVTypeCRC32C         TLVType = 0x03
	TLVTypeNoOp           TLVType = 0x04
	TLVTypeUniqueID       TLVType = 0x05
	TLVTypeSSL            TLVType = 0x20
	TLVTypeSSLVersion     TLVType = 0x21
	TLVTypeSSLCN          TLVType = 0x22
	TLVTypeSSLCipher      TLVType = 0x23
	TLVTypeSSLSignature   TLVType = 0x24
	TLVTypeSSLKeyAlg      TLVType = 0x25
	TLVTypeNetNS          TLVType = 0x30
	TLVTypeAWSVPCEndpoint TLVType = 0xea
)

type TLV struct {
	Type  TLVType
	Value []byte
}

type Message struct {
	Version            Version
	Command            Command
	Protocol           Protocol
	SourceAddress      string
	DestinationAddress string
	SourcePort         uint16
	DestinationPort    uint16
	TLVs               []TLV
}

func (p Protocol) String() string {
	switch p {
	case ProtocolTCP4:
		return "TCP4"
	case ProtocolUDP4:
		return "UDP4"
	case ProtocolTCP6:
		return "TCP6"
	case ProtocolUDP6:
		return "UDP6"
	case ProtocolUnixStream:
		return "UNIX_STREAM"
	case ProtocolUnixDgram:
		return "UNIX_DGRAM"
	default:
		return "UNKNOWN"
	}
}

func protocolFromString(s string) (Protocol, bool) {
	switch s {
	case "TCP4":
		return ProtocolTCP4, true
	case "TCP6":
		return ProtocolTCP6, true
	case "UNKNOWN":
		return ProtocolUnknown, true
	default:
		return 0, false
	}
}
