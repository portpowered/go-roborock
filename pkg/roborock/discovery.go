package roborock

import (
	"context"
	"strconv"

	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// GetHome returns the Roborock home identifier required by inventory requests.
func (c *Client) GetHome(ctx context.Context, request AccountRequest) (Home, error) {
	out, err := c.rest.HomeDetail(ctx, rest.HomeDetailRequest{Auth: authWire(request.Auth)})
	if err != nil {
		return Home{}, roborockerrors.Wrap(roborockerrors.Protocol, "get_home", "home lookup failed", err)
	}

	return Home{ID: out.Home.RrHomeId,
		Name: valueOrZero(out.Home.Name)}, nil
}

// GetHomeData joins device and product identity and includes shared devices.
// Unknown protocols are preserved; opening an unsupported version returns a typed error.
func (c *Client) GetHomeData(ctx context.Context, request HomeDataRequest) (HomeData, error) {
	version := request.Version

	if version == 0 {
		version = HomeDataV1
	}

	out, err := c.rest.HomeData(ctx, rest.HomeDataRequest{Auth: authWire(request.Auth),
		HomeID:  request.HomeID,
		Version: int(version),
	})
	if err != nil {
		return HomeData{}, roborockerrors.Wrap(roborockerrors.Protocol, "get_home_data", "inventory lookup failed", err)
	}

	home := out.Home
	products := make(map[string]dependencymodels.HomeProduct)

	for _, p := range valueOrZero(home.Products) {
		products[p.Id] = p
	}

	result := HomeData{Home: Home{ID: home.Id,
		Name: home.Name},
		Devices: []Device{},
		Rooms:   []Room{}}
	seen := make(map[string]bool)

	for _, device := range valueOrZero(home.Devices) {
		result.Devices = append(result.Devices, deviceProjection(device, products[device.ProductId], false))
		seen[device.Duid] = true
	}

	for _, device := range valueOrZero(home.ReceivedDevices) {
		if !seen[device.Duid] {
			result.Devices = append(result.Devices, deviceProjection(device, products[device.ProductId], true))
			seen[device.Duid] = true
		}
	}

	for _, room := range valueOrZero(home.Rooms) {
		result.Rooms = append(result.Rooms, Room{ID: room.Id,
			Name: room.Name})
	}

	return result, nil
}

func deviceProjection(device dependencymodels.HomeDevice, product dependencymodels.HomeProduct, shared bool) Device {
	protocol := ProtocolVersion(valueOrZero(device.Pv))

	if device.Pv == nil {
		protocol = ProtocolV1
	}

	category := product.Category
	result := Device{ID: device.Duid,
		Name:                device.Name,
		LocalKey:            device.LocalKey,
		Model:               product.Model,
		ProductID:           device.ProductId,
		Protocol:            protocol,
		Online:              device.Online,
		Shared:              shared,
		Firmware:            device.Fv,
		Category:            &category,
		Capability:          product.Capability,
		SupportedProperties: advertisedProperties(product),
		RoomID:              nil,
	}

	result.RoomID = device.RoomId

	return result
}

func advertisedProperties(product dependencymodels.HomeProduct) []int {
	result := []int{}
	seen := make(map[int]bool)

	for _, property := range valueOrZero(product.Schema) {
		if property.Id == nil {
			continue
		}

		propertyID := numericPropertyID(*property.Id)

		if propertyID > 0 && !seen[propertyID] {
			result = append(result, propertyID)
			seen[propertyID] = true
		}
	}

	return result
}

// numericPropertyID accepts only positive representable integer property identifiers.
func numericPropertyID(property dependencymodels.ProductPropertyID) int {
	numeric, err := property.AsProductPropertyID1()
	if err == nil {
		converted := int(numeric)

		if numeric > 0 && int64(converted) == numeric {
			return converted
		}

		return 0
	}

	text, err := property.AsProductPropertyID0()
	if err != nil {
		return 0
	}

	numericID, err := strconv.Atoi(text)

	if err != nil || numericID <= 0 {
		return 0
	}

	return numericID
}

// ListDevices performs home lookup followed by v1 inventory discovery.
func (c *Client) ListDevices(ctx context.Context, request AccountRequest) (ListDevicesResult, error) {
	home, err := c.GetHome(ctx, request)
	if err != nil {
		return ListDevicesResult{}, err
	}

	data, err := c.GetHomeData(ctx, HomeDataRequest{Auth: request.Auth,
		HomeID:  home.ID,
		Version: HomeDataV1})
	if err != nil {
		return ListDevicesResult{}, err
	}

	return ListDevicesResult{Devices: data.Devices}, nil
}
