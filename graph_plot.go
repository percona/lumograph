package main

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

const (
	legendPadding = 8

	ops_str = "ops"
)

// Palette is a slice of Color{}s derived from Grafana's "classic" palette
var Palette = []color.Color{
	color.RGBA{R: 0x73, G: 0xBF, B: 0x69, A: 255}, // rgb(115, 191, 105)
	color.RGBA{R: 0xF2, G: 0xCC, B: 0x0C, A: 255}, // rgb(242, 204, 12)
	color.RGBA{R: 0x8A, G: 0xB8, B: 0xFF, A: 255}, // rgb(138, 184, 255)
	color.RGBA{R: 0xFF, G: 0x78, B: 0x0A, A: 255}, // rgb(255, 120, 10)
	color.RGBA{R: 0xF2, G: 0x49, B: 0x5C, A: 255}, // rgb(242, 73, 92)
	color.RGBA{R: 0x57, G: 0x94, B: 0xF2, A: 255}, // rgb(87, 148, 242)
	color.RGBA{R: 0xB8, G: 0x77, B: 0xD9, A: 255}, // rgb(184, 119, 217)
	color.RGBA{R: 0x70, G: 0x5D, B: 0xA0, A: 255}, // rgb(112, 93, 160)
	color.RGBA{R: 0x37, G: 0x87, B: 0x2D, A: 255}, // rgb(55, 135, 45)
	color.RGBA{R: 0xFA, G: 0xDE, B: 0x2A, A: 255}, // rgb(250, 222, 42)
}

type HourlyGrid struct {
	Color color.Color
	Width vg.Length
}

func (g HourlyGrid) Plot(canvas draw.Canvas, plt *plot.Plot) {

	trX, _ := plt.Transforms(&canvas)
	minX, maxX := plt.X.Min, plt.X.Max

	t := time.Unix(int64(minX), 0).Local().Truncate(time.Hour)
	if float64(t.Unix()) < minX {
		t = t.Add(time.Hour)
	}

	for ; float64(t.Unix()) <= maxX; t = t.Add(time.Hour) {
		if t.Hour()%2 == 0 {
			x := trX(float64(t.Unix()))
			canvas.StrokeLine2(draw.LineStyle{Color: g.Color, Width: g.Width}, x, canvas.Min.Y, x, canvas.Max.Y)
		}
	}
}

type HourlyTicker struct{}

func (HourlyTicker) Ticks(minVal, maxVal float64) []plot.Tick {

	var ticks []plot.Tick

	t := time.Unix(int64(minVal), 0).Local().Truncate(time.Hour)
	if float64(t.Unix()) < minVal {
		t = t.Add(time.Hour)
	}

	for ; float64(t.Unix()) <= maxVal; t = t.Add(time.Hour) {
		if t.Hour()%2 == 0 {
			// Provide a non-empty string so it's treated as a major tick
			ticks = append(ticks, plot.Tick{Value: float64(t.Unix()), Label: " "})
		}
	}

	return ticks
}

type HLine struct {
	Y     float64
	Color color.Color
	Width vg.Length
}

func (l HLine) Plot(c draw.Canvas, plt *plot.Plot) {
	_, trY := plt.Transforms(&c)
	y := trY(l.Y)
	c.StrokeLine2(draw.LineStyle{Color: l.Color, Width: l.Width}, c.Min.X, y, c.Max.X, y)
}

type CustomYTicker struct {
	Unit string
}

func (t CustomYTicker) Ticks(minVal, maxVal float64) []plot.Tick {

	ticks := plot.DefaultTicks{}.Ticks(minVal, maxVal)
	for i := range ticks {
		if ticks[i].Label == "" {
			continue // Skip minor ticks
		}

		val := ticks[i].Value
		absVal := math.Abs(val)

		var labelStr string

		switch {
		case absVal >= 1000000:
			labelStr = fmt.Sprintf("%.1fM", val/1000000)
		case absVal >= 1000:
			labelStr = fmt.Sprintf("%.1fK", val/1000)
		case absVal <= 1 && maxVal <= 1:
			labelStr = fmt.Sprintf("%.4f", val)
		default:
			labelStr = fmt.Sprintf("%.0f", val)
		}

		if t.Unit == ops_str {
			labelStr += " ops/s"
		}

		ticks[i].Label = labelStr
	}

	return ticks
}

