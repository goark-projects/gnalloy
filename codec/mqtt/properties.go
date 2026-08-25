package mqtt

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/codec"
)

func mqttPropertiesSize(props MQTT5Properties) int {
	body := mqttPropertiesBodySize(props)
	return variableByteIntegerSize(body) + body
}

func mqttPropertiesBodySize(props MQTT5Properties) int {
	size := 0
	if props.HasPayloadFormatIndicator {
		size += propertyIDSize(PropertyPayloadFormatIndicator) + 1
	}
	if props.HasMessageExpiryInterval {
		size += propertyIDSize(PropertyMessageExpiryInterval) + 4
	}
	if props.ContentType != "" {
		size += propertyIDSize(PropertyContentType) + mqttStringSize(props.ContentType)
	}
	if props.ResponseTopic != "" {
		size += propertyIDSize(PropertyResponseTopic) + mqttStringSize(props.ResponseTopic)
	}
	if props.CorrelationData != nil {
		size += propertyIDSize(PropertyCorrelationData) + mqttBinarySize(props.CorrelationData)
	}
	if props.HasSubscriptionIdentifier {
		size += propertyIDSize(PropertySubscriptionIdentifier) + variableByteIntegerSize(int(props.SubscriptionIdentifier))
	}
	if props.HasSessionExpiryInterval {
		size += propertyIDSize(PropertySessionExpiryInterval) + 4
	}
	if props.AssignedClientIdentifier != "" {
		size += propertyIDSize(PropertyAssignedClientIdentifier) + mqttStringSize(props.AssignedClientIdentifier)
	}
	if props.HasServerKeepAlive {
		size += propertyIDSize(PropertyServerKeepAlive) + 2
	}
	if props.AuthenticationMethod != "" {
		size += propertyIDSize(PropertyAuthenticationMethod) + mqttStringSize(props.AuthenticationMethod)
	}
	if props.AuthenticationData != nil {
		size += propertyIDSize(PropertyAuthenticationData) + mqttBinarySize(props.AuthenticationData)
	}
	if props.HasRequestProblemInfo {
		size += propertyIDSize(PropertyRequestProblemInformation) + 1
	}
	if props.HasWillDelayInterval {
		size += propertyIDSize(PropertyWillDelayInterval) + 4
	}
	if props.HasRequestResponseInfo {
		size += propertyIDSize(PropertyRequestResponseInformation) + 1
	}
	if props.ResponseInformation != "" {
		size += propertyIDSize(PropertyResponseInformation) + mqttStringSize(props.ResponseInformation)
	}
	if props.ServerReference != "" {
		size += propertyIDSize(PropertyServerReference) + mqttStringSize(props.ServerReference)
	}
	if props.ReasonString != "" {
		size += propertyIDSize(PropertyReasonString) + mqttStringSize(props.ReasonString)
	}
	if props.HasReceiveMaximum {
		size += propertyIDSize(PropertyReceiveMaximum) + 2
	}
	if props.HasTopicAliasMaximum {
		size += propertyIDSize(PropertyTopicAliasMaximum) + 2
	}
	if props.HasTopicAlias {
		size += propertyIDSize(PropertyTopicAlias) + 2
	}
	if props.HasMaximumQoS {
		size += propertyIDSize(PropertyMaximumQoS) + 1
	}
	if props.HasRetainAvailable {
		size += propertyIDSize(PropertyRetainAvailable) + 1
	}
	if props.HasMaximumPacketSize {
		size += propertyIDSize(PropertyMaximumPacketSize) + 4
	}
	if props.HasWildcardSubscription {
		size += propertyIDSize(PropertyWildcardSubscriptionAvailable) + 1
	}
	if props.HasSubscriptionIDAvailable {
		size += propertyIDSize(PropertySubscriptionIdentifierAvailable) + 1
	}
	if props.HasSharedSubscription {
		size += propertyIDSize(PropertySharedSubscriptionAvailable) + 1
	}
	for _, user := range props.UserProperties {
		size += propertyIDSize(PropertyUserProperty) + mqttStringSize(user.Key) + mqttStringSize(user.Value)
	}
	return size
}

