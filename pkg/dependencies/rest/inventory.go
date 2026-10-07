package rest

import (
	"encoding/json"
	"strings"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// DecodeProperty decodes JSON-encoded product constraints; JSON null returns nil.
func DecodeProperty(encoded string) (*dependencymodels.ProductPropertyConstraints, error) {
	return decodeInventory[dependencymodels.ProductPropertyConstraints](encoded, "ProductPropertyConstraints")
}

// DecodeProductInfo decodes the JSON string in inventory datapoint 10005.
func DecodeProductInfo(encoded string) (*dependencymodels.InventoryProductInfo, error) {
	return decodeInventory[dependencymodels.InventoryProductInfo](encoded, "InventoryProductInfo")
}

// DecodeOTA decodes the JSON string in inventory datapoint 10007.
func DecodeOTA(encoded string) (*dependencymodels.InventoryOTA, error) {
	return decodeInventory[dependencymodels.InventoryOTA](encoded, "InventoryOTA")
}

// DecodeProgramState decodes the JSON string in inventory datapoint 10001.
func DecodeProgramState(encoded string) (*dependencymodels.InventoryProgramState, error) {
	return decodeInventory[dependencymodels.InventoryProgramState](encoded, "InventoryProgramState")
}

func decodeInventory[T any](encoded, schemaName string) (*T, error) {
	if strings.TrimSpace(encoded) == "null" {
		return nil, nil
	}

	schemas, err := responseSchemas()
	if err != nil {
		return nil, roborockerrors.New(roborockerrors.Protocol, "inventory", "contract unavailable", err)
	}

	schema := schemas["schema:"+schemaName]
	if schema == nil {
		return nil, roborockerrors.New(roborockerrors.Protocol, "inventory", "missing embedded inventory contract", nil)
	}

	var value any

	err = json.Unmarshal([]byte(encoded), &value)
	if err != nil {
		return nil, roborockerrors.New(roborockerrors.Protocol, "inventory", "invalid embedded JSON", err)
	}

	err = schema.VisitJSON(withoutNulls(value))
	if err != nil {
		return nil, roborockerrors.New(roborockerrors.Protocol, "inventory", "embedded JSON violates contract", err)
	}

	var result T

	err = json.Unmarshal([]byte(encoded), &result)
	if err != nil {
		return nil, roborockerrors.New(roborockerrors.Protocol, "inventory", "invalid embedded inventory value", err)
	}

	return &result, nil
}
