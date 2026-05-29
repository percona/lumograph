package main

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"regexp"
	"sort"
	"time"

	"go.uber.org/zap"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

// generateGraph is responsible for creating the plot, adding the grid, and drawing the legend table
func generateGraph(lumoConfig *LumoConfig, cfg *GraphConfig, output string) error {

	// Create a new plot
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

	// Create a new grid for the plot
	grid := plotter.NewGrid()
	grid.Vertical.Color = color.Transparent
	grid.Horizontal.Color = color.Gray{Y: 220}

	// Add the grid to the plot
	p.Add(grid)
	p.Add(HourlyGrid{Color: color.Gray{Y: 220}, Width: vg.Points(0.5)})

	// Initialize the table rows for the legend table
	var tableRows []TableRow

	// Initialize the global maximum Y value for the plot
	globalMaxY := -math.MaxFloat64

	for i, s := range cfg.Series {

		// Fetch the series data from PMM Victoria Metrics API
		vmResp, err := fetchSeries(lumoConfig, s.Expr, s.Legend)
		if err != nil {
			zap.S().Errorf("error: query failed for %s: %v", s.Legend, err)
			continue
		}

		if len(vmResp.Data.Result) == 0 {
			zap.S().Warnf("warning: no data returned for %s", s.Legend)
			continue
		}

		// Iterate over the results from the VMResponse
		for resultIdx, result := range vmResp.Data.Result {

			// Parse the series data from the VMResponse
			seriesData, err := parseSeriesData(result.Values)
			if err != nil {
				continue
			}

			// Update the global maximum Y value if the series data max is greater
			if seriesData.Max > globalMaxY {
				globalMaxY = seriesData.Max
			}

			// Initialize the legend text for the series
			legendText := s.Legend

			// Some legend text may contain variables that need to be replaced with the actual values
			// from the metric. This typically happens when a group() PromQL function is used.
			for key, val := range result.Metric {

				// Of course, there is no standard, and sometimes there are spaces, and sometimes not
				re := regexp.MustCompile("\\{\\{\\s*" + regexp.QuoteMeta(key) + "\\s*\\}\\}")
				legendText = re.ReplaceAllString(legendText, val)
			}

			// Get the color for the series from the palette
			seriesColor := Palette[(i+resultIdx)%len(Palette)]

			// Add the visual series (ie: the graph line and fill polygon) to the plot
			if err := addVisualSeries(p, seriesData, seriesColor); err != nil {
				zap.S().Warnf("Error adding visual series: %v", err)
				continue
			}

			// Add the series data to the table rows for the legend table
			tableRows = append(tableRows, TableRow{
				Legend: legendText,
				Color:  seriesColor,
				Min:    seriesData.Min,
				Max:    seriesData.Max,
				Avg:    seriesData.Sum / seriesData.Count,
			})
		}
	}

	// If the global maximum Y value is not the default value, add the maximum Y value line to the plot
	if globalMaxY != -math.MaxFloat64 {

		// Calculate the padding for the maximum Y value line
		padding := math.Max(math.Abs(globalMaxY)*0.10, 1.0)

		// If the maximum Y value is less than 1, set it to 1
		if globalMaxY < 1 {
			p.Y.Max = 1
		} else {
			p.Y.Max = globalMaxY + padding
		}

		// Add a maximum Y value line to the plot
		p.Add(HLine{Y: p.Y.Max, Color: color.Gray{Y: 220}, Width: vg.Points(0.5)})
	}

	// Sort the table rows by average value in descending order
	sort.Slice(tableRows, func(i, j int) bool {
		return tableRows[i].Avg > tableRows[j].Avg
	})

	tableOffset := vg.Points(15*float64(len(tableRows)) + 30)

	// Create a new canvas for the image, which includes the plot area and the legend table
	c := vgimg.New(12*vg.Inch, 4*vg.Inch+tableOffset)

	// Create a new drawing context for the image canvas
	dc := draw.New(c)

	// Crop and draw the plot area into the image canvas
	plotCanvas := draw.Crop(dc, 0, 0, tableOffset, 0)
	p.Draw(plotCanvas)

	// If the graph title is not empty, add a line below the title
	if cfg.Title != "" {

		// Calculate the width of the title text so we know how long to draw the line below the title
		titleWidth := p.Title.TextStyle.Width(cfg.Title)
		extents := p.Title.TextStyle.FontExtents()
		lineY := plotCanvas.Max.Y - extents.Ascent - extents.Descent
		plotCanvas.StrokeLine2(draw.LineStyle{Color: color.Black, Width: vg.Points(1)},
			(plotCanvas.Max.X/2)-titleWidth/2, lineY, (plotCanvas.Max.X/2)+titleWidth/2, lineY)
	}

	// Draw the legend table canvas
	drawLegendTable(draw.Crop(dc, 0, 0, 0, -(4*vg.Inch)), tableRows)

	// Create the output file image
	f, err := os.Create(output) // #nosec
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCreateOutput, err)
	}

	defer func() { _ = f.Close() }()

	if _, err := (vgimg.PngCanvas{Canvas: c}).WriteTo(f); err != nil {
		return fmt.Errorf("%w: %w", ErrSavePlot, err)
	}

	zap.S().Infof("  -> Saved chart to '%s'", output)

	return nil
}
