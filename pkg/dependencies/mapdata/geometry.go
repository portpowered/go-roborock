package mapdata

import (
	"fmt"
	"math"

	"github.com/portpowered/go-roborock/internal/mapmodel"
)

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func calibrated(grid mapmodel.MapGrid) error {
	if grid.Origin == nil || grid.Resolution == nil || !finite(*grid.Resolution) || *grid.Resolution <= 0 ||
		!finite(grid.Origin.X) || !finite(grid.Origin.Y) || (grid.RowYDirection != 1 && grid.RowYDirection != -1) {
		return fmt.Errorf("grid calibration unavailable: %w", ErrMalformed)
	}

	_, err := cells(grid.Width, grid.Height)

	return err
}

// MapToPixel converts source-frame world coordinates to displayed top-down pixels.
func MapToPixel(grid mapmodel.MapGrid, point mapmodel.MapPoint) (mapmodel.MapPoint, error) {
	err := calibrated(grid)
	if err != nil {
		return mapmodel.MapPoint{}, err
	}

	if !finite(point.X) || !finite(point.Y) {
		return mapmodel.MapPoint{}, fmt.Errorf("non-finite point: %w", ErrMalformed)
	}

	result := mapmodel.MapPoint{
		X: (point.X - grid.Origin.X) / *grid.Resolution,
		Y: (point.Y - grid.Origin.Y) / *grid.Resolution / float64(grid.RowYDirection),
	}
	if !grid.TopDown {
		result.Y = float64(grid.Height-1) - result.Y
	}

	if !finite(result.X) || !finite(result.Y) {
		return mapmodel.MapPoint{}, fmt.Errorf("point overflow: %w", ErrMalformed)
	}

	return result, nil
}

// PixelToMap reverses MapToPixel without rounding or changing firmware frames.
func PixelToMap(grid mapmodel.MapGrid, point mapmodel.MapPoint) (mapmodel.MapPoint, error) {
	err := calibrated(grid)
	if err != nil {
		return mapmodel.MapPoint{}, err
	}

	if !finite(point.X) || !finite(point.Y) {
		return mapmodel.MapPoint{}, fmt.Errorf("non-finite point: %w", ErrMalformed)
	}

	rowY := point.Y
	if !grid.TopDown {
		rowY = float64(grid.Height-1) - rowY
	}

	result := mapmodel.MapPoint{
		X: grid.Origin.X + point.X**grid.Resolution,
		Y: grid.Origin.Y + rowY**grid.Resolution*float64(grid.RowYDirection),
	}
	if !finite(result.X) || !finite(result.Y) {
		return mapmodel.MapPoint{}, fmt.Errorf("point overflow: %w", ErrMalformed)
	}

	return result, nil
}

// PixelRectangleToMap validates a displayed raster rectangle and converts its corners.
func PixelRectangleToMap(grid mapmodel.MapGrid, rectangle mapmodel.MapRectangle) (mapmodel.MapRectangle, error) {
	if rectangle.Min.X < 0 || rectangle.Min.Y < 0 || rectangle.Max.X > float64(grid.Width-1) ||
		rectangle.Max.Y > float64(grid.Height-1) || rectangle.Min.X >= rectangle.Max.X || rectangle.Min.Y >= rectangle.Max.Y {
		return mapmodel.MapRectangle{}, fmt.Errorf("rectangle outside grid or degenerate: %w", ErrMalformed)
	}

	minimum, err := PixelToMap(grid, rectangle.Min)
	if err != nil {
		return mapmodel.MapRectangle{}, err
	}

	maximum, err := PixelToMap(grid, rectangle.Max)
	if err != nil {
		return mapmodel.MapRectangle{}, err
	}

	return mapmodel.MapRectangle{
		Min: mapmodel.MapPoint{
			X: math.Min(minimum.X,
				maximum.X),
			Y: math.Min(minimum.Y,
				maximum.Y),
		},
		Max: mapmodel.MapPoint{
			X: math.Max(minimum.X,
				maximum.X),
			Y: math.Max(minimum.Y,
				maximum.Y),
		},
	}, nil
}
