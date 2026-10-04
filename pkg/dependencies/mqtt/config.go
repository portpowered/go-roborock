package mqtt

import (
	"github.com/portpowered/go-roborock/internal/protocol"
	"net/url"
	"strings"
)

func validateConfig(config Config) (*url.URL, error) {
	endpoint, err := url.Parse(config.BrokerURL)
	if err != nil {
		return nil, err
	}

	if endpoint.Scheme != "ssl" && endpoint.Scheme != "mqtts" {
		return nil, errMQTTBrokerRequiresTLSSslOrMqtts
	}

	if endpoint.Hostname() == "" || endpoint.Port() == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errInvalidMQTTBrokerURL
	}

	for _, value := range []string{config.User, config.Secret, config.Key, config.DeviceID, config.LocalKey} {
		if value == "" || len(value) > 65535 || strings.ContainsAny(value, "\x00") {
			return nil, errInvalidMQTTCredentialsOrDeviceIdentifier
		}
	}

	if strings.ContainsAny(config.User+config.DeviceID, "+#/") {
		return nil, errInvalidMQTTAccountOrDeviceTopicIdentifier
	}

	if len(protocol.MQTTSubscribeTopicPrefix)+len(config.User)+len(config.DeviceID)+10 > 65535 {
		return nil, invalid("open", "device topic exceeds MQTT wire limit")
	}

	if len(config.LocalKey) != 16 {
		return nil, errDeviceLocalKeyMustContain16Bytes
	}

	if config.Protocol != "" && config.Protocol != protocol.MQTTVersionV1 && config.Protocol != protocol.MQTTVersionA01 {
		return nil, unsupported("open")
	}

	return endpoint, nil
}