func writeMQTT5Properties(buf buffer.ByteBuf, props MQTT5Properties) error {
	bodySize := mqttPropertiesBodySize(props)
	if err := writeVariableByteInteger(buf, bodySize); err != nil {
		return err
	}
	return writeMQTT5PropertyBody(buf, props)
}

func writeMQTT5PropertyBody(buf buffer.ByteBuf, props MQTT5Properties) error {
	if props.HasPayloadFormatIndicator {
		if err := writePropertyID(buf, PropertyPayloadFormatIndicator); err != nil {
			return err
		}
		if err := writeByte(buf, props.PayloadFormatIndicator); err != nil {
			return err
		}
	}
	if props.HasMessageExpiryInterval {
		if err := writePropertyID(buf, PropertyMessageExpiryInterval); err != nil {
			return err
		}
		if err := writeUint32(buf, props.MessageExpiryInterval); err != nil {
			return err
		}
	}
	if props.ContentType != "" {
		if err := writeStringProperty(buf, PropertyContentType, props.ContentType); err != nil {
			return err
		}
	}
	if props.ResponseTopic != "" {
		if err := writeStringProperty(buf, PropertyResponseTopic, props.ResponseTopic); err != nil {
			return err
		}
	}
	if props.CorrelationData != nil {
		if err := writeBinaryProperty(buf, PropertyCorrelationData, props.CorrelationData); err != nil {
			return err
		}
	}
	if props.HasSubscriptionIdentifier {
		if props.SubscriptionIdentifier > maxRemainingLength {
			return codec.ErrInvalidLengthField
		}
		if err := writePropertyID(buf, PropertySubscriptionIdentifier); err != nil {
			return err
		}
		if err := writeVariableByteInteger(buf, int(props.SubscriptionIdentifier)); err != nil {
			return err
		}
	}
	if props.HasSessionExpiryInterval {
		if err := writeUint32Property(buf, PropertySessionExpiryInterval, props.SessionExpiryInterval); err != nil {
			return err
		}
	}
	if props.AssignedClientIdentifier != "" {
		if err := writeStringProperty(buf, PropertyAssignedClientIdentifier, props.AssignedClientIdentifier); err != nil {
			return err
		}
	}
	if props.HasServerKeepAlive {
		if err := writeUint16Property(buf, PropertyServerKeepAlive, props.ServerKeepAlive); err != nil {
			return err
		}
	}
	if props.AuthenticationMethod != "" {
		if err := writeStringProperty(buf, PropertyAuthenticationMethod, props.AuthenticationMethod); err != nil {
			return err
		}
	}
	if props.AuthenticationData != nil {
		if err := writeBinaryProperty(buf, PropertyAuthenticationData, props.AuthenticationData); err != nil {
			return err
		}
	}
	if props.HasRequestProblemInfo {
		if err := writeBoolProperty(buf, PropertyRequestProblemInformation, props.RequestProblemInformation); err != nil {
			return err
		}
	}
	if props.HasWillDelayInterval {
		if err := writeUint32Property(buf, PropertyWillDelayInterval, props.WillDelayInterval); err != nil {
			return err
		}
	}
	if props.HasRequestResponseInfo {
		if err := writeBoolProperty(buf, PropertyRequestResponseInformation, props.RequestResponseInfo); err != nil {
			return err
		}
	}
	if props.ResponseInformation != "" {
		if err := writeStringProperty(buf, PropertyResponseInformation, props.ResponseInformation); err != nil {
			return err
		}
	}
	if props.ServerReference != "" {
		if err := writeStringProperty(buf, PropertyServerReference, props.ServerReference); err != nil {
			return err
		}
	}
	if props.ReasonString != "" {
		if err := writeStringProperty(buf, PropertyReasonString, props.ReasonString); err != nil {
			return err
		}
	}
	if props.HasReceiveMaximum {
		if err := writeUint16Property(buf, PropertyReceiveMaximum, props.ReceiveMaximum); err != nil {
			return err
		}
	}
	if props.HasTopicAliasMaximum {
		if err := writeUint16Property(buf, PropertyTopicAliasMaximum, props.TopicAliasMaximum); err != nil {
			return err
		}
	}
	if props.HasTopicAlias {
		if err := writeUint16Property(buf, PropertyTopicAlias, props.TopicAlias); err != nil {
			return err
		}
	}
	if props.HasMaximumQoS {
		if err := writeByteProperty(buf, PropertyMaximumQoS, props.MaximumQoS); err != nil {
			return err
		}
	}
	if props.HasRetainAvailable {
		if err := writeBoolProperty(buf, PropertyRetainAvailable, props.RetainAvailable); err != nil {
			return err
		}
	}
	if props.HasMaximumPacketSize {
		if err := writeUint32Property(buf, PropertyMaximumPacketSize, props.MaximumPacketSize); err != nil {
			return err
		}
	}
	if props.HasWildcardSubscription {
		if err := writeBoolProperty(buf, PropertyWildcardSubscriptionAvailable, props.WildcardSubscription); err != nil {
			return err
		}
	}
	if props.HasSubscriptionIDAvailable {
		if err := writeBoolProperty(buf, PropertySubscriptionIdentifierAvailable, props.SubscriptionIDAvailable); err != nil {
			return err
		}
	}
	if props.HasSharedSubscription {
		if err := writeBoolProperty(buf, PropertySharedSubscriptionAvailable, props.SharedSubscription); err != nil {
			return err
		}
	}
	for _, user := range props.UserProperties {
		if !validMQTTString(user.Key) || !validMQTTString(user.Value) {
			return codec.ErrInvalidFrameLength
		}
		if err := writePropertyID(buf, PropertyUserProperty); err != nil {
			return err
		}
		if err := writeMQTTString(buf, user.Key); err != nil {
			return err
		}
		if err := writeMQTTString(buf, user.Value); err != nil {
			return err
		}
	}
	return nil
}

