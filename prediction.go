package main

import (
	"database/sql"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"sort"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	_ "modernc.org/sqlite"
)

// ── Lineare Regression (OLS) ──────────────────────────────────────────────────

type RegressionModel struct {
	slope     float64
	intercept float64
}

func linearRegression(xs, ys []float64) RegressionModel {
	n := float64(len(xs))
	if n == 0 {
		return RegressionModel{}
	}
	var sumX, sumY, sumXY, sumXX float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumXX += xs[i] * xs[i]
	}
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return RegressionModel{intercept: sumY / n}
	}
	slope := (n*sumXY - sumX*sumY) / denom
	intercept := (sumY - slope*sumX) / n
	return RegressionModel{slope, intercept}
}

func (m RegressionModel) predict(x float64) float64 {
	v := m.slope*x + m.intercept
	if v < 0 {
		return 0
	}
	return v
}

// residuals → stddev für Konfidenzband
func residualStddev(xs, ys []float64, m RegressionModel) float64 {
	if len(xs) < 2 {
		return 0
	}
	var sumSq float64
	for i := range xs {
		diff := ys[i] - m.predict(xs[i])
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(len(xs)-1))
}

// ── Hilfsfunktionen ───────────────────────────────────────────────────────────

// xValues und yValues aus MonthlyData für eine Series extrahieren
func seriesXY(data []MonthlyData, s Series) ([]float64, []float64) {
	xs := make([]float64, len(data))
	ys := make([]float64, len(data))
	for i, d := range data {
		xs[i] = monthKeyToFloat(d.MonthKey)
		ys[i] = s.getter(d)
	}
	return xs, ys
}

// Vorhersage-XYs über einen x-Bereich erzeugen
func predictionLine(model RegressionModel, xMin, xMax float64, steps int) plotter.XYs {
	pts := make(plotter.XYs, steps)
	for i := range pts {
		x := xMin + (xMax-xMin)*float64(i)/float64(steps-1)
		pts[i] = plotter.XY{X: x, Y: model.predict(x)}
	}
	return pts
}

// Konfidenz-Polygon (min-Band unten, max-Band oben)
func confidenceBand(model RegressionModel, stddev, xMin, xMax float64, steps int) plotter.XYs {
	upper := make(plotter.XYs, steps)
	lower := make(plotter.XYs, steps)
	for i := range upper {
		x := xMin + (xMax-xMin)*float64(i)/float64(steps-1)
		y := model.predict(x)
		upper[i] = plotter.XY{X: x, Y: math.Max(0, y+stddev)}
		lower[i] = plotter.XY{X: x, Y: math.Max(0, y-stddev)}
	}
	// Polygon: oben vorwärts + unten rückwärts
	poly := make(plotter.XYs, 2*steps)
	copy(poly[:steps], upper)
	for i := 0; i < steps; i++ {
		poly[steps+i] = lower[steps-1-i]
	}
	return poly
}

// aktuelles Jahr
func currentYear() int {
	return time.Now().Year()
}

