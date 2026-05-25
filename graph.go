package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"math"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/text"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

const legendPadding = 8

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
		default:
			labelStr = fmt.Sprintf("%.0f", val)
		}

		if t.Unit == "ops" {
			labelStr += " ops/s"
		}

		ticks[i].Label = labelStr
	}

	return ticks
}

func fetchSeries(lumoConfig *LumoConfig, expr, legend string) (*VMResponse, error) {

	// Handle trailing slash in endpoint URL
	urlPath, err := url.JoinPath(strings.TrimRight(lumoConfig.Endpoint, "/"), "victoriametrics/prometheus/api/v1/query_range")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateRequest, err)
	}

	q := url.Values{}
	q.Set("query", interpolateGraphConfig(expr, lumoConfig))
	q.Set("step", lumoConfig.Interval)
	q.Set("start", strconv.FormatInt(lumoConfig.Start.Unix(), 10))
	q.Set("end", strconv.FormatInt(lumoConfig.End.Unix(), 10))

	urlPath += "?" + q.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlPath, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateRequest, err)
	}

	if lumoConfig.Token != "" {
		req.Header.Set("Authorization", "Bearer "+lumoConfig.Token)
	}

	if lumoConfig.Debug {
		dump, err := httputil.DumpRequestOut(req, true)
		if err == nil {
			zap.S().Debugf("--- DEBUG: HTTP Request (%s) ---\n%s\n---------------------------", legend, dump)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrExecRequest, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if lumoConfig.Debug {
		dump, err := httputil.DumpResponse(resp, true)
		if err == nil {
			zap.S().Debugf("--- DEBUG: HTTP Response (%s) ---\n%s\n----------------------------", legend, dump)
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedHTTPStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReadResponse, err)
	}

	var vmResp VMResponse
	if err := json.Unmarshal(body, &vmResp); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParsingJSON, err)
	}

	if vmResp.Status != "success" {
		return nil, fmt.Errorf("%w: %s", ErrAPIStatus, vmResp.Status)
	}

	return &vmResp, nil
}

type SeriesData struct {
	Points plotter.XYs
	Min    float64
	Max    float64
	Sum    float64
	Count  float64
}

func parseSeriesData(values [][]interface{}) (SeriesData, error) {

	seriesData := SeriesData{
		Points: make(plotter.XYs, 0, len(values)),
		Min:    math.MaxFloat64,
		Max:    -math.MaxFloat64,
		Sum:    0.0,
		Count:  0.0,
	}

	// Loop over the values from the series and mangle
	for _, v := range values {

		if len(v) != 2 {
			return seriesData, fmt.Errorf("%w: invalid value length", ErrInvalidValueLength)
		}

		t, ok1 := v[0].(float64)
		valStr, ok2 := v[1].(string)

		if !ok1 || !ok2 {
			return seriesData, fmt.Errorf("%w: invalid value type", ErrInvalidValueType)
		}

		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			return seriesData, fmt.Errorf("%w: invalid value type", ErrInvalidValueType)
		}

		if val < seriesData.Min {
			seriesData.Min = val
		}

		if val > seriesData.Max {
			seriesData.Max = val
		}

		seriesData.Sum += val
		seriesData.Count++

		seriesData.Points = append(seriesData.Points, plotter.XY{X: t, Y: val})
	}

	if seriesData.Count == 0 {
		return seriesData, fmt.Errorf("%w: no valid points", ErrNoValidPoints)
	}

	return seriesData, nil
}

// addVisualSeries is responsible for taking the XY points and adding them to the graph image
func addVisualSeries(p *plot.Plot, seriesData SeriesData, baseColor color.Color) {

	line, err := plotter.NewLine(seriesData.Points)
	if err != nil {
		zap.S().Warnf("warning: creating visual series line: %v", err)
	}

	line.Color = baseColor

	p.Add(line)

	polyPts := make(plotter.XYs, len(seriesData.Points)+2)
	polyPts[0] = plotter.XY{X: seriesData.Points[0].X, Y: 0}

	for j, pt := range seriesData.Points {
		polyPts[j+1] = pt
	}

	polyPts[len(polyPts)-1] = plotter.XY{X: seriesData.Points[len(seriesData.Points)-1].X, Y: 0}

	poly, err := plotter.NewPolygon(polyPts)
	if err != nil {
		zap.S().Warnf("warning: creating visual series polygon: %v", err)
	}

	r, g, b, _ := baseColor.RGBA()
	poly.Color = color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 32} // #nosec
	poly.Width = 0

	p.Add(poly)
}

