package main

import (
	"database/sql"
	"fmt"
	"image/color"
	"log"
	"math"
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

func monthKeyToFloat(mk int) float64 {
	year := mk / 100
	month := mk % 100
	return float64(year) + float64(month-1)/12.0
}

func diagramOfTimeToUsage() {
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

	var data []MonthlyData
	for rows.Next() {
		var d MonthlyData
		if err := rows.Scan(&d.MonthKey, &d.IInternat, &d.IUzAula, &d.LInternat,
			&d.LGemeinschaft, &d.PUzp2, &d.PUzp3, &d.PUzp4, &d.SUzp4, &d.SBhkw); err != nil {
			log.Fatal(err)
		}
		data = append(data, d)
	}

	if len(data) == 0 {
		log.Fatal("Keine Daten in der Datenbank gefunden")
	}

	type Series struct {
		name   string
		getter func(MonthlyData) float64
		color  color.RGBA
	}

	series := []Series{
		{"Internat", func(d MonthlyData) float64 { return d.IInternat }, color.RGBA{R: 31, G: 119, B: 180, A: 255}},
		{"UZ Aula", func(d MonthlyData) float64 { return d.IUzAula }, color.RGBA{R: 255, G: 127, B: 14, A: 255}},
		{"L13 Internat", func(d MonthlyData) float64 { return d.LInternat }, color.RGBA{R: 44, G: 160, B: 44, A: 255}},
		{"L13 Gemeinschaft", func(d MonthlyData) float64 { return d.LGemeinschaft }, color.RGBA{R: 214, G: 39, B: 40, A: 255}},
		{"UZ P002", func(d MonthlyData) float64 { return d.PUzp2 }, color.RGBA{R: 148, G: 103, B: 189, A: 255}},
		{"UZ P003", func(d MonthlyData) float64 { return d.PUzp3 }, color.RGBA{R: 140, G: 86, B: 75, A: 255}},
		{"UZ P004", func(d MonthlyData) float64 { return d.PUzp4 }, color.RGBA{R: 227, G: 119, B: 194, A: 255}},
		{"Sporthalle UZ P4", func(d MonthlyData) float64 { return d.SUzp4 }, color.RGBA{R: 127, G: 127, B: 127, A: 255}},
		{"Strom BHKW", func(d MonthlyData) float64 { return d.SBhkw }, color.RGBA{R: 188, G: 189, B: 34, A: 255}},
	}

	p := plot.New()
	p.Title.Text = "Stromverbrauch nach Bereich"
	p.Title.TextStyle.Font.Size = vg.Points(16)
	p.X.Label.Text = "Zeit"
	p.Y.Label.Text = "Verbrauch (kWh)"
	p.Add(plotter.NewGrid())

	// Collect all x values for tick generation
	var allX []float64
	for _, d := range data {
		allX = append(allX, monthKeyToFloat(d.MonthKey))
	}

	// Year ticks
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
	p.X.Tick.Marker = plot.ConstantTicks(ticks)

	maxVal := 0.0
	for _, d := range data {
		for _, s := range series {
			v := s.getter(d)
			if v > maxVal {
				maxVal = v
			}
		}
	}

	for _, s := range series {
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
		//line.StepStyle = draw.No
		p.Add(line)
		p.Legend.Add(s.name, line)
	}

	p.Legend.Top = true
	p.Legend.Left = false
	p.Legend.XOffs = vg.Points(5)
	p.Legend.YOffs = vg.Points(-5)
	p.Legend.TextStyle.Font.Size = vg.Points(10)

	// Round up y-max nicely
	p.Y.Min = 0
	p.Y.Max = math.Ceil(maxVal/1000) * 1000

	_ = allX // used implicitly via data loop

	width := 20 * vg.Centimeter
	height := 12 * vg.Centimeter

	if err := p.Save(width, height, "stromverbrauch.png"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Diagramm gespeichert: stromverbrauch.png")
}