// ── Einzelner Prognose-Plot ───────────────────────────────────────────────────
// trainData  = Daten auf denen die Regression basiert
// compareData = nil  → reines Forecast-Jahr
//
//	≠ nil → bereits vorhandene echte Messwerte im Forecast-Jahr (laufendes/vergangenes Vergleichsjahr)
func savePredictionPlot(
	trainData []MonthlyData,
	compareData []MonthlyData, // nil wenn kein Vergleich
	s Series,
	ticks []plot.Tick,
	forecastXMin, forecastXMax float64,
	title, filePath string,
) {
	p := plot.New()
	p.Title.Text = title
	p.Title.TextStyle.Font.Size = vg.Points(14)
	p.X.Label.Text = "Zeit"
	p.Y.Label.Text = "Verbrauch (kWh)"
	p.Add(plotter.NewGrid())
	p.X.Tick.Marker = plot.ConstantTicks(ticks)

	xs, ys := seriesXY(trainData, s)
	model := linearRegression(xs, ys)
	stddev := residualStddev(xs, ys, model)

	const steps = 120

	// ── Konfidenzband ──
	bandPoly := confidenceBand(model, stddev, forecastXMin, forecastXMax, steps)
	polygon, err := plotter.NewPolygon(bandPoly)
	if err != nil {
		log.Fatal(err)
	}
	bandColor := s.color
	bandColor.A = 50
	polygon.Color = bandColor
	polygon.LineStyle.Width = 0
	p.Add(polygon)

	// ── Prognose-Linie ──
	predPts := predictionLine(model, forecastXMin, forecastXMax, steps)
	predLine, err := plotter.NewLine(predPts)
	if err != nil {
		log.Fatal(err)
	}
	predLine.Color = s.color
	predLine.Width = vg.Points(2)
	predLine.Dashes = []vg.Length{vg.Points(8), vg.Points(4)}
	p.Add(predLine)
	p.Legend.Add("Prognose", predLine)

	// ── Trainingsdaten als Punkte + Linie ──
	trainPts := make(plotter.XYs, len(trainData))
	for i, d := range trainData {
		trainPts[i] = plotter.XY{X: monthKeyToFloat(d.MonthKey), Y: s.getter(d)}
	}
	trainLine, err := plotter.NewLine(trainPts)
	if err != nil {
		log.Fatal(err)
	}
	trainLine.Color = color.RGBA{R: 120, G: 120, B: 120, A: 200}
	trainLine.Width = vg.Points(1.2)
	p.Add(trainLine)
	p.Legend.Add("Historisch", trainLine)

	// ── Vergleichsdaten (echte Werte im Prognosezeitraum) ──
	if len(compareData) > 0 {
		cmpPts := make(plotter.XYs, len(compareData))
		for i, d := range compareData {
			cmpPts[i] = plotter.XY{X: monthKeyToFloat(d.MonthKey), Y: s.getter(d)}
		}
		cmpLine, err := plotter.NewLine(cmpPts)
		if err != nil {
			log.Fatal(err)
		}
		cmpLine.Color = color.RGBA{R: 220, G: 50, B: 50, A: 255}
		cmpLine.Width = vg.Points(2)
		p.Add(cmpLine)
		p.Legend.Add("Ist-Werte", cmpLine)

		// Scatter-Punkte für Ist-Werte
		scatter, err := plotter.NewScatter(cmpPts)
		if err != nil {
			log.Fatal(err)
		}
		scatter.GlyphStyle.Color = color.RGBA{R: 220, G: 50, B: 50, A: 255}
		scatter.GlyphStyle.Radius = vg.Points(3)
		p.Add(scatter)
	}

	p.Legend.Top = true
	p.Legend.Left = true
	p.Legend.TextStyle.Font.Size = vg.Points(9)

	// Y-Achse sinnvoll skalieren
	allYs := ys
	for _, d := range compareData {
		allYs = append(allYs, s.getter(d))
	}
	maxY := 0.0
	for _, v := range allYs {
		if v > maxY {
			maxY = v
		}
	}
	// Prognose-Max mit einbeziehen
	for _, pt := range predPts {
		if pt.Y > maxY {
			maxY = pt.Y
		}
	}
	if maxY == 0 {
		maxY = 100
	}
	p.Y.Min = 0
	p.Y.Max = math.Ceil(maxY/500) * 500 * 1.1

	if err := p.Save(20*vg.Centimeter, 12*vg.Centimeter, filePath); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Gespeichert:", filePath)
}

