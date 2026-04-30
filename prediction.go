package main

import (
	"database/sql"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"sort"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	_ "modernc.org/sqlite"
)

// ── Forecast-specific types ────────────────────────────────────────────────

// ForecastPoint holds a single x/y prediction with a confidence band.
type ForecastPoint struct {
	X, YMin, YAvg, YMax float64
}

// linearRegression returns slope and intercept for the given (x, y) pairs.
func linearRegression(xs, ys []float64) (slope, intercept float64) {
	n := float64(len(xs))
	if n == 0 {
		return 0, 0
	}
	var sumX, sumY, sumXX, sumXY float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXX += xs[i] * xs[i]
		sumXY += xs[i] * ys[i]
	}
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 0, sumY / n
	}
	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n
	return
}

// residualStdDev returns the standard deviation of residuals around the regression line.
func residualStdDev(xs, ys []float64, slope, intercept float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var sumSq float64
	for i := range xs {
		diff := ys[i] - (slope*xs[i] + intercept)
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(len(xs)-1))
}

// buildForecast creates 12 monthly forecast points for the year following the
// training data. sigma multipliers: min = −1σ, avg = 0, max = +1σ.
func buildForecast(data []MonthlyData, getter func(MonthlyData) float64, forecastYear int) []ForecastPoint {
	var xs, ys []float64
	for _, d := range data {
		xs = append(xs, monthKeyToFloat(d.MonthKey))
		ys = append(ys, getter(d))
	}
	slope, intercept := linearRegression(xs, ys)
	sigma := residualStdDev(xs, ys, slope, intercept)

	var pts []ForecastPoint
	for month := 1; month <= 12; month++ {
		x := float64(forecastYear) + float64(month-1)/12.0
		yAvg := slope*x + intercept
		if yAvg < 0 {
			yAvg = 0
		}
		yMin := math.Max(0, yAvg-sigma)
		yMax := yAvg + sigma
		pts = append(pts, ForecastPoint{X: x, YMin: yMin, YAvg: yAvg, YMax: yMax})
	}
	return pts
}

// ── Tick helpers (reuse from original file) ───────────────────────────────

func fcMonthKeyToFloat(mk int) float64       { return monthKeyToFloat(mk) }
func fcMonthKeyToLabel(mk int) string         { return monthKeyToMonthLabel(mk) }

