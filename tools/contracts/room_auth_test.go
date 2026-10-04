package main

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestRoomRequestAuthResolvesSharedCredentialContract(t *testing.T) {
	t.Parallel()

	const synthetic = "synthetic"

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.Context = t.Context()

	document, err := loader.LoadFromFile("../../api/map-client-models.openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"HomeRoomsRequest", "SharedDeviceRoomsRequest"} {
		request := document.Components.Schemas[name].Value

		auth := request.Properties["auth"].Value
		if len(auth.AllOf) != 1 || auth.AllOf[0].Ref != "./client-models.openapi.yaml#/components/schemas/AuthContext" {
			t.Fatalf("%s must use the shared credential contract", name)
		}

		complete := map[string]any{"token": synthetic, "clientId": synthetic, "baseUrl": "https://example.invalid",
			"mqtt": map[string]any{"user": synthetic, "secret": synthetic, "key": synthetic,
				"brokerUrl": "ssl://example.invalid:8883", "apiUrl": "https://example.invalid", "signingKey": synthetic}}

		err = auth.VisitJSON(complete)
		if err != nil {
			t.Fatalf("%s valid credentials: %v", name, err)
		}

		delete(complete, "token")

		if auth.VisitJSON(complete) == nil {
			t.Fatalf("%s accepted missing account token", name)
		}

		complete["token"] = synthetic
		mqtt, _ := complete["mqtt"].(map[string]any)
		delete(mqtt, "secret")

		if auth.VisitJSON(complete) == nil {
			t.Fatalf("%s accepted missing nested MQTT secret", name)
		}
	}
}
