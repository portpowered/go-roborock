package mqtt

import (
	"crypto/aes"
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

	if config.Protocol != "" && config.Protocol != protocol.MQTTVersionV1 && config.Protocol != protocol.MQTTVersionA01 {
		return nil, unsupported("open")
	}

	return endpoint, nil
}

func validateEndpoint(endpoint *url.URL) error {
	if endpoint.Scheme != "ssl" && endpoint.Scheme != "mqtts" {
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
		if value == "" || len(value) > protocol.MQTTMaxStringLength || strings.ContainsRune(value, '\x00') {
			return errInvalidMQTTCredentialsOrDeviceIdentifier
		}
	}

	if strings.ContainsAny(config.User+config.DeviceID, "+#/") {
		return errInvalidMQTTAccountOrDeviceTopicIdentifier
	}

	topicLength := len(protocol.MQTTSubscribeTopicPrefix) + len(config.User) + len(config.DeviceID)

	topicLength += protocol.MQTTUsernameLength + protocol.MQTTTopicSeparators

	if topicLength > protocol.MQTTMaxStringLength {
		return invalid("open", "device topic exceeds MQTT wire limit")
	}

	if len(config.LocalKey) != aes.BlockSize {
		return errDeviceLocalKeyMustContain16Bytes
	}

	return nil
}
