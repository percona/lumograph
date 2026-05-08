package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"image/color"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/font"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/text"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
	"math"
)

//go:embed resources/fonts/Poppins-Medium.ttf
var poppinsTTF []byte

//go:embed resources/fonts/Poppins-Bold.ttf
var poppinsBoldTTF []byte

var poppinsFont = font.Font{Typeface: "Poppins", Size: vg.Points(10)}
var poppinsBoldFont = font.Font{Typeface: "Poppins", Weight: xfont.WeightBold, Size: vg.Points(10)}

const LegendPadding = 8

type VMResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

type SeriesConfig struct {
	Legend string `json:"legend"`
	Expr   string `json:"expr"`
}

type TableRow struct {
	Legend string
	Color  color.Color
	Min    float64
	Max    float64
	Avg    float64
}

type GraphConfig struct {
	Title  string         `json:"title,omitempty"`
	Series []SeriesConfig `json:"series"`
}

var Palette = []color.Color{
	color.RGBA{R: 0x1E, G: 0x88, B: 0xE5, A: 255}, // Blue
	color.RGBA{R: 0xD8, G: 0x1B, B: 0x60, A: 255}, // Pink
	color.RGBA{R: 0xFF, G: 0xC1, B: 0x07, A: 255}, // Amber
	color.RGBA{R: 0x00, G: 0x4D, B: 0x40, A: 255}, // Teal
	color.RGBA{R: 0x5E, G: 0x35, B: 0xB1, A: 255}, // Deep Purple
	color.RGBA{R: 0xFF, G: 0x8F, B: 0x00, A: 255}, // Orange
}

func generateGraph(endpoint, service, interval string, series []SeriesConfig, title, token, output string, startTime, endTime time.Time, debug bool) error {
	p := plot.New()
	p.Title.Text = title
	p.Title.Padding = 20
	p.X.Tick.Marker = plot.TimeTicks{
		Format: "15:04:05",
		Time:   plot.UnixTimeIn(time.Local),
	}
	// Disabled native legend
	p.Legend.Padding = LegendPadding

	// Add faint grid lines
	grid := plotter.NewGrid()
	grid.Vertical.Color = color.Gray{Y: 220}
	grid.Horizontal.Color = color.Gray{Y: 220}
	p.Add(grid)

	base, _ := url.Parse(endpoint)
	base.Path = "/victoriametrics/prometheus/api/v1/query_range"
	var tableRows []TableRow

	for i, s := range series {

		q := base.Query()
		interpolatedExpr := strings.ReplaceAll(s.Expr, "$service_name", service)
		interpolatedExpr = strings.ReplaceAll(interpolatedExpr, "$interval", interval)
		q.Set("query", interpolatedExpr)
		q.Set("step", "60s")
		q.Set("start", fmt.Sprintf("%d", startTime.Unix()))
		q.Set("end", fmt.Sprintf("%d", endTime.Unix()))
		base.RawQuery = q.Encode()

		req, err := http.NewRequest("POST", base.String(), nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: creating request for %s: %v\n", s.Legend, err)
			continue
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		if debug {
			dump, err := httputil.DumpRequestOut(req, true)
			if err == nil {
				fmt.Fprintf(os.Stderr, "--- DEBUG: HTTP Request (%s) ---\n%s\n---------------------------\n", s.Legend, dump)
			}
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: failed to query VictoriaMetrics for %s: %v\n", s.Legend, err)
			continue
		}

		if debug {
			dump, err := httputil.DumpResponse(resp, true)
			if err == nil {
				fmt.Fprintf(os.Stderr, "--- DEBUG: HTTP Response (%s) ---\n%s\n----------------------------\n", s.Legend, dump)
			}
		}

		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "error: unexpected HTTP status for %s: %d\n", s.Legend, resp.StatusCode)
			resp.Body.Close()
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: reading response for %s: %v\n", s.Legend, err)
			continue
		}

		var vmResp VMResponse
		if err := json.Unmarshal(body, &vmResp); err != nil {
			fmt.Fprintf(os.Stderr, "error: parsing response for %s: %v\n", s.Legend, err)
			continue
		}

		if debug {
			fmt.Fprintf(os.Stderr, "--- DEBUG: RAW METRICS (%s) ---\n%+v\n---------------------------\n", s.Legend, vmResp)
		}

		if vmResp.Status != "success" || len(vmResp.Data.Result) == 0 {
			fmt.Fprintf(os.Stderr, "warning: no data returned for %s\n", s.Legend)
			continue
		}

		minVal := math.MaxFloat64
		maxVal := -math.MaxFloat64
		sumVal := 0.0
		countVal := 0.0

		pts := make(plotter.XYs, 0, len(vmResp.Data.Result[0].Values))
		for _, v := range vmResp.Data.Result[0].Values {
			if len(v) != 2 {
				fmt.Fprintf(os.Stderr, "error: not enough values")
				continue
			}
			t, ok := v[0].(float64)
			if !ok {
				fmt.Fprintf(os.Stderr, "error: could not parse timestamp")
				continue
			}
			valStr, ok := v[1].(string)
			if !ok {
				fmt.Fprintf(os.Stderr, "error: could not parse string value")
				continue
			}
			val, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: could not parse float value")
				continue
			}

			if val < minVal {
				minVal = val
			}
			if val > maxVal {
				maxVal = val
			}
			sumVal += val
			countVal++

			pts = append(pts, plotter.XY{X: t, Y: val})
		}

		if len(pts) < 1 {
			fmt.Fprintf(os.Stderr, "error: no xy points")
			continue
		}

		if debug {
			fmt.Fprintf(os.Stderr, "--- DEBUG: PARSED METRICS (%s) ---\n%+v\n---------------------------\n", s.Legend, pts)
		}

		line, err := plotter.NewLine(pts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: creating line for %s: %v\n", s.Legend, err)
			continue
		}
		seriesColor := Palette[i%len(Palette)]
		line.Color = seriesColor

		p.Add(line)

		avgVal := 0.0
		if countVal > 0 {
			avgVal = sumVal / countVal
		}

		tableRows = append(tableRows, TableRow{
			Legend: s.Legend,
			Color:  seriesColor,
			Min:    minVal,
			Max:    maxVal,
			Avg:    avgVal,
		})
	}

	tableOffset := vg.Points(15*float64(len(tableRows)) + 30)

	c := vgimg.New(12*vg.Inch, 4*vg.Inch+tableOffset)
	dc := draw.New(c)

	plotCanvas := draw.Crop(dc, 0, 0, tableOffset, 0)
	p.Draw(plotCanvas)

	tableCanvas := draw.Crop(dc, 0, 0, 0, -(4 * vg.Inch))

	styleNormal := text.Style{
		Font:    poppinsFont,
		Color:   color.Black,
		Handler: plot.DefaultTextHandler,
	}

	styleBold := text.Style{
		Font:    poppinsBoldFont,
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
			tableCanvas.FillText(styleNormal, vg.Point{X: colMin, Y: y}, fmt.Sprintf("%.4g", row.Min))
			tableCanvas.FillText(styleNormal, vg.Point{X: colMax, Y: y}, fmt.Sprintf("%.4g", row.Max))
			tableCanvas.FillText(styleNormal, vg.Point{X: colAvg, Y: y}, fmt.Sprintf("%.4g", row.Avg))
		}
	}
	f, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	png := vgimg.PngCanvas{Canvas: c}
	if _, err := png.WriteTo(f); err != nil {
		return fmt.Errorf("saving plot to png: %w", err)
	}

	fmt.Printf("Saved chart to %s\n", output)
	return nil
}

