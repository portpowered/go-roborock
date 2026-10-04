package mqtt

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

func validateConfig(config Config) (*url.URL, error) {
	endpoint, err := url.Parse(config.BrokerURL)
	if err != nil {
		return nil, roborockerrors.New(roborockerrors.InvalidArgument, "mqtt open", "invalid broker URL", err)
	}

	err = validateEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	err = validateCredentials(config)
	if err != nil {
		return nil, err
	}

	switch config.Protocol {
	case "", protocol.MQTTVersionV1, protocol.MQTTVersionA01, protocol.B01Version:
	default:
		return nil, unsupported("open")
	}

	return endpoint, nil
}

func validateEndpoint(endpoint *url.URL) error {
	if endpoint.Scheme != protocol.MQTTBrokerSchemeSSL && endpoint.Scheme != protocol.MQTTBrokerSchemeMQTTS {
		return errMQTTBrokerRequiresTLSSslOrMqtts
	}

	if endpoint.Hostname() == "" || endpoint.Port() == "" || endpoint.User != nil {
		return errInvalidMQTTBrokerURL
	}

	if endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return errInvalidMQTTBrokerURL
	}

	return nil
}

func validateCredentials(config Config) error {
	for _, value := range []string{config.User, config.Secret, config.Key, config.DeviceID, config.LocalKey} {
		if value == "" || len(value) > protocol.MQTTMaxStringLength || strings.ContainsRune(value, protocol.MQTTNullRune) {
			return errInvalidMQTTCredentialsOrDeviceIdentifier
		}
	}

	if strings.ContainsAny(config.User+config.DeviceID, protocol.MQTTForbiddenTopicCharacters) {
		return errInvalidMQTTAccountOrDeviceTopicIdentifier
	}

	topicLength := len(fmt.Sprintf(protocol.MQTTSubscribeTopicFormat, config.User, "", config.DeviceID))
	topicLength += protocol.MQTTUsernameDigestHexEnd - protocol.MQTTUsernameDigestHexStart

	if topicLength > protocol.MQTTMaxStringLength {
		return invalid("open", "device topic exceeds MQTT wire limit")
	}

	if len(config.LocalKey) != protocol.MQTTLocalKeyBytes {
		return errDeviceLocalKeyMustContain16Bytes
	}

	return nil
}
