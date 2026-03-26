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

type MonthlyData struct {
	MonthKey      int
	IInternat     float64
	IUzAula       float64
	LInternat     float64
	LGemeinschaft float64
	PUzp2         float64
	PUzp3         float64
	PUzp4         float64
	SUzp4         float64
	SBhkw         float64
}

type Series struct {
	id     string
	name   string
	getter func(MonthlyData) float64
	color  color.RGBA
}

func monthKeyToFloat(mk int) float64 {
	year := mk / 100
	month := mk % 100
	return float64(year) + float64(month-1)/12.0
}

func monthKeyToMonthLabel(mk int) string {
	month := mk % 100
	months := []string{
		"", "Jan", "Feb", "Mär", "Apr", "Mai", "Jun",
		"Jul", "Aug", "Sep", "Okt", "Nov", "Dez",
	}
	if month >= 1 && month <= 12 {
		return months[month]
	}
	return fmt.Sprintf("%d", month)
}

func buildYearTicks(data []MonthlyData) []plot.Tick {
	yearSet := map[int]bool{}
	for _, d := range data {
		yearSet[d.MonthKey/100] = true
	}
	var years []int
	for y := range yearSet {
		years = append(years, y)
	}
	sort.Ints(years)

	var ticks []plot.Tick
	for _, y := range years {
		ticks = append(ticks, plot.Tick{
			Value: float64(y),
			Label: fmt.Sprintf("%d", y),
		})
	}
	return ticks
}

func buildMonthTicks(data []MonthlyData) []plot.Tick {
	var ticks []plot.Tick
	for _, d := range data {
		ticks = append(ticks, plot.Tick{
			Value: monthKeyToFloat(d.MonthKey),
			Label: monthKeyToMonthLabel(d.MonthKey),
		})
	}
	return ticks
}

func maxValue(data []MonthlyData, seriesList []Series) float64 {
	maxVal := 0.0
	for _, d := range data {
		for _, s := range seriesList {
			v := s.getter(d)
			if v > maxVal {
				maxVal = v
			}
		}
	}
	return maxVal
}

func saveSingleSeriesPlot(data []MonthlyData, s Series, ticks []plot.Tick, title, filePath string) {
	p := plot.New()
	p.Title.Text = title
	p.Title.TextStyle.Font.Size = vg.Points(16)
	p.X.Label.Text = "Zeit"
	p.Y.Label.Text = "Verbrauch (kWh)"
	p.Add(plotter.NewGrid())
	p.X.Tick.Marker = plot.ConstantTicks(ticks)

	pts := make(plotter.XYs, len(data))
	for i, d := range data {
		pts[i].X = monthKeyToFloat(d.MonthKey)
		pts[i].Y = s.getter(d)
	}

	line, err := plotter.NewLine(pts)
	if err != nil {
		log.Fatal(err)
	}
	line.Color = s.color
	line.Width = vg.Points(2)
	p.Add(line)
	p.Legend.Add(s.name, line)

	p.Legend.Top = true
	p.Legend.Left = false
	p.Legend.TextStyle.Font.Size = vg.Points(10)

	p.Y.Min = 0
	mv := maxValue(data, []Series{s})
	if mv == 0 {
		mv = 100
	}
	p.Y.Max = math.Ceil(mv/1000) * 1000
	if p.Y.Max == 0 {
		p.Y.Max = mv * 1.1
	}

	width := 20 * vg.Centimeter
	height := 12 * vg.Centimeter

	if err := p.Save(width, height, filePath); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Gespeichert:", filePath)
}

func saveAllSeriesPlot(data []MonthlyData, seriesList []Series, ticks []plot.Tick, title, filePath string) {
	p := plot.New()
	p.Title.Text = title
	p.Title.TextStyle.Font.Size = vg.Points(16)
	p.X.Label.Text = "Zeit"
	p.Y.Label.Text = "Verbrauch (kWh)"
	p.Add(plotter.NewGrid())
	p.X.Tick.Marker = plot.ConstantTicks(ticks)

	mv := maxValue(data, seriesList)

	for _, s := range seriesList {
		pts := make(plotter.XYs, len(data))
		for i, d := range data {
			pts[i].X = monthKeyToFloat(d.MonthKey)
			pts[i].Y = s.getter(d)
		}

		line, err := plotter.NewLine(pts)
		if err != nil {
			log.Fatal(err)
		}
		line.Color = s.color
		line.Width = vg.Points(1.5)
		p.Add(line)
		p.Legend.Add(s.name, line)
	}

	p.Legend.Top = true
	p.Legend.Left = false
	p.Legend.XOffs = vg.Points(5)
	p.Legend.YOffs = vg.Points(-5)
	p.Legend.TextStyle.Font.Size = vg.Points(10)

	p.Y.Min = 0
	if mv == 0 {
		mv = 100
	}
	p.Y.Max = math.Ceil(mv/1000) * 1000
	if p.Y.Max == 0 {
		p.Y.Max = mv * 1.1
	}

	width := 20 * vg.Centimeter
	height := 12 * vg.Centimeter

	if err := p.Save(width, height, filePath); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Gespeichert:", filePath)
}

func diagramsOfTimeToUsage() {
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

	// ── Daten nach Jahren gruppieren ──
	yearDataMap := map[int][]MonthlyData{}
	for _, d := range allData {
		year := d.MonthKey / 100
		yearDataMap[year] = append(yearDataMap[year], d)
	}
	var years []int
	for y := range yearDataMap {
		years = append(years, y)
	}
	sort.Ints(years)

	basePath := "assets/diagrams/time_to_usage"

	// ── Pro Jahr: Ordner + Einzeldiagramme + all.png ──
	for _, year := range years {
		yearData := yearDataMap[year]
		dirPath := fmt.Sprintf("%s/%d", basePath, year)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			log.Fatal(err)
		}

		ticks := buildMonthTicks(yearData)

		for _, s := range seriesList {
			filePath := fmt.Sprintf("%s/%s.png", dirPath, s.id)
			title := fmt.Sprintf("%s – %d", s.name, year)
			saveSingleSeriesPlot(yearData, s, ticks, title, filePath)
		}

		allFilePath := fmt.Sprintf("%s/all.png", dirPath)
		allTitle := fmt.Sprintf("Alle Zähler – %d", year)
		saveAllSeriesPlot(yearData, seriesList, ticks, allTitle, allFilePath)
	}

	// ── Alltime: Ordner + Einzeldiagramme + all.png ──
	alltimePath := fmt.Sprintf("%s/alltime", basePath)
	if err := os.MkdirAll(alltimePath, 0755); err != nil {
		log.Fatal(err)
	}

	alltimeTicks := buildYearTicks(allData)

	for _, s := range seriesList {
		filePath := fmt.Sprintf("%s/%s.png", alltimePath, s.id)
		title := fmt.Sprintf("%s – Gesamtzeitraum", s.name)
		saveSingleSeriesPlot(allData, s, alltimeTicks, title, filePath)
	}

	allFilePath := fmt.Sprintf("%s/all.png", alltimePath)
	saveAllSeriesPlot(allData, seriesList, alltimeTicks, "Alle Zähler – Gesamtzeitraum", allFilePath)

	fmt.Println("\nAlle Diagramme erfolgreich generiert.")
}