func buildForecastMonthTicks(baseYear, forecastYear int) []plot.Tick {
	var ticks []plot.Tick
	months := []string{"", "Jan", "Feb", "Mär", "Apr", "Mai", "Jun",
		"Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}
	// actual year months
	for m := 1; m <= 12; m++ {
		mk := baseYear*100 + m
		ticks = append(ticks, plot.Tick{
			Value: monthKeyToFloat(mk),
			Label: months[m],
		})
	}
	// forecast year months
	for m := 1; m <= 12; m++ {
		mk := forecastYear*100 + m
		ticks = append(ticks, plot.Tick{
			Value: monthKeyToFloat(mk),
			Label: months[m],
		})
	}
	return ticks
}

func buildAlltimeForecastTicks(allData []MonthlyData, forecastYear int) []plot.Tick {
	ticks := buildYearTicks(allData)
	ticks = append(ticks, plot.Tick{
		Value: float64(forecastYear),
		Label: fmt.Sprintf("%d*", forecastYear),
	})
	return ticks
}

// ── Forecast colors ───────────────────────────────────────────────────────

var (
	colorForecastMin  = color.RGBA{R: 52, G: 168, B: 83, A: 255}   // green
	colorForecastAvg  = color.RGBA{R: 66, G: 133, B: 244, A: 255}  // blue
	colorForecastMax  = color.RGBA{R: 234, G: 67, B: 53, A: 255}   // red
	colorActualCompare = color.RGBA{R: 0, G: 0, B: 0, A: 200}      // near-black
	colorBand         = color.RGBA{R: 66, G: 133, B: 244, A: 40}   // transparent fill
)

// ── Plot helpers ──────────────────────────────────────────────────────────

// addForecastBand draws a filled polygon between the min and max forecast lines.
func addForecastBand(p *plot.Plot, pts []ForecastPoint) {
	poly := make(plotter.XYs, 2*len(pts))
	for i, fp := range pts {
		poly[i] = plotter.XY{X: fp.X, Y: fp.YMax}
	}
	for i, fp := range pts {
		poly[len(pts)+i] = plotter.XY{X: pts[len(pts)-1-i].X, Y: fp.YMin}
	}
	// reverse the bottom half so the polygon closes correctly
	for i, j := len(pts), 2*len(pts)-1; i < j; i, j = i+1, j-1 {
		poly[i], poly[j] = poly[j], poly[i]
	}
	polygon, err := plotter.NewPolygon(poly)
	if err != nil {
		return
	}
	polygon.Color = colorBand
	polygon.LineStyle.Width = 0
	p.Add(polygon)
}

func forecastToXYs(pts []ForecastPoint, pick func(ForecastPoint) float64) plotter.XYs {
	xys := make(plotter.XYs, len(pts))
	for i, fp := range pts {
		xys[i] = plotter.XY{X: fp.X, Y: pick(fp)}
	}
	return xys
}

func newForecastLine(pts []ForecastPoint, pick func(ForecastPoint) float64, c color.RGBA, dash bool) *plotter.Line {
	line, err := plotter.NewLine(forecastToXYs(pts, pick))
	if err != nil {
		log.Fatal(err)
	}
	line.Color = c
	line.Width = vg.Points(1.8)
	if dash {
		line.Dashes = []vg.Length{vg.Points(5), vg.Points(3)}
	}
	return line
}

// ── Single-series forecast plot ───────────────────────────────────────────

// saveForecastSinglePlot draws one series' actual data for baseYear plus
// forecast lines for forecastYear. If actualNextData is non-nil, a "real"
// comparison line is also drawn.
func saveForecastSinglePlot(
	baseData []MonthlyData,
	actualNextData []MonthlyData, // nil when not yet available
	s Series,
	forecastPts []ForecastPoint,
	ticks []plot.Tick,
	title, filePath string,
) {
	p := plot.New()
	p.Title.Text = title
	p.Title.TextStyle.Font.Size = vg.Points(15)
	p.X.Label.Text = "Zeit"
	p.Y.Label.Text = "Verbrauch (kWh)"
	p.Add(plotter.NewGrid())
	p.X.Tick.Marker = plot.ConstantTicks(ticks)

	// Actual base-year line
	basePts := make(plotter.XYs, len(baseData))
	for i, d := range baseData {
		basePts[i] = plotter.XY{X: monthKeyToFloat(d.MonthKey), Y: s.getter(d)}
	}
	actualLine, err := plotter.NewLine(basePts)
	if err != nil {
		log.Fatal(err)
	}
	actualLine.Color = s.color
	actualLine.Width = vg.Points(2)
	p.Add(actualLine)
	p.Legend.Add("Ist", actualLine)

	// Forecast band + lines
	addForecastBand(p, forecastPts)

	minLine := newForecastLine(forecastPts, func(f ForecastPoint) float64 { return f.YMin }, colorForecastMin, true)
	avgLine := newForecastLine(forecastPts, func(f ForecastPoint) float64 { return f.YAvg }, colorForecastAvg, false)
	maxLine := newForecastLine(forecastPts, func(f ForecastPoint) float64 { return f.YMax }, colorForecastMax, true)
	p.Add(minLine, avgLine, maxLine)
	p.Legend.Add("Prognose Min", minLine)
	p.Legend.Add("Prognose Avg", avgLine)
	p.Legend.Add("Prognose Max", maxLine)

	// Comparison line (actual data for forecast year)
	if len(actualNextData) > 0 {
		nextPts := make(plotter.XYs, len(actualNextData))
		for i, d := range actualNextData {
			nextPts[i] = plotter.XY{X: monthKeyToFloat(d.MonthKey), Y: s.getter(d)}
		}
		nextLine, err := plotter.NewLine(nextPts)
		if err != nil {
			log.Fatal(err)
		}
		nextLine.Color = colorActualCompare
		nextLine.Width = vg.Points(2)
		nextLine.Dashes = []vg.Length{vg.Points(8), vg.Points(4)}
		p.Add(nextLine)
		p.Legend.Add("Tatsächlich (Folgejahr)", nextLine)
	}

	p.Legend.Top = true
	p.Legend.Left = false
	p.Legend.TextStyle.Font.Size = vg.Points(9)

	// Y axis range
	maxVal := 0.0
	for _, pt := range forecastPts {
		if pt.YMax > maxVal {
			maxVal = pt.YMax
		}
	}
	for _, d := range baseData {
		if v := s.getter(d); v > maxVal {
			maxVal = v
		}
	}
	if len(actualNextData) > 0 {
		for _, d := range actualNextData {
			if v := s.getter(d); v > maxVal {
				maxVal = v
			}
		}
	}
	if maxVal == 0 {
		maxVal = 100
	}
	p.Y.Min = 0
	p.Y.Max = math.Ceil(maxVal/1000) * 1000
	if p.Y.Max == 0 {
		p.Y.Max = maxVal * 1.1
	}

	if err := p.Save(20*vg.Centimeter, 12*vg.Centimeter, filePath); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Gespeichert:", filePath)
}

// ── All-series overlay forecast plot ─────────────────────────────────────

func saveForecastAllSeriesPlot(
	baseData []MonthlyData,
	actualNextData []MonthlyData,
	seriesList []Series,
	forecastMap map[string][]ForecastPoint,
	ticks []plot.Tick,
	title, filePath string,
) {
	p := plot.New()
	p.Title.Text = title
	p.Title.TextStyle.Font.Size = vg.Points(15)
	p.X.Label.Text = "Zeit"
	p.Y.Label.Text = "Verbrauch (kWh)"
	p.Add(plotter.NewGrid())
	p.X.Tick.Marker = plot.ConstantTicks(ticks)

	maxVal := 0.0

	for _, s := range seriesList {
		// Actual
		basePts := make(plotter.XYs, len(baseData))
		for i, d := range baseData {
			basePts[i] = plotter.XY{X: monthKeyToFloat(d.MonthKey), Y: s.getter(d)}
			if s.getter(d) > maxVal {
				maxVal = s.getter(d)
			}
		}
		actualLine, err := plotter.NewLine(basePts)
		if err != nil {
			log.Fatal(err)
		}
		actualLine.Color = s.color
		actualLine.Width = vg.Points(1.5)
		p.Add(actualLine)
		p.Legend.Add(s.name+" Ist", actualLine)

		// Forecast avg only (keep chart readable)
		fps := forecastMap[s.id]
		avgLine := newForecastLine(fps, func(f ForecastPoint) float64 { return f.YAvg }, s.color, true)
		p.Add(avgLine)
		p.Legend.Add(s.name+" Prog.", avgLine)
		for _, fp := range fps {
			if fp.YMax > maxVal {
				maxVal = fp.YMax
			}
		}
	}

	// Actual next-year data per series
	if len(actualNextData) > 0 {
		for _, s := range seriesList {
			nextPts := make(plotter.XYs, len(actualNextData))
			for i, d := range actualNextData {
				nextPts[i] = plotter.XY{X: monthKeyToFloat(d.MonthKey), Y: s.getter(d)}
				if s.getter(d) > maxVal {
					maxVal = s.getter(d)
				}
			}
			nextLine, err := plotter.NewLine(nextPts)
			if err != nil {
				log.Fatal(err)
			}
			c := s.color
			c.A = 120
			nextLine.Color = c
			nextLine.Width = vg.Points(1.2)
			nextLine.Dashes = []vg.Length{vg.Points(8), vg.Points(4)}
			p.Add(nextLine)
		}
	}

	p.Legend.Top = true
	p.Legend.Left = false
	p.Legend.TextStyle.Font.Size = vg.Points(7)

	p.Y.Min = 0
	if maxVal == 0 {
		maxVal = 100
	}
	p.Y.Max = math.Ceil(maxVal/1000) * 1000

	if err := p.Save(20*vg.Centimeter, 12*vg.Centimeter, filePath); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Gespeichert:", filePath)
}

// ── Stacked forecast plot ─────────────────────────────────────────────────

func saveForecastCombinedPlot(
	baseData []MonthlyData,
	actualNextData []MonthlyData,
	seriesList []Series,
	forecastMap map[string][]ForecastPoint,
	ticks []plot.Tick,
	title, filePath string,
) {
	p := plot.New()
	p.Title.Text = title
	p.Title.TextStyle.Font.Size = vg.Points(15)
	p.X.Label.Text = "Zeit"
	p.Y.Label.Text = "Gesamtverbrauch (kWh)"
	p.Add(plotter.NewGrid())
	p.X.Tick.Marker = plot.ConstantTicks(ticks)

	nFc := 12 // months in forecast year

	// ── Stacked actual (base year) ────────────────────────────────────────
	cumBase := make([]float64, len(baseData))
	prevBase := make(plotter.XYs, len(baseData))
	for i, d := range baseData {
		prevBase[i] = plotter.XY{X: monthKeyToFloat(d.MonthKey), Y: 0}
	}

	for _, s := range seriesList {
		layer := make(plotter.XYs, len(baseData))
		for i, d := range baseData {
			cumBase[i] += s.getter(d)
			layer[i] = plotter.XY{X: monthKeyToFloat(d.MonthKey), Y: cumBase[i]}
		}
		poly := make(plotter.XYs, 2*len(baseData))
		copy(poly, layer)
		for i := range baseData {
			poly[len(baseData)+i] = prevBase[len(baseData)-1-i]
		}
		polygon, err := plotter.NewPolygon(poly)
		if err != nil {
			log.Fatal(err)
		}
		c := s.color
		c.A = 160
		polygon.Color = c
		polygon.LineStyle.Width = 0
		p.Add(polygon)

		borderLine, err := plotter.NewLine(layer)
		if err != nil {
			log.Fatal(err)
		}
		borderLine.Color = s.color
		borderLine.Width = vg.Points(0.8)
		p.Add(borderLine)
		p.Legend.Add(s.name, borderLine)

		copy(prevBase, layer)
	}

	// ── Stacked forecast avg ──────────────────────────────────────────────
	cumFcAvg := make([]float64, nFc)
	prevFcAvg := make(plotter.XYs, nFc)
	for i, fp := range forecastMap[seriesList[0].id] {
		prevFcAvg[i] = plotter.XY{X: fp.X, Y: 0}
	}

	for _, s := range seriesList {
		fps := forecastMap[s.id]
		layer := make(plotter.XYs, nFc)
		for i, fp := range fps {
			cumFcAvg[i] += fp.YAvg
			layer[i] = plotter.XY{X: fp.X, Y: cumFcAvg[i]}
		}
		poly := make(plotter.XYs, 2*nFc)
		copy(poly, layer)
		for i := range fps {
			poly[nFc+i] = prevFcAvg[nFc-1-i]
		}
		polygon, err := plotter.NewPolygon(poly)
		if err != nil {
			log.Fatal(err)
		}
		c := s.color
		c.A = 80
		polygon.Color = c
		polygon.LineStyle.Width = 0
		p.Add(polygon)

		borderLine, err := plotter.NewLine(layer)
		if err != nil {
			log.Fatal(err)
		}
		borderLine.Color = s.color
		borderLine.Width = vg.Points(0.8)
		borderLine.Dashes = []vg.Length{vg.Points(5), vg.Points(3)}
		p.Add(borderLine)

		copy(prevFcAvg, layer)
	}

	// Total actual + total forecast avg dashed
	totalBase := make(plotter.XYs, len(baseData))
	for i := range baseData {
		totalBase[i] = prevBase[i]
	}
	totalLine, err := plotter.NewLine(totalBase)
	if err != nil {
		log.Fatal(err)
	}
	totalLine.Color = color.RGBA{0, 0, 0, 255}
	totalLine.Width = vg.Points(2)
	p.Add(totalLine)
	p.Legend.Add("Gesamt Ist", totalLine)

	totalFc := make(plotter.XYs, nFc)
	for i := range prevFcAvg {
		totalFc[i] = prevFcAvg[i]
	}
	totalFcLine, err := plotter.NewLine(totalFc)
	if err != nil {
		log.Fatal(err)
	}
	totalFcLine.Color = colorForecastAvg
	totalFcLine.Width = vg.Points(2)
	totalFcLine.Dashes = []vg.Length{vg.Points(7), vg.Points(4)}
	p.Add(totalFcLine)
	p.Legend.Add("Gesamt Prognose", totalFcLine)

	// Optional actual next-year stacked total
	if len(actualNextData) > 0 {
		cumNext := make([]float64, len(actualNextData))
		for _, s := range seriesList {
			for i, d := range actualNextData {
				cumNext[i] += s.getter(d)
			}
		}
		nextTotalPts := make(plotter.XYs, len(actualNextData))
		for i, d := range actualNextData {
			nextTotalPts[i] = plotter.XY{X: monthKeyToFloat(d.MonthKey), Y: cumNext[i]}
		}
		nextTotalLine, err := plotter.NewLine(nextTotalPts)
		if err != nil {
			log.Fatal(err)
		}
		nextTotalLine.Color = colorActualCompare
		nextTotalLine.Width = vg.Points(2)
		nextTotalLine.Dashes = []vg.Length{vg.Points(9), vg.Points(5)}
		p.Add(nextTotalLine)
		p.Legend.Add("Gesamt Tatsächlich", nextTotalLine)
	}

	p.Legend.Top = true
	p.Legend.Left = false
	p.Legend.TextStyle.Font.Size = vg.Points(8)

	// Y max
	maxTotal := 0.0
	for _, pt := range totalBase {
		if pt.Y > maxTotal {
			maxTotal = pt.Y
		}
	}
	for _, pt := range totalFc {
		if pt.Y > maxTotal {
			maxTotal = pt.Y
		}
	}
	if len(actualNextData) > 0 {
		cumChk := 0.0
		for _, s := range seriesList {
			for _, d := range actualNextData {
				cumChk += s.getter(d)
			}
		}
		if cumChk > maxTotal {
			maxTotal = cumChk
		}
	}
	if maxTotal == 0 {
		maxTotal = 100
	}
	p.Y.Min = 0
	p.Y.Max = math.Ceil(maxTotal/1000) * 1000

	if err := p.Save(20*vg.Centimeter, 12*vg.Centimeter, filePath); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Gespeichert:", filePath)
}

// ── Main entry point ──────────────────────────────────────────────────────

func diagramsOfForecasts() {
	db, err := sql.Open("sqlite", "assets/data/datenbank.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT month_key, i_internat, i_uz_aula, l_internat, l_gemeinschaft,
		       p_uzp2, p_uzp3, p_uzp4, s_uzp4, s_bhkw
		FROM electricity_usage
		ORDER BY month_key ASC
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var allData []MonthlyData
	for rows.Next() {
		var d MonthlyData
		if err := rows.Scan(&d.MonthKey, &d.IInternat, &d.IUzAula, &d.LInternat,
			&d.LGemeinschaft, &d.PUzp2, &d.PUzp3, &d.PUzp4, &d.SUzp4, &d.SBhkw); err != nil {
			log.Fatal(err)
		}
		allData = append(allData, d)
	}
	if len(allData) == 0 {
		log.Fatal("Keine Daten in der Datenbank gefunden")
	}

	seriesList := []Series{
		{"i_internat", "Internat", func(d MonthlyData) float64 { return d.IInternat }, color.RGBA{R: 31, G: 119, B: 180, A: 255}},
		{"i_uz_aula", "UZ Aula", func(d MonthlyData) float64 { return d.IUzAula }, color.RGBA{R: 255, G: 127, B: 14, A: 255}},
		{"l_internat", "L13 Internat", func(d MonthlyData) float64 { return d.LInternat }, color.RGBA{R: 44, G: 160, B: 44, A: 255}},
		{"l_gemeinschaft", "L13 Gemeinschaft", func(d MonthlyData) float64 { return d.LGemeinschaft }, color.RGBA{R: 214, G: 39, B: 40, A: 255}},
		{"p_uzp2", "UZ P002", func(d MonthlyData) float64 { return d.PUzp2 }, color.RGBA{R: 148, G: 103, B: 189, A: 255}},
		{"p_uzp3", "UZ P003", func(d MonthlyData) float64 { return d.PUzp3 }, color.RGBA{R: 140, G: 86, B: 75, A: 255}},
		{"p_uzp4", "UZ P004", func(d MonthlyData) float64 { return d.PUzp4 }, color.RGBA{R: 227, G: 119, B: 194, A: 255}},
		{"s_uzp4", "Sporthalle UZ P4", func(d MonthlyData) float64 { return d.SUzp4 }, color.RGBA{R: 127, G: 127, B: 127, A: 255}},
		{"s_bhkw", "Strom BHKW", func(d MonthlyData) float64 { return d.SBhkw }, color.RGBA{R: 188, G: 189, B: 34, A: 255}},
	}

	// Group data by year
	yearDataMap := map[int][]MonthlyData{}
	for _, d := range allData {
		y := d.MonthKey / 100
		yearDataMap[y] = append(yearDataMap[y], d)
	}
	var years []int
	for y := range yearDataMap {
		years = append(years, y)
	}
	sort.Ints(years)

	basePath := "assets/diagrams/forecasts"

	// ── Per-year forecast plots ───────────────────────────────────────────
	for _, year := range years {
		baseData := yearDataMap[year]
		forecastYear := year + 1

		// Build forecast map for all series
		forecastMap := map[string][]ForecastPoint{}
		for _, s := range seriesList {
			forecastMap[s.id] = buildForecast(baseData, s.getter, forecastYear)
		}

		// Actual data for forecast year (may be nil)
		actualNext := yearDataMap[forecastYear] // nil if not present

		dirPath := fmt.Sprintf("%s/%d", basePath, year)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			log.Fatal(err)
		}

		ticks := buildForecastMonthTicks(year, forecastYear)

		// Individual series
		for _, s := range seriesList {
			title := fmt.Sprintf("%s – %d + Prognose %d", s.name, year, forecastYear)
			filePath := fmt.Sprintf("%s/%s.png", dirPath, s.id)
			saveForecastSinglePlot(baseData, actualNext, s, forecastMap[s.id], ticks, title, filePath)
		}

		// All series overlay
		saveForecastAllSeriesPlot(
			baseData, actualNext, seriesList, forecastMap, ticks,
			fmt.Sprintf("Alle Zähler – %d + Prognose %d", year, forecastYear),
			fmt.Sprintf("%s/all.png", dirPath),
		)

		// Stacked combined
		saveForecastCombinedPlot(
			baseData, actualNext, seriesList, forecastMap, ticks,
			fmt.Sprintf("Gesamtverbrauch gestapelt – %d + Prognose %d", year, forecastYear),
			fmt.Sprintf("%s/combined.png", dirPath),
		)
	}

	// ── Alltime forecast plots ────────────────────────────────────────────
	lastYear := years[len(years)-1]
	forecastYear := lastYear + 1

	forecastMap := map[string][]ForecastPoint{}
	for _, s := range seriesList {
		forecastMap[s.id] = buildForecast(allData, s.getter, forecastYear)
	}

	alltimePath := fmt.Sprintf("%s/alltime", basePath)
	if err := os.MkdirAll(alltimePath, 0755); err != nil {
		log.Fatal(err)
	}

	alltimeTicks := buildAlltimeForecastTicks(allData, forecastYear)

	// Build a synthetic 12-point "alltime base" using year totals for the
	// alltime chart, so the x-axis aligns with the year ticks.
	// We pass allData directly; monthKeyToFloat handles the float mapping.

	for _, s := range seriesList {
		title := fmt.Sprintf("%s – Gesamtzeitraum + Prognose %d", s.name, forecastYear)
		filePath := fmt.Sprintf("%s/%s.png", alltimePath, s.id)
		saveForecastSinglePlot(allData, nil, s, forecastMap[s.id], alltimeTicks, title, filePath)
	}

	saveForecastAllSeriesPlot(
		allData, nil, seriesList, forecastMap, alltimeTicks,
		fmt.Sprintf("Alle Zähler – Gesamtzeitraum + Prognose %d", forecastYear),
		fmt.Sprintf("%s/all.png", alltimePath),
	)

	saveForecastCombinedPlot(
		allData, nil, seriesList, forecastMap, alltimeTicks,
		fmt.Sprintf("Gesamtverbrauch gestapelt – Gesamtzeitraum + Prognose %d", forecastYear),
		fmt.Sprintf("%s/combined.png", alltimePath),
	)

	fmt.Println("\nAlle Prognose-Diagramme erfolgreich generiert.")
}