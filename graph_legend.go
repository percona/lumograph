package main

import (
	"image/color"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/text"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

// drawLegendTable is responsible for drawing the legend table at the bottom of the image
func drawLegendTable(tableCanvas draw.Canvas, rows []TableRow) {

	// Define the text styles for the legend table
	styleNormal := text.Style{Font: mediumFont, Color: color.Black, Handler: plot.DefaultTextHandler}
	styleBold := text.Style{Font: boldFont, Color: color.Black, Handler: plot.DefaultTextHandler}

	// Define the column widths
	colColor := vg.Points(10)
	colName := vg.Points(30)
	colMin := tableCanvas.Max.X * 0.55
	colMax := tableCanvas.Max.X * 0.70
	colAvg := tableCanvas.Max.X * 0.85

	// Draw the header row
	y := tableCanvas.Max.Y - vg.Points(15)
	tableCanvas.FillText(styleBold, vg.Point{X: colName, Y: y}, "Name")
	tableCanvas.FillText(styleBold, vg.Point{X: colMin, Y: y}, "Min")
	tableCanvas.FillText(styleBold, vg.Point{X: colMax, Y: y}, "Max")
	tableCanvas.FillText(styleBold, vg.Point{X: colAvg, Y: y}, "Average")

	sepStyle := draw.LineStyle{Color: color.Gray{Y: 220}, Width: vg.Points(0.5)}

	// Draw the data rows
	for _, row := range rows {

		// Draw the separator line
		lineY := y - vg.Points(4)
		tableCanvas.StrokeLine2(sepStyle, colColor, lineY, tableCanvas.Max.X, lineY)

		y -= vg.Points(15)

		// Draw the color box
		boxSize := vg.Points(8)
		tableCanvas.FillPolygon(row.Color, []vg.Point{
			{X: colColor, Y: y}, {X: colColor + boxSize, Y: y},
			{X: colColor + boxSize, Y: y + boxSize}, {X: colColor, Y: y + boxSize},
		})

		// Draw the series name text
		tableCanvas.FillText(styleNormal, vg.Point{X: colName, Y: y}, row.Legend)

		// Draw the min, max, and average values text
		if row.Min <= row.Max {
			tableCanvas.FillText(styleNormal, vg.Point{X: colMin, Y: y}, formatValue(row.Min))
			tableCanvas.FillText(styleNormal, vg.Point{X: colMax, Y: y}, formatValue(row.Max))
			tableCanvas.FillText(styleNormal, vg.Point{X: colAvg, Y: y}, formatValue(row.Avg))
		}
	}
}
