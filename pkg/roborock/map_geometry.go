package roborock

import (
	"github.com/portpowered/go-roborock/pkg/dependencies/mapdata"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// MapToPixel projects a source-world point into the map raster using explicit calibration.
func MapToPixel(grid MapGrid, point MapPoint) (MapPoint, error) {
	result, err := mapdata.MapToPixel(grid, point)
	if err != nil {
		return MapPoint{}, roborockerrors.New(roborockerrors.InvalidArgument, "MapToPixel", "invalid map geometry", err)
	}

	return result, nil
}

// PixelToMap converts a raster position to the source map's documented coordinate frame and unit.
func PixelToMap(grid MapGrid, point MapPoint) (MapPoint, error) {
	result, err := mapdata.PixelToMap(grid, point)
	if err != nil {
		return MapPoint{}, roborockerrors.New(roborockerrors.InvalidArgument, "PixelToMap", "invalid map geometry", err)
	}

	return result, nil
}

// PixelRectangleToMap converts both corners and orders the source-world rectangle.
// Inspect grid.Frame and grid.Unit before using the result for a cleaning command.
func PixelRectangleToMap(grid MapGrid, rectangle MapRectangle) (MapRectangle, error) {
	result, err := mapdata.PixelRectangleToMap(grid, rectangle)
	if err != nil {
		return MapRectangle{}, roborockerrors.New(roborockerrors.InvalidArgument,
			"PixelRectangleToMap", "invalid map geometry", err)
	}

	return result, nil
}