// SeriesData represents the data for a single series
type SeriesData struct {
	Points plotter.XYs
	Min    float64
	Max    float64
	Sum    float64
	Count  float64
}

// parseSeriesData parses the values from the VMResponse and returns a SeriesData struct
func parseSeriesData(values [][]interface{}) (SeriesData, error) {

	seriesData := SeriesData{
		Points: make(plotter.XYs, 0, len(values)),
		Min:    math.MaxFloat64,
		Max:    -math.MaxFloat64,
		Sum:    0.0,
		Count:  0.0,
	}

	for _, v := range values {

		if len(v) != 2 {
			return seriesData, fmt.Errorf("%w: invalid value length", ErrInvalidValueLength)
		}

		// Parse the timestamp and value from the VMResponse
		t, ok1 := v[0].(float64)
		valStr, ok2 := v[1].(string)

		// If the timestamp or value is not a float64 or string, return an error
		if !ok1 || !ok2 {
			return seriesData, fmt.Errorf("%w: invalid value type", ErrInvalidValueType)
		}

		// Parse the value from the string to a float64
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			return seriesData, fmt.Errorf("%w: invalid value type", ErrInvalidValueType)
		}

		// Prometheus may return +Inf, -Inf, or NaN; gonum plotter cannot render these.
		if math.IsInf(val, 0) || math.IsNaN(val) {
			continue
		}

		// Update the min value for this series
		if val < seriesData.Min {
			seriesData.Min = val
		}

		// Update the max value for this series
		if val > seriesData.Max {
			seriesData.Max = val
		}

		// Update the sum, and count for this series to calculate the average
		seriesData.Sum += val
		seriesData.Count++

		// Append the timestamp and value to the series data points
		seriesData.Points = append(seriesData.Points, plotter.XY{X: t, Y: val})
	}

	if seriesData.Count == 0 {
		return seriesData, fmt.Errorf("%w: no valid points", ErrNoValidPoints)
	}

	return seriesData, nil
}

// addVisualSeries creates the line and fill polygon for the series and adds them to the image.
func addVisualSeries(p *plot.Plot, seriesData SeriesData, baseColor color.Color) error {

	// Create the line for the series
	line, err := plotter.NewLine(seriesData.Points)
	if err != nil {
		return fmt.Errorf("%w: creating visual series line", err)
	}

	// Set the color of the line
	line.Color = baseColor

	// Add the line to the image
	p.Add(line)

	// Initialize the fill polygon for the series
	polyPts := make(plotter.XYs, len(seriesData.Points)+2)

	// Set the first point of the fill polygon to the first point of the series
	polyPts[0] = plotter.XY{X: seriesData.Points[0].X, Y: 0}

	// Set the remaining points of the fill polygon to the points of the series
	for j, pt := range seriesData.Points {
		polyPts[j+1] = pt
	}

	// Set the last point of the fill polygon to the last point of the series
	polyPts[len(polyPts)-1] = plotter.XY{X: seriesData.Points[len(seriesData.Points)-1].X, Y: 0}

	// Create the fill polygon for the series
	poly, err := plotter.NewPolygon(polyPts)
	if err != nil {
		return fmt.Errorf("%w: creating visual series polygon", err)
	}

	// Set the color of the fill polygon
	r, g, b, _ := baseColor.RGBA()
	poly.Color = color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 32} // #nosec
	poly.Width = 0

	// Add the fill polygon to the plot
	p.Add(poly)

	return nil
}
