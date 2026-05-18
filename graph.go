package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"image/color"

	"go.uber.org/zap"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/text"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

const legendPadding = 8

// Color palette from Grafana's "classic"
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

func (g HourlyGrid) Plot(c draw.Canvas, plt *plot.Plot) {
	trX, _ := plt.Transforms(&c)
	minX, maxX := plt.X.Min, plt.X.Max

	t := time.Unix(int64(minX), 0).Local().Truncate(time.Hour)
	if float64(t.Unix()) < minX {
		t = t.Add(time.Hour)
	}

	for ; float64(t.Unix()) <= maxX; t = t.Add(time.Hour) {
		if t.Hour()%2 == 0 {
			x := trX(float64(t.Unix()))
			c.StrokeLine2(draw.LineStyle{Color: g.Color, Width: g.Width}, x, c.Min.Y, x, c.Max.Y)
		}
	}
}

type HourlyTicker struct{}

func (HourlyTicker) Ticks(min, max float64) []plot.Tick {

	var ticks []plot.Tick

	t := time.Unix(int64(min), 0).Local().Truncate(time.Hour)
	if float64(t.Unix()) < min {
		t = t.Add(time.Hour)
	}

	for ; float64(t.Unix()) <= max; t = t.Add(time.Hour) {
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

func (t CustomYTicker) Ticks(min, max float64) []plot.Tick {
	ticks := plot.DefaultTicks{}.Ticks(min, max)

	for i := range ticks {
		if ticks[i].Label == "" {
			continue // Skip minor ticks
		}

		val := ticks[i].Value
		absVal := math.Abs(val)

		var labelStr string
		if absVal >= 1000000 {
			labelStr = fmt.Sprintf("%.1fM", val/1000000)
		} else if absVal >= 1000 {
			labelStr = fmt.Sprintf("%.1fK", val/1000)
		} else {
			labelStr = fmt.Sprintf("%.0f", val)
		}

		if t.Unit == "ops" {
			labelStr += " ops/s"
		}

		ticks[i].Label = labelStr
	}

	return ticks
}

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

	// Add faint grid lines
	grid := plotter.NewGrid()
	grid.Vertical.Color = color.Transparent
	grid.Horizontal.Color = color.Gray{Y: 220}
	p.Add(grid)

	p.Add(HourlyGrid{
		Color: color.Gray{Y: 220},
		Width: vg.Points(0.5),
	})

	base, _ := url.Parse(lumoConfig.Endpoint)
	base.Path = "/victoriametrics/prometheus/api/v1/query_range"

	var tableRows []TableRow

	globalMaxY := -math.MaxFloat64

	for i, s := range cfg.Series {

		q := base.Query()

		interpolatedExpr := strings.ReplaceAll(s.Expr, "$service_name", lumoConfig.Service)
		interpolatedExpr = strings.ReplaceAll(interpolatedExpr, "$interval", lumoConfig.Interval)

		if lumoConfig.Node != "" {
			interpolatedExpr = strings.ReplaceAll(interpolatedExpr, "$node_name", lumoConfig.Node)
		}

		q.Set("query", interpolatedExpr)
		q.Set("step", "60s")
		q.Set("start", fmt.Sprintf("%d", lumoConfig.Start.Unix()))
		q.Set("end", fmt.Sprintf("%d", lumoConfig.End.Unix()))
		base.RawQuery = q.Encode()

		req, err := http.NewRequest("POST", base.String(), nil)
		if err != nil {
			zap.S().Errorf("error: creating request for %s: %v", s.Legend, err)
			continue
		}

		if lumoConfig.Token != "" {
			req.Header.Set("Authorization", "Bearer "+lumoConfig.Token)
		}

		if lumoConfig.Debug {
			dump, err := httputil.DumpRequestOut(req, true)
			if err == nil {
				zap.S().Debugf("--- DEBUG: HTTP Request (%s) ---\n%s\n---------------------------", s.Legend, dump)
			}
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			zap.S().Errorf("error: failed to query VictoriaMetrics for %s: %v", s.Legend, err)
			continue
		}

		if lumoConfig.Debug {
			dump, err := httputil.DumpResponse(resp, true)
			if err == nil {
				zap.S().Debugf("--- DEBUG: HTTP Response (%s) ---\n%s\n----------------------------", s.Legend, dump)
			}
		}

		if resp.StatusCode != http.StatusOK {
			zap.S().Errorf("error: unexpected HTTP status for %s: %d", s.Legend, resp.StatusCode)
			_ = resp.Body.Close()
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			zap.S().Errorf("error: reading response for %s: %v", s.Legend, err)
			continue
		}

		var vmResp VMResponse
		if err := json.Unmarshal(body, &vmResp); err != nil {
			zap.S().Errorf("error: parsing response for %s: %v", s.Legend, err)
			continue
		}

		zap.S().Debugf("--- DEBUG: RAW METRICS (%s) ---\n%+v\n---------------------------", s.Legend, vmResp)

		if vmResp.Status != "success" || len(vmResp.Data.Result) == 0 {
			zap.S().Errorf("warning: no data returned for %s", s.Legend)
			continue
		}

		for resultIdx, result := range vmResp.Data.Result {

			minVal := math.MaxFloat64
			maxVal := -math.MaxFloat64
			sumVal := 0.0
			countVal := 0.0

			pts := make(plotter.XYs, 0, len(result.Values))

			for _, v := range result.Values {

				if len(v) != 2 {
					zap.S().Errorf("error: not enough values")
					continue
				}

				t, ok := v[0].(float64)
				if !ok {
					zap.S().Errorf("error: could not parse timestamp")
					continue
				}

				valStr, ok := v[1].(string)
				if !ok {
					zap.S().Errorf("error: could not parse string value")
					continue
				}

				val, err := strconv.ParseFloat(valStr, 64)
				if err != nil {
					zap.S().Errorf("error: could not parse float value")
					continue
				}

				if val < minVal {
					minVal = val
				}

				if val > maxVal {
					maxVal = val
				}

				if val > globalMaxY {
					globalMaxY = val
				}

				sumVal += val
				countVal++

				pts = append(pts, plotter.XY{X: t, Y: val})
			}

			if len(pts) < 1 {
				zap.S().Errorf("error: no xy points")
				continue
			}

			// Generate interpolated legend
			legendText := s.Legend
			for key, val := range result.Metric {
				legendText = strings.ReplaceAll(legendText, "{{ "+key+" }}", val)
			}

			zap.S().Debugf("--- DEBUG: PARSED METRICS (%s) ---\n%+v\n---------------------------", legendText, pts)

			line, err := plotter.NewLine(pts)
			if err != nil {
				zap.S().Errorf("error: creating line for %s: %v", legendText, err)
				continue
			}

			seriesColor := Palette[(i+resultIdx)%len(Palette)]
			line.Color = seriesColor

			// Create polygon points for filling the area under the line
			polyPts := make(plotter.XYs, len(pts)+2)

			// Start point at (X[0], 0)
			polyPts[0] = plotter.XY{X: pts[0].X, Y: 0}

			// Copy data points
			for j, pt := range pts {
				polyPts[j+1] = pt
			}

			// End point at (X[len-1], 0)
			polyPts[len(polyPts)-1] = plotter.XY{X: pts[len(pts)-1].X, Y: 0}

			poly, err := plotter.NewPolygon(polyPts)
			if err == nil {
				// Extract RGBA from seriesColor and add alpha transparency
				r, g, b, _ := seriesColor.RGBA()
				poly.Color = color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 32} // ~12.5% opaque
				poly.Width = 0                                                         // No border on the polygon
				p.Add(poly)
			}

			p.Add(line)

			avgVal := 0.0
			if countVal > 0 {
				avgVal = sumVal / countVal
			}

			tableRows = append(tableRows, TableRow{
				Legend: legendText,
				Color:  seriesColor,
				Min:    minVal,
				Max:    maxVal,
				Avg:    avgVal,
			})
		}
	}

	if globalMaxY != -math.MaxFloat64 {

		// Add 10% padding to the max Y value
		padding := math.Abs(globalMaxY) * 0.10
		if globalMaxY == 0 {
			padding = 1.0 // Ensure at least some graph area if all lines are exactly 0
		}

		p.Y.Max = globalMaxY + padding

		p.Add(HLine{
			Y:     p.Y.Max,
			Color: color.Gray{Y: 220},
			Width: vg.Points(0.5),
		})
	}

	// Sort table rows by Average descending
	sort.Slice(tableRows, func(i, j int) bool {
		return tableRows[i].Avg > tableRows[j].Avg
	})

	tableOffset := vg.Points(15*float64(len(tableRows)) + 30)

	c := vgimg.New(12*vg.Inch, 4*vg.Inch+tableOffset)
	dc := draw.New(c)

	plotCanvas := draw.Crop(dc, 0, 0, tableOffset, 0)
	p.Draw(plotCanvas)

	// Draw underline for the title
	if cfg.Title != "" {
		titleWidth := p.Title.TextStyle.Width(cfg.Title)
		centerX := plotCanvas.Max.X / 2
		// The title is drawn at the top center. We calculate its bottom Y coordinate.
		// plotCanvas.Max.Y is the top. The title takes up its ascent + descent.
		// In gonum, p.Title.TextStyle is used to draw.
		extents := p.Title.TextStyle.FontExtents()
		lineY := plotCanvas.Max.Y - extents.Ascent - extents.Descent

		underlineStyle := draw.LineStyle{
			Color: color.Black,
			Width: vg.Points(1),
		}
		plotCanvas.StrokeLine2(underlineStyle, centerX-titleWidth/2, lineY, centerX+titleWidth/2, lineY)
	}

	tableCanvas := draw.Crop(dc, 0, 0, 0, -(4 * vg.Inch))

	styleNormal := text.Style{
		Font:    mediumFont,
		Color:   color.Black,
		Handler: plot.DefaultTextHandler,
	}

	styleBold := text.Style{
		Font:    boldFont,
		Color:   color.Black,
		Handler: plot.DefaultTextHandler,
	}

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

	sepStyle := draw.LineStyle{
		Color: color.Gray{Y: 220},
		Width: vg.Points(0.5),
	}

	for _, row := range tableRows {

		// Draw separator line above the row
		lineY := y - vg.Points(4)
		tableCanvas.StrokeLine2(sepStyle, colColor, lineY, tableCanvas.Max.X, lineY)

		y -= vg.Points(15)

		boxSize := vg.Points(8)
		boxRect := []vg.Point{
			{X: colColor, Y: y},
			{X: colColor + boxSize, Y: y},
			{X: colColor + boxSize, Y: y + boxSize},
			{X: colColor, Y: y + boxSize},
		}
		tableCanvas.FillPolygon(row.Color, boxRect)

		tableCanvas.FillText(styleNormal, vg.Point{X: colName, Y: y}, row.Legend)

		if row.Min <= row.Max {
			tableCanvas.FillText(styleNormal, vg.Point{X: colMin, Y: y}, formatValue(row.Min))
			tableCanvas.FillText(styleNormal, vg.Point{X: colMax, Y: y}, formatValue(row.Max))
			tableCanvas.FillText(styleNormal, vg.Point{X: colAvg, Y: y}, formatValue(row.Avg))
		}
	}

	f, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}

	defer func() { _ = f.Close() }()

	png := vgimg.PngCanvas{Canvas: c}
	if _, err := png.WriteTo(f); err != nil {
		return fmt.Errorf("saving plot to png: %w", err)
	}

	zap.S().Infof("Saved chart to %s", output)

	return nil
}
