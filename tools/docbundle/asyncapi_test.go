package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAsyncBundleNamespacesChannelsAndPreservesOperations(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	const (
		first = `{
 "asyncapi":"3.0.0", "info":{"title":"First","version":"1"},
 "channels":{"requests":{"address":"rr/m/i/{device}","messages":{
 "request":{"payload":{"$ref":"./second.asyncapi.yaml#/components/schemas/Value"}}
 }}},
 "operations":{"Send":{"action":"send","channel":{"$ref":"#/channels/requests"},
 "messages":[{"$ref":"#/channels/requests/messages/request"}],"tags":[{"name":"vacuum"}]}},
 "components":{"schemas":{"Value":{"type":"integer"}}}
 }`
		second = `{
 "asyncapi":"3.0.0", "info":{"title":"Second","version":"1"},
 "channels":{"requests":{"address":"rr/m/o/{device}","messages":{}}},
 "operations":{},"components":{"schemas":{"Value":{"type":"string"}}}
 }`
	)

	files := []string{filepath.Join(directory, "first.asyncapi.yaml"), filepath.Join(directory, "second.asyncapi.yaml")}
	for index, source := range []string{first, second} {
		err := os.WriteFile(files[index], []byte(source), fileMode)
		if err != nil {
			t.Fatal(err)
		}
	}

	data, err := buildAsync(files)
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any

	err = json.Unmarshal(data, &document)
	if err != nil {
		t.Fatal(err)
	}

	verifyAsyncBundle(t, document)
}

func verifyAsyncBundle(t *testing.T, document map[string]any) {
	t.Helper()

	channels, _ := document["channels"].(map[string]any)
	if channels["first.requests"] == nil || channels["second.requests"] == nil {
		t.Fatal("channel names collided")
	}

	operations, _ := document["operations"].(map[string]any)
	operation, _ := operations["Send"].(map[string]any)

	channel, _ := operation["channel"].(map[string]any)
	if channel["$ref"] != "#/channels/first.requests" {
		t.Fatal("operation channel reference was not rewritten")
	}

	firstChannel, _ := channels["first.requests"].(map[string]any)
	messages, _ := firstChannel["messages"].(map[string]any)
	message, _ := messages["request"].(map[string]any)

	payload, _ := message["payload"].(map[string]any)
	if payload["$ref"] != "#/components/schemas/second.Value" {
		t.Fatal("cross-document schema reference was not rewritten")
	}
}

func TestDocumentationMessagesResolveInOneHop(t *testing.T) {
	t.Parallel()

	payload := map[string]any{"$ref": "#/components/schemas/wire.Request"}
	definition := map[string]any{"payload": payload}
	messages := map[string]any{"request": map[string]any{"$ref": "#/components/messages/wire.Request"}}
	document := object{
		componentKey: object{"messages": object{"wire.Request": definition}},
		"channels":   object{"wire.requests": map[string]any{"messages": messages}},
	}
	inlineChannelMessages(document)

	message, _ := messages["request"].(map[string]any)

	schema, _ := message["payload"].(map[string]any)
	if schema["$ref"] != "#/components/schemas/wire.Request" {
		t.Fatal("operation message targets must expose payloads after one reference resolution")
	}
}
