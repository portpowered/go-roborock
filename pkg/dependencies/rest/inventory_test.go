package rest_test

import (
	"errors"
	"testing"

	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

func TestDecodePropertyConstraints(t *testing.T) {
	t.Parallel()

	value, err := rest.DecodeProperty(`{
		"range":["quiet","standard"],"min":0,"max":100,"step":1,"unit":null,"scale":1,"future":true
	}`)
	if err != nil {
		t.Fatal(err)
	}

	if value == nil || value.Range == nil || len(*value.Range) != 2 ||
		value.Max == nil || *value.Max != 100 || value.Unit != nil {
		t.Fatal("typed property constraints lost")
	}

	if len(value.AdditionalProperties["future"]) == 0 {
		t.Fatal("unknown metadata lost")
	}
}

func TestNullAndMalformedProperty(t *testing.T) {
	t.Parallel()

	none, err := rest.DecodeProperty("null")
	if err != nil || none != nil {
		t.Fatalf("null constraints: %v", err)
	}

	_, err = rest.DecodeProperty(`{"min":"invalid"}`)
	if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "", "", nil)) {
		t.Fatalf("invalid property accepted: %v", err)
	}
}

func TestDecodeProductInfo(t *testing.T) {
	t.Parallel()

	product, err := rest.DecodeProductInfo(`{
		"sn":"synthetic-sn","ssid":"synthetic-network","rssi":-57,"oba":{"language":"en","featureset":"0"}
	}`)
	if err != nil || product == nil || product.Rssi == nil || *product.Rssi != -57 ||
		product.Oba == nil || product.Oba.Language == nil || *product.Oba.Language != "en" {
		t.Fatalf("product metadata: %v", err)
	}
}

func TestDecodeOTA(t *testing.T) {
	t.Parallel()

	ota, err := rest.DecodeOTA(`{"mqttOtaData":{"mqttOtaStatus":{"status":"FUTURE_STATUS"}}}`)
	if err != nil || ota == nil || ota.MqttOtaData == nil || ota.MqttOtaData.MqttOtaStatus == nil ||
		ota.MqttOtaData.MqttOtaStatus.Status == nil || *ota.MqttOtaData.MqttOtaStatus.Status != "FUTURE_STATUS" {
		t.Fatalf("OTA metadata: %v", err)
	}
}

func TestDecodeProgramState(t *testing.T) {
	t.Parallel()

	program, err := rest.DecodeProgramState(`{"f":"t"}`)
	if err != nil || program == nil || program.F == nil || *program.F != "t" {
		t.Fatalf("program metadata: %v", err)
	}
}