// ── Kombinierter gestapelter Prognose-Plot (alle Series) ─────────────────────
func saveCombinedPredictionPlot(
	trainData []MonthlyData,
	compareData []MonthlyData,
	seriesList []Series,
	ticks []plot.Tick,
	forecastXMin, forecastXMax float64,
	title, filePath string,
) {
	p := plot.New()
	p.Title.Text = title
	p.Title.TextStyle.Font.Size = vg.Points(14)
	p.X.Label.Text = "Zeit"
	p.Y.Label.Text = "Gesamtverbrauch (kWh)"
	p.Add(plotter.NewGrid())
	p.X.Tick.Marker = plot.ConstantTicks(ticks)

	const steps = 120

	// Für gestapelte Prognose: kumulierte Oberkante
	cumulPred := make([]float64, steps)
	cumulBandHi := make([]float64, steps)
	cumulBandLo := make([]float64, steps)
	predXs := make([]float64, steps)
	for i := range predXs {
		predXs[i] = forecastXMin + (forecastXMax-forecastXMin)*float64(i)/float64(steps-1)
	}

	prevLayer := make(plotter.XYs, steps)

	for _, s := range seriesList {
		xs, ys := seriesXY(trainData, s)
		model := linearRegression(xs, ys)
		stddev := residualStddev(xs, ys, model)

		// Kumulierte Prognose-Schicht aufbauen
		layer := make(plotter.XYs, steps)
		for i, x := range predXs {
			cumulPred[i] += model.predict(x)
			cumulBandHi[i] += math.Max(0, model.predict(x)+stddev)
			cumulBandLo[i] += math.Max(0, model.predict(x)-stddev)
			layer[i] = plotter.XY{X: x, Y: cumulPred[i]}
		}

		// Polygon zwischen prevLayer und layer
		poly := make(plotter.XYs, 2*steps)
		copy(poly[:steps], layer)
		for i := 0; i < steps; i++ {
			poly[steps+i] = prevLayer[steps-1-i]
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

		line, err := plotter.NewLine(layer)
		if err != nil {
			log.Fatal(err)
		}
		line.Color = s.color
		line.Width = vg.Points(1)
		line.Dashes = []vg.Length{vg.Points(6), vg.Points(3)}
		p.Add(line)
		p.Legend.Add(s.name+" (Prog.)", line)

		copy(prevLayer, layer)
	}

	// Gesamt-Prognose-Linie (gestrichelt, schwarz)
	totalPredPts := make(plotter.XYs, steps)
	for i, x := range predXs {
		totalPredPts[i] = plotter.XY{X: x, Y: cumulPred[i]}
	}
	totalLine, err := plotter.NewLine(totalPredPts)
	if err != nil {
		log.Fatal(err)
	}
	totalLine.Color = color.RGBA{0, 0, 0, 255}
	totalLine.Width = vg.Points(2)
	totalLine.Dashes = []vg.Length{vg.Points(8), vg.Points(4)}
	p.Add(totalLine)
	p.Legend.Add("Gesamt Prognose", totalLine)

	// ── Konfidenzband Gesamt ──
	bandPoly := make(plotter.XYs, 2*steps)
	for i, x := range predXs {
		bandPoly[i] = plotter.XY{X: x, Y: cumulBandHi[i]}
	}
	for i := range predXs {
		bandPoly[steps+i] = plotter.XY{X: predXs[steps-1-i], Y: cumulBandLo[steps-1-i]}
	}
	bandPolygon, err := plotter.NewPolygon(bandPoly)
	if err != nil {
		log.Fatal(err)
	}
	bandPolygon.Color = color.RGBA{180, 180, 180, 60}
	bandPolygon.LineStyle.Width = 0
	p.Add(bandPolygon)

	// ── Ist-Werte Gesamt (Summe aller Series) ──
	if len(compareData) > 0 {
		cmpPts := make(plotter.XYs, len(compareData))
		for i, d := range compareData {
			var total float64
			for _, s := range seriesList {
				total += s.getter(d)
			}
			cmpPts[i] = plotter.XY{X: monthKeyToFloat(d.MonthKey), Y: total}
		}
		cmpLine, err := plotter.NewLine(cmpPts)
		if err != nil {
			log.Fatal(err)
		}
		cmpLine.Color = color.RGBA{220, 50, 50, 255}
		cmpLine.Width = vg.Points(2)
		p.Add(cmpLine)
		p.Legend.Add("Ist-Gesamt", cmpLine)
	}

	p.Legend.Top = true
	p.Legend.Left = true
	p.Legend.TextStyle.Font.Size = vg.Points(8)

	maxY := 0.0
	for _, v := range cumulBandHi {
		if v > maxY {
			maxY = v
		}
	}
	if maxY == 0 {
		maxY = 1000
	}
	p.Y.Min = 0
	p.Y.Max = math.Ceil(maxY/1000) * 1000 * 1.1

	if err := p.Save(20*vg.Centimeter, 12*vg.Centimeter, filePath); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Gespeichert:", filePath)
}

// ── Haupt-Einstiegspunkt ──────────────────────────────────────────────────────

func diagramsPrediction() {
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

	// Jahre gruppieren
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

	basePath := "assets/diagrams/prediction"
	now := currentYear()

	// ── Pro Jahr ──────────────────────────────────────────────────────────────
	for idx, year := range years {
		dirPath := fmt.Sprintf("%s/%d", basePath, year)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			log.Fatal(err)
		}

		// Trainingsdaten = alle Jahre VOR diesem Jahr
		var trainData []MonthlyData
		for _, y := range years[:idx] {
			trainData = append(trainData, yearDataMap[y]...)
		}
		if len(trainData) == 0 {
			// Erstes Jahr: Regression auf sich selbst (kein sinnvoller Prior)
			// Wir überspringen oder verwenden das Jahr selbst als Training
			trainData = yearDataMap[year]
		}

		// Vergleichsdaten: echte Werte des Jahres selbst (laufend oder vergangen)
		compareData := yearDataMap[year]

		// x-Bereich des Forecast-Jahres: Jan–Dez
		forecastXMin := monthKeyToFloat(year*100 + 1)
		forecastXMax := monthKeyToFloat(year*100 + 12)

		// Ticks: Monate des Jahres
		ticks := buildMonthTicks(yearDataMap[year])

		// Liegt das Jahr in der Zukunft? → kein Vergleich
		if year > now {
			compareData = nil
		}

		for _, s := range seriesList {
			filePath := fmt.Sprintf("%s/%s.png", dirPath, s.id)
			var title string
			if year > now {
				title = fmt.Sprintf("%s – Prognose %d", s.name, year)
			} else if year == now {
				title = fmt.Sprintf("%s – Prognose vs. Ist %d (laufend)", s.name, year)
			} else {
				title = fmt.Sprintf("%s – Prognose vs. Ist %d", s.name, year)
			}
			savePredictionPlot(trainData, compareData, s, ticks,
				forecastXMin, forecastXMax, title, filePath)
		}

		// Kombinierter gestapelter Plot
		var combinedTitle string
		switch {
		case year > now:
			combinedTitle = fmt.Sprintf("Gesamtprognose gestapelt – %d", year)
		case year == now:
			combinedTitle = fmt.Sprintf("Gesamtprognose vs. Ist gestapelt – %d (laufend)", year)
		default:
			combinedTitle = fmt.Sprintf("Gesamtprognose vs. Ist gestapelt – %d", year)
		}
		saveCombinedPredictionPlot(trainData, compareData, seriesList, ticks,
			forecastXMin, forecastXMax, combinedTitle,
			fmt.Sprintf("%s/combined.png", dirPath))
	}

	// ── Alltime-Prognose ──────────────────────────────────────────────────────
	alltimePath := fmt.Sprintf("%s/alltime", basePath)
	if err := os.MkdirAll(alltimePath, 0755); err != nil {
		log.Fatal(err)
	}

	// Trainingsdaten = alle vorhanden Daten
	// Forecast-Bereich: letztes bekanntes Jahr +1 und +2
	lastYear := years[len(years)-1]
	forecastStart := monthKeyToFloat(lastYear*100+1) // Beginn letztes Jahr als Ankerpunkt
	forecastEnd := monthKeyToFloat((lastYear+2)*100 + 12)

	// Alltime-Ticks: Jahre + Forecast-Jahre
	alltimeTicks := buildYearTicks(allData)
	for _, fy := range []int{lastYear + 1, lastYear + 2} {
		alltimeTicks = append(alltimeTicks, plot.Tick{
			Value: float64(fy),
			Label: fmt.Sprintf("%d*", fy),
		})
	}
	sort.Slice(alltimeTicks, func(i, j int) bool {
		return alltimeTicks[i].Value < alltimeTicks[j].Value
	})

	for _, s := range seriesList {
		filePath := fmt.Sprintf("%s/%s.png", alltimePath, s.id)
		title := fmt.Sprintf("%s – Gesamtzeitraum + Prognose", s.name)
		savePredictionPlot(allData, nil, s, alltimeTicks,
			forecastStart, forecastEnd, title, filePath)
	}

	saveCombinedPredictionPlot(allData, nil, seriesList, alltimeTicks,
		forecastStart, forecastEnd,
		"Gesamtverbrauch gestapelt – Gesamtzeitraum + Prognose",
		fmt.Sprintf("%s/combined.png", alltimePath))

	fmt.Println("\nAlle Prognose-Diagramme erfolgreich generiert.")
}