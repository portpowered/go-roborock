package roborock

import (
	"context"
	"encoding/json"

	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

// GetHome returns the Roborock home identifier required by inventory requests.
func (c *Client) GetHome(ctx context.Context, r AccountRequest) (Home, error) {
	out, err := c.rest.HomeDetail(ctx, rest.HomeDetailRequest{Auth: authWire(r.Auth)})
	if err != nil {
		return Home{}, err
	}
	return Home{ID: out.Home.RrHomeId, Name: valueOrZero(out.Home.Name)}, nil
}

// GetHomeData joins device and product identity and includes shared devices.
// Unknown protocols are preserved; opening an unsupported version returns a typed error.
func (c *Client) GetHomeData(ctx context.Context, r HomeDataRequest) (HomeData, error) {
	version := r.Version
	if version == 0 {
		version = HomeDataV1
	}
	out, err := c.rest.HomeData(ctx, rest.HomeDataRequest{Auth: authWire(r.Auth), HomeID: r.HomeID, Version: int(version)})
	if err != nil {
		return HomeData{}, err
	}
	h := out.Home
	products := make(map[string]dependencymodels.HomeProduct)
	for _, p := range valueOrZero(h.Products) {
		products[p.Id] = p
	}
	result := HomeData{Home: Home{ID: h.Id, Name: h.Name}, Devices: []Device{}, Rooms: []Room{}}
	seen := make(map[string]bool)
	for _, d := range valueOrZero(h.Devices) {
		result.Devices = append(result.Devices, deviceProjection(d, products[d.ProductId], false))
		seen[d.Duid] = true
	}
	for _, d := range valueOrZero(h.ReceivedDevices) {
		if !seen[d.Duid] {
			result.Devices = append(result.Devices, deviceProjection(d, products[d.ProductId], true))
			seen[d.Duid] = true
		}
	}
	for _, room := range valueOrZero(h.Rooms) {
		result.Rooms = append(result.Rooms, Room{ID: room.Id, Name: room.Name})
	}
	return result, nil
}

func deviceProjection(d dependencymodels.HomeDevice, p dependencymodels.HomeProduct, shared bool) Device {
	protocol := ProtocolVersion(valueOrZero(d.Pv))
	if d.Pv == nil {
		protocol = ProtocolV1
	}
	result := Device{ID: d.Duid, Name: d.Name, LocalKey: d.LocalKey, Model: p.Model, ProductID: d.ProductId, Protocol: protocol, Online: valueOrZero(d.Online), Shared: shared, Firmware: d.Fv}
	if d.RoomId != nil {
		var roomID int64
		if json.Unmarshal(*d.RoomId, &roomID) == nil {
			result.RoomID = &roomID
		}
	}
	return result
}

// ListDevices performs home lookup followed by v1 inventory discovery.
func (c *Client) ListDevices(ctx context.Context, r AccountRequest) (ListDevicesResult, error) {
	h, err := c.GetHome(ctx, r)
	if err != nil {
		return ListDevicesResult{}, err
	}
	data, err := c.GetHomeData(ctx, HomeDataRequest{Auth: r.Auth, HomeID: h.ID, Version: HomeDataV1})
	if err != nil {
		return ListDevicesResult{}, err
	}
	return ListDevicesResult{Devices: data.Devices}, nil
}
