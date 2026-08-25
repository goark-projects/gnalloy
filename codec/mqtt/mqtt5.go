package mqtt

// ProtocolVersion 描述 MQTT 协议版本。
type ProtocolVersion byte

const (
	ProtocolVersion311 ProtocolVersion = mqttProtocolLevel31
	ProtocolVersion5   ProtocolVersion = mqttProtocolLevel5
)

func (v ProtocolVersion) Byte() byte {
	return byte(v)
}

// QoS 描述 MQTT 服务质量等级。
type QoS byte

const (
	QoSAtMostOnce  QoS = 0
	QoSAtLeastOnce QoS = 1
	QoSExactlyOnce QoS = 2
)

func (q QoS) Byte() byte {
	return byte(q)
}

// ReasonCode 描述 MQTT 5 原因码。
type ReasonCode byte

const (
	ReasonSuccess                 ReasonCode = 0x00
	ReasonGrantedQoS0             ReasonCode = 0x00
	ReasonGrantedQoS1             ReasonCode = 0x01
	ReasonGrantedQoS2             ReasonCode = 0x02
	ReasonNoSubscriptionExisted   ReasonCode = 0x11
	ReasonUnspecifiedError        ReasonCode = 0x80
	ReasonMalformedPacket         ReasonCode = 0x81
	ReasonProtocolError           ReasonCode = 0x82
	ReasonImplementationSpecific  ReasonCode = 0x83
	ReasonUnsupportedProtocol     ReasonCode = 0x84
	ReasonClientIdentifierInvalid ReasonCode = 0x85
	ReasonBadUsernameOrPassword   ReasonCode = 0x86
	ReasonNotAuthorized           ReasonCode = 0x87
	ReasonServerUnavailable       ReasonCode = 0x88
	ReasonServerBusy              ReasonCode = 0x89
	ReasonBadAuthenticationMethod ReasonCode = 0x8c
	ReasonTopicFilterInvalid      ReasonCode = 0x8f
	ReasonTopicNameInvalid        ReasonCode = 0x90
	ReasonPacketIdentifierInUse   ReasonCode = 0x91
	ReasonPacketIdentifierMissing ReasonCode = 0x92
	ReasonReceiveMaximumExceeded  ReasonCode = 0x93
	ReasonTopicAliasInvalid       ReasonCode = 0x94
	ReasonPacketTooLarge          ReasonCode = 0x95
	ReasonMessageRateTooHigh      ReasonCode = 0x96
	ReasonQuotaExceeded           ReasonCode = 0x97
	ReasonAdministrativeAction    ReasonCode = 0x98
	ReasonPayloadFormatInvalid    ReasonCode = 0x99
	ReasonRetainNotSupported      ReasonCode = 0x9a
	ReasonQoSNotSupported         ReasonCode = 0x9b
)

func (c ReasonCode) Byte() byte {
	return byte(c)
}

// PropertyID 描述 MQTT 5 属性编号。
type PropertyID byte

const (
	PropertyPayloadFormatIndicator          PropertyID = 0x01
	PropertyMessageExpiryInterval           PropertyID = 0x02
	PropertyContentType                     PropertyID = 0x03
	PropertyResponseTopic                   PropertyID = 0x08
	PropertyCorrelationData                 PropertyID = 0x09
	PropertySubscriptionIdentifier          PropertyID = 0x0b
	PropertySessionExpiryInterval           PropertyID = 0x11
	PropertyAssignedClientIdentifier        PropertyID = 0x12
	PropertyServerKeepAlive                 PropertyID = 0x13
	PropertyAuthenticationMethod            PropertyID = 0x15
	PropertyAuthenticationData              PropertyID = 0x16
	PropertyRequestProblemInformation       PropertyID = 0x17
	PropertyWillDelayInterval               PropertyID = 0x18
	PropertyRequestResponseInformation      PropertyID = 0x19
	PropertyResponseInformation             PropertyID = 0x1a
	PropertyServerReference                 PropertyID = 0x1c
	PropertyReasonString                    PropertyID = 0x1f
	PropertyReceiveMaximum                  PropertyID = 0x21
	PropertyTopicAliasMaximum               PropertyID = 0x22
	PropertyTopicAlias                      PropertyID = 0x23
	PropertyMaximumQoS                      PropertyID = 0x24
	PropertyRetainAvailable                 PropertyID = 0x25
	PropertyUserProperty                    PropertyID = 0x26
	PropertyMaximumPacketSize               PropertyID = 0x27
	PropertyWildcardSubscriptionAvailable   PropertyID = 0x28
	PropertySubscriptionIdentifierAvailable PropertyID = 0x29
	PropertySharedSubscriptionAvailable     PropertyID = 0x2a
)

