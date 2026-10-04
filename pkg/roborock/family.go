package roborock

import (
	"context"
	"strings"

	"github.com/portpowered/go-roborock/internal/protocol"

	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

type deviceMetadata struct {
	family        DeviceFamily
	serial, model string
}

func productFamily(version ProtocolVersion, model, category string) DeviceFamily {
	switch version {
	case ProtocolV1:
		if category == protocol.DeviceCategoryVacuum {
			return FamilyV1Vacuum
		}
	case ProtocolA01:
		switch category {
		case protocol.DeviceCategoryWetDry:
			return FamilyDyad
		case protocol.DeviceCategoryWasher:
			return FamilyZeo
		}
	case ProtocolB01:
		parts := strings.Split(model, ".")

		suffix := parts[len(parts)-1]
		if strings.Contains(suffix, protocol.DeviceQ10ModelPrefix) {
			return FamilyB01Q10
		}

		if strings.Contains(suffix, protocol.DeviceQ7ModelPrefix) {
			return FamilyB01Q7
		}
	case ProtocolL01:
		return FamilyUnknown
	}

	return FamilyUnknown
}

func (s *DeviceSession) deviceFamily() DeviceFamily {
	if s.family != "" {
		return s.family
	}

	if s.protocol == string(ProtocolV1) {
		return FamilyV1Vacuum
	}

	return FamilyUnknown
}

func (c *Client) openMetadata(ctx context.Context, request OpenDeviceRequest) (deviceMetadata, error) {
	if request.Protocol != ProtocolB01 {
		family := FamilyUnknown
		if request.Protocol == ProtocolV1 {
			family = FamilyV1Vacuum
		}

		return deviceMetadata{family: family, serial: "", model: ""}, nil
	}

	home, err := c.GetHome(ctx, AccountRequest{Auth: request.Auth})
	if err != nil {
		return deviceMetadata{}, err
	}

	data, err := c.rest.HomeData(ctx, rest.HomeDataRequest{Auth: authWire(request.Auth),
		HomeID: home.ID, Version: int(HomeDataV1)})
	if err != nil {
		return deviceMetadata{}, operationError("open_device", err)
	}

	return metadataFromHome(data.Home, request.DeviceID)
}

func metadataFromHome(home dependencymodels.HomeData, deviceID string) (deviceMetadata, error) {
	products := make(map[string]dependencymodels.HomeProduct)
	for _, product := range valueOrZero(home.Products) {
		products[product.Id] = product
	}

	for _, device := range valueOrZero(home.Devices) {
		if device.Duid == deviceID {
			return metadataFromDevice(device, products[device.ProductId])
		}
	}

	for _, device := range valueOrZero(home.ReceivedDevices) {
		if device.Duid == deviceID {
			return metadataFromDevice(device, products[device.ProductId])
		}
	}

	return deviceMetadata{}, roborockerrors.New(roborockerrors.NotFound,
		"open_device", "device missing from account inventory", nil)
}

func metadataFromDevice(
	device dependencymodels.HomeDevice, product dependencymodels.HomeProduct,
) (deviceMetadata, error) {
	family := productFamily(ProtocolVersion(valueOrZero(device.Pv)), product.Model, product.Category)
	if family != FamilyB01Q7 && family != FamilyB01Q10 {
		return deviceMetadata{}, unsupportedMap("open_device")
	}

	metadata := deviceMetadata{family: family, serial: valueOrZero(device.Sn), model: product.Model}
	if family == FamilyB01Q7 && metadata.serial == "" {
		return deviceMetadata{}, roborockerrors.New(roborockerrors.Protocol,
			"open_device", "Q7 inventory omits serial required for map decryption", nil)
	}

	return metadata, nil
}

// GetCapabilities reports supported operations independently of the implementation family.
func (s *DeviceSession) GetCapabilities(ctx context.Context, _ EmptyRequest) (DeviceCapabilities, error) {
	ctx, finish := s.operationContext(ctx)
	defer finish()

	err := ctx.Err()
	if err != nil {
		return DeviceCapabilities{}, operationError("GetCapabilities", err)
	}

	var capabilities DeviceCapabilities

	switch s.deviceFamily() {
	case FamilyV1Vacuum:
		capabilities = DeviceCapabilities{MapContent: true, MapList: true, MapRooms: true,
			MapSelection: true, MapTrace: false, RoomCleaning: true, ZoneCleaning: true}
	case FamilyB01Q7:
		capabilities = DeviceCapabilities{MapContent: true, MapList: true, MapRooms: true,
			MapSelection: false, MapTrace: false, RoomCleaning: true, ZoneCleaning: false}
	case FamilyB01Q10:
		capabilities = DeviceCapabilities{MapContent: true, MapList: true, MapRooms: true,
			MapSelection: false, MapTrace: true, RoomCleaning: true, ZoneCleaning: true}
	case FamilyDyad, FamilyZeo, FamilyUnknown:
		return capabilities, nil
	}

	return capabilities, nil
}