// drawLegendTable draws each of the 'rows' within the table-legend
func drawLegendTable(tableCanvas draw.Canvas, rows []TableRow) {

	styleNormal := text.Style{Font: mediumFont, Color: color.Black, Handler: plot.DefaultTextHandler}
	styleBold := text.Style{Font: boldFont, Color: color.Black, Handler: plot.DefaultTextHandler}

	colColor := vg.Points(10)
	colName := vg.Points(30)
	colMin := tableCanvas.Max.X * 0.55
	colMax := tableCanvas.Max.X * 0.70
	colAvg := tableCanvas.Max.X * 0.85

	y := tableCanvas.Max.Y - vg.Points(15)
	tableCanvas.FillText(styleBold, vg.Point{X: colName, Y: y}, "Name")
	tableCanvas.FillText(styleBold, vg.Point{X: colMin, Y: y}, "Min")
	tableCanvas.FillText(styleBold, vg.Point{X: colMax, Y: y}, "Max")
	tableCanvas.FillText(styleBold, vg.Point{X: colAvg, Y: y}, "Average")

	sepStyle := draw.LineStyle{Color: color.Gray{Y: 220}, Width: vg.Points(0.5)}

	for _, row := range rows {

		lineY := y - vg.Points(4)
		tableCanvas.StrokeLine2(sepStyle, colColor, lineY, tableCanvas.Max.X, lineY)

		y -= vg.Points(15)

		boxSize := vg.Points(8)
		tableCanvas.FillPolygon(row.Color, []vg.Point{
			{X: colColor, Y: y}, {X: colColor + boxSize, Y: y},
			{X: colColor + boxSize, Y: y + boxSize}, {X: colColor, Y: y + boxSize},
		})

		tableCanvas.FillText(styleNormal, vg.Point{X: colName, Y: y}, row.Legend)

		if row.Min <= row.Max {
			tableCanvas.FillText(styleNormal, vg.Point{X: colMin, Y: y}, formatValue(row.Min))
			tableCanvas.FillText(styleNormal, vg.Point{X: colMax, Y: y}, formatValue(row.Max))
			tableCanvas.FillText(styleNormal, vg.Point{X: colAvg, Y: y}, formatValue(row.Avg))
		}
	}
}

// generateGraph is the main function coordinating creating the raw graph image,
// fetching series from PMM, parsing returned values, and adding series lines to the image.
func generateGraph(lumoConfig *LumoConfig, cfg *GraphConfig, output string) error {

	p := plot.New()

	p.Title.Text = cfg.Title
	p.Title.Padding = 20
	p.X.Tick.Marker = plot.TimeTicks{
		Ticker: HourlyTicker{},
		Format: "15:04",
		Time:   plot.UnixTimeIn(time.Local),
	}
	p.Y.Tick.Marker = CustomYTicker{Unit: cfg.Unit}
	p.Legend.Padding = legendPadding

	grid := plotter.NewGrid()
	grid.Vertical.Color = color.Transparent
	grid.Horizontal.Color = color.Gray{Y: 220}

	p.Add(grid)
	p.Add(HourlyGrid{Color: color.Gray{Y: 220}, Width: vg.Points(0.5)})

	var tableRows []TableRow

	globalMaxY := -math.MaxFloat64

	// Loop over each series in this graph
	for i, s := range cfg.Series {

		// Fetch series from PMM
		vmResp, err := fetchSeries(lumoConfig, s.Expr, s.Legend)
		if err != nil {
			zap.S().Errorf("error: query failed for %s: %v", s.Legend, err)
			continue
		}

		if len(vmResp.Data.Result) == 0 {
			zap.S().Warnf("warning: no data returned for %s", s.Legend)
			continue
		}

		// Loop over results from PMM
		for resultIdx, result := range vmResp.Data.Result {

			seriesData, err := parseSeriesData(result.Values)
			if err != nil {
				continue
			}

			if seriesData.Max > globalMaxY {
				globalMaxY = seriesData.Max
			}

			legendText := s.Legend
			for key, val := range result.Metric {
				re := regexp.MustCompile("\\{\\{\\s*" + regexp.QuoteMeta(key) + "\\s*\\}\\}")
				legendText = re.ReplaceAllString(legendText, val)
			}

			seriesColor := Palette[(i+resultIdx)%len(Palette)]
			addVisualSeries(p, seriesData, seriesColor)

			tableRows = append(tableRows, TableRow{
				Legend: legendText,
				Color:  seriesColor,
				Min:    seriesData.Min,
				Max:    seriesData.Max,
				Avg:    seriesData.Sum / seriesData.Count,
			})
		}
	}

	if globalMaxY != -math.MaxFloat64 {

		padding := math.Max(math.Abs(globalMaxY)*0.10, 1.0)

		p.Y.Max = globalMaxY + padding
		p.Add(HLine{Y: p.Y.Max, Color: color.Gray{Y: 220}, Width: vg.Points(0.5)})
	}

	// Sort the table-legend by average, descending
	sort.Slice(tableRows, func(i, j int) bool {
		return tableRows[i].Avg > tableRows[j].Avg
	})

	// Offset the start of the table 30px below the graph area
	tableOffset := vg.Points(15*float64(len(tableRows)) + 30)
	c := vgimg.New(12*vg.Inch, 4*vg.Inch+tableOffset)
	dc := draw.New(c)

	// Resize the canvas
	plotCanvas := draw.Crop(dc, 0, 0, tableOffset, 0)
	p.Draw(plotCanvas)

	// Underline the title
	if cfg.Title != "" {
		titleWidth := p.Title.TextStyle.Width(cfg.Title)
		extents := p.Title.TextStyle.FontExtents()
		lineY := plotCanvas.Max.Y - extents.Ascent - extents.Descent
		plotCanvas.StrokeLine2(draw.LineStyle{Color: color.Black, Width: vg.Points(1)},
			(plotCanvas.Max.X/2)-titleWidth/2, lineY, (plotCanvas.Max.X/2)+titleWidth/2, lineY)
	}

	drawLegendTable(draw.Crop(dc, 0, 0, 0, -(4*vg.Inch)), tableRows)

	f, err := os.Create(output) // #nosec
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCreateOutput, err)
	}

	defer func() { _ = f.Close() }()

	if _, err := (vgimg.PngCanvas{Canvas: c}).WriteTo(f); err != nil {
		return fmt.Errorf("%w: %w", ErrSavePlot, err)
	}

	zap.S().Infof("Saved chart to %s", output)

	return nil
}