func toSnakeCase(s string) string {
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func main() {

	// Flags
	endpoint := flag.String("endpoint", "", "VictoriaMetrics endpoint URL (required)")
	service := flag.String("service", "", "Service name for query substitution (required)")
	interval := flag.String("interval", "5m", "Interval duration string for query substitution (e.g., 5m, 1h)")
	start := flag.Duration("start", time.Hour, "Start time (duration)")
	end := flag.Duration("end", 0*time.Second, "End time (duration from now, 0 = now)")
	token := flag.String("token", "", "Bearer token for VictoriaMetrics auth (required)")
	debug := flag.Bool("debug", false, "Print detailed HTTP request and response information")

	flag.Parse()

	if *endpoint == "" {
		fmt.Fprintln(os.Stderr, "error: -endpoint flag is required")
		os.Exit(1)
	}

	if *service == "" {
		fmt.Fprintln(os.Stderr, "error: -service flag is required")
		os.Exit(1)
	}

	if *token == "" {
		fmt.Fprintln(os.Stderr, "error: -token flag is required")
		os.Exit(1)
	}

	endTime := time.Now().Add(-*end)
	startTime := endTime.Add(-*start)

	var configs []GraphConfig

	// Read configs from static graphs.json file
	data, err := os.ReadFile("graphs/graphs.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to read graphs.json: %v\n", err)
		os.Exit(1)
	}
	if err := json.Unmarshal(data, &configs); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to parse graphs.json: %v\n", err)
		os.Exit(1)
	}

	// Load fonts
	ttf, err := opentype.Parse(poppinsTTF)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing embedded font: %v\n", err)
		os.Exit(1)
	}

	ttfBold, err := opentype.Parse(poppinsBoldTTF)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing embedded bold font: %v\n", err)
		os.Exit(1)
	}

	font.DefaultCache.Add([]font.Face{
		{
			Font: poppinsFont,
			Face: ttf,
		},
		{
			Font: poppinsBoldFont,
			Face: ttfBold,
		},
	})

	plot.DefaultFont = poppinsFont

	for _, cfg := range configs {
		if len(cfg.Series) == 0 {
			fmt.Fprintf(os.Stderr, "skipping graph '%s': no series defined\n", cfg.Title)
			continue
		}

		nameBase := cfg.Title
		if nameBase == "" {
			nameBase = "untitled_graph"
		}
		outputFile := toSnakeCase(nameBase) + ".png"

		fmt.Printf("Generating graph for title: %s -> %s\n", cfg.Title, outputFile)
		if err := generateGraph(*endpoint, *service, *interval, cfg.Series, cfg.Title, *token, outputFile, startTime, endTime, *debug); err != nil {
			fmt.Fprintf(os.Stderr, "error generating graph: %v\n", err)
		}
	}
}