func (id PropertyID) Byte() byte {
	return byte(id)
}

// UserProperty 表示 MQTT 5 的用户自定义键值属性。
type UserProperty struct {
	Key   string
	Value string
}

// MQTT5Properties 是 MQTT 5 属性的 Go 化容器，后续完整编解码基于该结构扩展。
type MQTT5Properties struct {
	PayloadFormatIndicator     byte
	HasPayloadFormatIndicator  bool
	MessageExpiryInterval      uint32
	HasMessageExpiryInterval   bool
	ContentType                string
	ResponseTopic              string
	CorrelationData            []byte
	SubscriptionIdentifier     uint32
	HasSubscriptionIdentifier  bool
	SessionExpiryInterval      uint32
	HasSessionExpiryInterval   bool
	AssignedClientIdentifier   string
	ServerKeepAlive            uint16
	HasServerKeepAlive         bool
	AuthenticationMethod       string
	AuthenticationData         []byte
	RequestProblemInformation  bool
	HasRequestProblemInfo      bool
	WillDelayInterval          uint32
	HasWillDelayInterval       bool
	RequestResponseInfo        bool
	HasRequestResponseInfo     bool
	ResponseInformation        string
	ServerReference            string
	ReceiveMaximum             uint16
	HasReceiveMaximum          bool
	TopicAliasMaximum          uint16
	HasTopicAliasMaximum       bool
	TopicAlias                 uint16
	HasTopicAlias              bool
	MaximumQoS                 byte
	HasMaximumQoS              bool
	RetainAvailable            bool
	HasRetainAvailable         bool
	MaximumPacketSize          uint32
	HasMaximumPacketSize       bool
	WildcardSubscription       bool
	HasWildcardSubscription    bool
	SubscriptionIDAvailable    bool
	HasSubscriptionIDAvailable bool
	SharedSubscription         bool
	HasSharedSubscription      bool
	ReasonString               string
	UserProperties             []UserProperty
}

func (p MQTT5Properties) Empty() bool {
	return !p.HasPayloadFormatIndicator &&
		!p.HasMessageExpiryInterval &&
		p.ContentType == "" &&
		p.ResponseTopic == "" &&
		len(p.CorrelationData) == 0 &&
		!p.HasSubscriptionIdentifier &&
		!p.HasSessionExpiryInterval &&
		p.AssignedClientIdentifier == "" &&
		!p.HasServerKeepAlive &&
		p.AuthenticationMethod == "" &&
		len(p.AuthenticationData) == 0 &&
		!p.HasRequestProblemInfo &&
		!p.HasWillDelayInterval &&
		!p.HasRequestResponseInfo &&
		p.ResponseInformation == "" &&
		p.ServerReference == "" &&
		!p.HasReceiveMaximum &&
		!p.HasTopicAliasMaximum &&
		!p.HasTopicAlias &&
		!p.HasMaximumQoS &&
		!p.HasRetainAvailable &&
		!p.HasMaximumPacketSize &&
		!p.HasWildcardSubscription &&
		!p.HasSubscriptionIDAvailable &&
		!p.HasSharedSubscription &&
		p.ReasonString == "" &&
		len(p.UserProperties) == 0
}