func readMQTT5Properties(buf buffer.ByteBuf, index int) (MQTT5Properties, int, error) {
	length, n, ok, err := readVariableByteInteger(buf, index)
	if err != nil || !ok {
		return MQTT5Properties{}, 0, codec.ErrInvalidFrameLength
	}
	index += n
	end := index + length
	if end > buf.WriterIndex() {
		return MQTT5Properties{}, 0, codec.ErrInvalidFrameLength
	}
	props, err := readMQTT5PropertyBody(buf, index, end)
	if err != nil {
		return MQTT5Properties{}, 0, err
	}
	return props, end, nil
}

func readMQTT5PropertyBody(buf buffer.ByteBuf, index int, end int) (MQTT5Properties, error) {
	var props MQTT5Properties
	seen := make(map[PropertyID]struct{}, 8)
	for index < end {
		rawID, n, ok, err := readVariableByteInteger(buf, index)
		if err != nil || !ok || rawID > 0xff {
			return MQTT5Properties{}, codec.ErrInvalidFrameLength
		}
		index += n
		id := PropertyID(rawID)
		if id != PropertyUserProperty {
			if _, exists := seen[id]; exists {
				return MQTT5Properties{}, codec.ErrInvalidFrameLength
			}
			seen[id] = struct{}{}
		}
		switch id {
		case PropertyPayloadFormatIndicator:
			v, next, err := readByteProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.PayloadFormatIndicator = v
			props.HasPayloadFormatIndicator = true
			index = next
		case PropertyMessageExpiryInterval:
			v, next, err := readUint32Property(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.MessageExpiryInterval = v
			props.HasMessageExpiryInterval = true
			index = next
		case PropertyContentType:
			v, next, err := readStringProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.ContentType = v
			index = next
		case PropertyResponseTopic:
			v, next, err := readStringProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.ResponseTopic = v
			index = next
		case PropertyCorrelationData:
			v, next, err := readBinaryProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.CorrelationData = v
			index = next
		case PropertySubscriptionIdentifier:
			v, n, ok, err := readVariableByteInteger(buf, index)
			if err != nil || !ok || index+n > end {
				return MQTT5Properties{}, codec.ErrInvalidFrameLength
			}
			props.SubscriptionIdentifier = uint32(v)
			props.HasSubscriptionIdentifier = true
			index += n
		case PropertySessionExpiryInterval:
			v, next, err := readUint32Property(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.SessionExpiryInterval = v
			props.HasSessionExpiryInterval = true
			index = next
		case PropertyAssignedClientIdentifier:
			v, next, err := readStringProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.AssignedClientIdentifier = v
			index = next
		case PropertyServerKeepAlive:
			v, next, err := readUint16Property(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.ServerKeepAlive = v
			props.HasServerKeepAlive = true
			index = next
		case PropertyAuthenticationMethod:
			v, next, err := readStringProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.AuthenticationMethod = v
			index = next
		case PropertyAuthenticationData:
			v, next, err := readBinaryProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.AuthenticationData = v
			index = next
		case PropertyRequestProblemInformation:
			v, next, err := readBoolProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.RequestProblemInformation = v
			props.HasRequestProblemInfo = true
			index = next
		case PropertyWillDelayInterval:
			v, next, err := readUint32Property(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.WillDelayInterval = v
			props.HasWillDelayInterval = true
			index = next
		case PropertyRequestResponseInformation:
			v, next, err := readBoolProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.RequestResponseInfo = v
			props.HasRequestResponseInfo = true
			index = next
		case PropertyResponseInformation:
			v, next, err := readStringProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.ResponseInformation = v
			index = next
		case PropertyServerReference:
			v, next, err := readStringProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.ServerReference = v
			index = next
		case PropertyReasonString:
			v, next, err := readStringProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.ReasonString = v
			index = next
		case PropertyReceiveMaximum:
			v, next, err := readUint16Property(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.ReceiveMaximum = v
			props.HasReceiveMaximum = true
			index = next
		case PropertyTopicAliasMaximum:
			v, next, err := readUint16Property(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.TopicAliasMaximum = v
			props.HasTopicAliasMaximum = true
			index = next
		case PropertyTopicAlias:
			v, next, err := readUint16Property(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.TopicAlias = v
			props.HasTopicAlias = true
			index = next
		case PropertyMaximumQoS:
			v, next, err := readByteProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.MaximumQoS = v
			props.HasMaximumQoS = true
			index = next
		case PropertyRetainAvailable:
			v, next, err := readBoolProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.RetainAvailable = v
			props.HasRetainAvailable = true
			index = next
		case PropertyMaximumPacketSize:
			v, next, err := readUint32Property(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.MaximumPacketSize = v
			props.HasMaximumPacketSize = true
			index = next
		case PropertyWildcardSubscriptionAvailable:
			v, next, err := readBoolProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.WildcardSubscription = v
			props.HasWildcardSubscription = true
			index = next
		case PropertySubscriptionIdentifierAvailable:
			v, next, err := readBoolProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.SubscriptionIDAvailable = v
			props.HasSubscriptionIDAvailable = true
			index = next
		case PropertySharedSubscriptionAvailable:
			v, next, err := readBoolProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.SharedSubscription = v
			props.HasSharedSubscription = true
			index = next
		case PropertyUserProperty:
			key, next, err := readStringProperty(buf, index, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			value, next, err := readStringProperty(buf, next, end)
			if err != nil {
				return MQTT5Properties{}, err
			}
			props.UserProperties = append(props.UserProperties, UserProperty{Key: key, Value: value})
			index = next
		default:
			return MQTT5Properties{}, codec.ErrInvalidFrameLength
		}
	}
	if index != end {
		return MQTT5Properties{}, codec.ErrInvalidFrameLength
	}
	return props, nil
}

func propertyIDSize(id PropertyID) int {
	return variableByteIntegerSize(int(id))
}

func writePropertyID(buf buffer.ByteBuf, id PropertyID) error {
	return writeVariableByteInteger(buf, int(id))
}

func writeStringProperty(buf buffer.ByteBuf, id PropertyID, value string) error {
	if !validMQTTString(value) {
		return codec.ErrInvalidFrameLength
	}
	if err := writePropertyID(buf, id); err != nil {
		return err
	}
	return writeMQTTString(buf, value)
}

func writeBinaryProperty(buf buffer.ByteBuf, id PropertyID, value []byte) error {
	if !validMQTTBinary(value) {
		return codec.ErrInvalidFrameLength
	}
	if err := writePropertyID(buf, id); err != nil {
		return err
	}
	return writeMQTTBinary(buf, value)
}

func writeUint16Property(buf buffer.ByteBuf, id PropertyID, value uint16) error {
	if err := writePropertyID(buf, id); err != nil {
		return err
	}
	return writeUint16(buf, value)
}

func writeUint32Property(buf buffer.ByteBuf, id PropertyID, value uint32) error {
	if err := writePropertyID(buf, id); err != nil {
		return err
	}
	return writeUint32(buf, value)
}

func writeByteProperty(buf buffer.ByteBuf, id PropertyID, value byte) error {
	if err := writePropertyID(buf, id); err != nil {
		return err
	}
	return writeByte(buf, value)
}

func writeBoolProperty(buf buffer.ByteBuf, id PropertyID, value bool) error {
	v := byte(0)
	if value {
		v = 1
	}
	return writeByteProperty(buf, id, v)
}

func readStringProperty(buf buffer.ByteBuf, index int, end int) (string, int, error) {
	value, next, err := readMQTTString(buf, index)
	if err != nil {
		return "", 0, err
	}
	if next > end {
		return "", 0, codec.ErrInvalidFrameLength
	}
	return value, next, nil
}

func readBinaryProperty(buf buffer.ByteBuf, index int, end int) ([]byte, int, error) {
	value, next, err := readMQTTBinary(buf, index)
	if err != nil {
		return nil, 0, err
	}
	if next > end {
		return nil, 0, codec.ErrInvalidFrameLength
	}
	return value, next, nil
}

func readUint16Property(buf buffer.ByteBuf, index int, end int) (uint16, int, error) {
	if index+2 > end {
		return 0, 0, codec.ErrInvalidFrameLength
	}
	value, err := readUint16(buf, index)
	if err != nil {
		return 0, 0, err
	}
	return value, index + 2, nil
}

func readUint32Property(buf buffer.ByteBuf, index int, end int) (uint32, int, error) {
	if index+4 > end {
		return 0, 0, codec.ErrInvalidFrameLength
	}
	value, err := readUint32(buf, index)
	if err != nil {
		return 0, 0, err
	}
	return value, index + 4, nil
}

func readByteProperty(buf buffer.ByteBuf, index int, end int) (byte, int, error) {
	if index+1 > end {
		return 0, 0, codec.ErrInvalidFrameLength
	}
	value, ok := buf.GetByte(index)
	if !ok {
		return 0, 0, codec.ErrInvalidFrameLength
	}
	return value, index + 1, nil
}

func readBoolProperty(buf buffer.ByteBuf, index int, end int) (bool, int, error) {
	value, next, err := readByteProperty(buf, index, end)
	if err != nil {
		return false, 0, err
	}
	if value > 1 {
		return false, 0, codec.ErrInvalidFrameLength
	}
	return value == 1, next, nil
}
