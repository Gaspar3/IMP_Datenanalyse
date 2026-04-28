package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ─── Data Structures ────────────────────────────────────────────────────────

type Entry struct {
	Gender string
	Class  string
	Votes  []int // one per answer option
}

type Question struct {
	Number  int
	Text    string
	Options []string
	Entries []Entry
}

// ─── CSV Parser ─────────────────────────────────────────────────────────────

func parseCSV(path string) ([]Question, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = ';'
	r.LazyQuotes = true

	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		rows = append(rows, rec)
	}

	var questions []Question
	var currentQ *Question
	var qNum int
	var currentGender string

	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		fmt.Println("line:", row)
		// Detect question header row: col1 is a number, col2 has question text
		if col1 := strings.TrimSpace(row[0]); col1 != "" {
			fmt.Println("Haben frage")
			if n, err := strconv.Atoi(col1); err == nil {
				if currentQ != nil {
					questions = append(questions, *currentQ)
				}
				qNum = n
				// Collect options from remaining non-empty cols
				var opts []string
				for _, c := range row[4:] {
					c = strings.TrimSpace(c)
					if c != "" {
						opts = append(opts, c)
					}
				}
				text := ""
				if len(row) > 2 {
					text = strings.TrimSpace(row[2])
				}
				currentQ = &Question{Number: qNum, Text: text, Options: opts}
				currentGender = ""
				continue
			}
		}

		if currentQ == nil {
			continue
		}

		// Detect gender row: col2 is Mädchen/Jungen
		col2 := strings.TrimSpace(row[1])
		if col2 == "Mädchen" || col2 == "Jungen" ||
			col2 == "M\xc3\xa4dchen" || strings.Contains(col2, "dchen") {
			currentGender = "Mädchen"
			continue
		}
		if col2 == "Jungen" {
			currentGender = "Jungen"
			continue
		}

		// Data row: col3 is class, col4+ are vote counts
		class := ""
		if len(row) > 3 {
			class = strings.TrimSpace(row[3])
		}
		if class == "" {
			continue
		}

		var votes []int
		hasData := false
		for _, c := range row[4:] {
			c = strings.TrimSpace(c)
			if c == "" {
				votes = append(votes, -1) // missing
			} else {
				v, err := strconv.Atoi(c)
				if err == nil {
					votes = append(votes, v)
					hasData = true
				} else {
					votes = append(votes, -1)
				}
			}
		}
		if hasData && currentGender != "" {
			currentQ.Entries = append(currentQ.Entries, Entry{
				Gender: currentGender,
				Class:  class,
				Votes:  votes,
			})
		}
	}
	if currentQ != nil {
		questions = append(questions, *currentQ)
	}
	return questions, nil
}

// ─── SVG Helpers ─────────────────────────────────────────────────────────────

const svgHeader = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
`

var palette = []string{"#E84393", "#3B82F6", "#F59E0B", "#10B981", "#8B5CF6", "#EF4444", "#06B6D4", "#84CC16"}
var girlColor = "#F472B6"
var boyColor = "#60A5FA"
var bgColor = "#0F172A"
var cardBg = "#1E293B"
var textColor = "#F1F5F9"
var mutedColor = "#94A3B8"
var gridColor = "#334155"

func svgText(x, y float64, text, anchor, fill, fontSize, fontWeight string) string {
	return fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="%s" fill="%s" font-size="%s" font-weight="%s" font-family="'Segoe UI',Arial,sans-serif">%s</text>`,
		x, y, anchor, fill, fontSize, fontWeight, xmlEscape(text))
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func roundedRect(x, y, w, h, r float64, fill, stroke string, opacity float64) string {
	return fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" fill="%s" stroke="%s" stroke-width="1" opacity="%.2f"/>`,
		x, y, w, h, r, fill, stroke, opacity)
}

// ─── Chart 1: Grouped Bar Chart – all classes, girls vs boys side by side ────

func makeGroupedBarChart(q *Question, outDir string) error {
	// Gather unique classes preserving order
	classOrder := []string{}
	seen := map[string]bool{}
	for _, e := range q.Entries {
		if !seen[e.Class] {
			classOrder = append(classOrder, e.Class)
			seen[e.Class] = true
		}
	}

	// For each class & gender, find the winning option index and its vote count
	type Bar struct {
		Class       string
		Gender      string
		WinnerIdx   int
		WinnerVotes int
		WinnerLabel string
		Total       int
	}
	var bars []Bar
	maxVotes := 0
	for _, cl := range classOrder {
		for _, gender := range []string{"Mädchen", "Jungen"} {
			for _, e := range q.Entries {
				if e.Class == cl && e.Gender == gender {
					wi, wv := 0, -1
					total := 0
					for i, v := range e.Votes {
						if v > wv {
							wv = v
							wi = i
						}
						if v >= 0 {
							total += v
						}
					}
					label := ""
					if wi < len(q.Options) {
						label = q.Options[wi]
					}
					if wv > maxVotes {
						maxVotes = wv
					}
					bars = append(bars, Bar{cl, gender, wi, wv, label, total})
					break
				}
			}
		}
	}

	nClasses := len(classOrder)
	if nClasses == 0 {
		return nil
	}

	W, H := 1200.0, 600.0
	marginL, marginR, marginT, marginB := 80.0, 200.0, 80.0, 120.0
	chartW := W - marginL - marginR
	chartH := H - marginT - marginB

	groupW := chartW / float64(nClasses)
	barW := groupW * 0.35
	gap := groupW * 0.05

	var sb strings.Builder
	sb.WriteString(svgHeader)
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, W, H, W, H))
	sb.WriteString(fmt.Sprintf(`<rect width="%.0f" height="%.0f" fill="%s"/>`, W, H, bgColor))

	// Title
	title := fmt.Sprintf("Frage %d – Meistgewählte Antwort pro Klasse", q.Number)
	sb.WriteString(svgText(W/2, 45, title, "middle", textColor, "22px", "700"))
	if q.Text != "" {
		sb.WriteString(svgText(W/2, 68, q.Text, "middle", mutedColor, "14px", "400"))
	}

	// Grid lines
	gridSteps := 5
	for i := 0; i <= gridSteps; i++ {
		yv := float64(i) / float64(gridSteps) * float64(maxVotes)
		yPos := marginT + chartH - (yv/float64(maxVotes))*chartH
		sb.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="0.5" stroke-dasharray="4,4"/>`,
			marginL, yPos, marginL+chartW, yPos, gridColor))
		sb.WriteString(svgText(marginL-8, yPos+4, fmt.Sprintf("%d", int(yv)), "end", mutedColor, "11px", "400"))
	}

	// Bars
	for i, cl := range classOrder {
		cx := marginL + float64(i)*groupW + groupW/2

		for _, b := range bars {
			if b.Class != cl {
				continue
			}
			var color string
			var xOff float64
			if b.Gender == "Mädchen" {
				color = girlColor
				xOff = -barW - gap/2
			} else {
				color = boyColor
				xOff = gap / 2
			}
			bx := cx + xOff
			if b.WinnerVotes <= 0 {
				continue
			}
			bh := (float64(b.WinnerVotes) / float64(maxVotes)) * chartH
			by := marginT + chartH - bh

			// Bar gradient effect via two rects
			sb.WriteString(roundedRect(bx, by, barW, bh, 4, color, "none", 0.25))
			sb.WriteString(roundedRect(bx, by, barW, bh, 4, color, "none", 0.75))

			// Value on top
			sb.WriteString(svgText(bx+barW/2, by-6, strconv.Itoa(b.WinnerVotes), "middle", textColor, "12px", "600"))

			// Winner label inside bar if space
			if bh > 30 && b.WinnerLabel != "" {
				lbl := b.WinnerLabel
				if len(lbl) > 8 {
					lbl = lbl[:8] + "…"
				}
				sb.WriteString(svgText(bx+barW/2, by+bh-8, lbl, "middle", bgColor, "10px", "600"))
			}
		}

		// Class label
		sb.WriteString(svgText(cx, marginT+chartH+20, cl, "middle", textColor, "12px", "600"))
	}

	// Axes
	sb.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5"/>`,
		marginL, marginT, marginL, marginT+chartH, mutedColor))
	sb.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5"/>`,
		marginL, marginT+chartH, marginL+chartW, marginT+chartH, mutedColor))

	// Legend
	lx := W - marginR + 20
	sb.WriteString(roundedRect(lx-10, marginT, 180, 80, 8, cardBg, gridColor, 1))
	sb.WriteString(svgText(lx+75, marginT+22, "Legende", "middle", textColor, "13px", "700"))
	sb.WriteString(roundedRect(lx, marginT+35, 14, 14, 3, girlColor, "none", 0.9))
	sb.WriteString(svgText(lx+20, marginT+47, "Mädchen", "start", textColor, "12px", "400"))
	sb.WriteString(roundedRect(lx, marginT+58, 14, 14, 3, boyColor, "none", 0.9))
	sb.WriteString(svgText(lx+20, marginT+70, "Jungen", "start", textColor, "12px", "400"))

	sb.WriteString(`</svg>`)

	return os.WriteFile(filepath.Join(outDir, "01_grouped_bars.svg"), []byte(sb.String()), 0644)
}

// ─── Chart 2: Stacked 100% Bar – option distribution per class ───────────────

func makeStackedBarChart(q *Question, outDir string) error {
	classOrder := []string{}
	seen := map[string]bool{}
	for _, e := range q.Entries {
		if !seen[e.Class] {
			classOrder = append(classOrder, e.Class)
			seen[e.Class] = true
		}
	}

	nClasses := len(classOrder)
	if nClasses == 0 || len(q.Options) == 0 {
		return nil
	}
	nOpts := len(q.Options)

	W, H := 1400.0, 700.0
	marginL, marginR, marginT, marginB := 80.0, 220.0, 80.0, 80.0
	chartW := W - marginL - marginR
	chartH := H - marginT - marginB

	barH := (chartH / float64(nClasses)) * 0.6
	gap := (chartH / float64(nClasses)) * 0.4

	var sb strings.Builder
	sb.WriteString(svgHeader)
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, W, H, W, H))
	sb.WriteString(fmt.Sprintf(`<rect width="%.0f" height="%.0f" fill="%s"/>`, W, H, bgColor))

	title := fmt.Sprintf("Frage %d – Antwortverteilung (100%%)", q.Number)
	sb.WriteString(svgText(W/2, 45, title, "middle", textColor, "22px", "700"))
	if q.Text != "" {
		sb.WriteString(svgText(W/2, 68, q.Text, "middle", mutedColor, "14px", "400"))
	}

	// Aggregate across genders
	for i, cl := range classOrder {
		totals := make([]float64, nOpts)
		sum := 0.0
		for _, e := range q.Entries {
			if e.Class != cl {
				continue
			}
			for j := 0; j < nOpts && j < len(e.Votes); j++ {
				if e.Votes[j] >= 0 {
					totals[j] += float64(e.Votes[j])
					sum += float64(e.Votes[j])
				}
			}
		}
		if sum == 0 {
			continue
		}

		y := marginT + float64(i)*(barH+gap)
		x := marginL
		for j, t := range totals {
			w := (t / sum) * chartW
			color := palette[j%len(palette)]
			sb.WriteString(roundedRect(x, y, w, barH, 0, color, "none", 0.85))
			if w > 30 {
				pct := t / sum * 100
				sb.WriteString(svgText(x+w/2, y+barH/2+5, fmt.Sprintf("%.0f%%", pct), "middle", "#fff", "11px", "700"))
			}
			x += w
		}
		sb.WriteString(svgText(marginL-8, y+barH/2+5, cl, "end", textColor, "12px", "600"))
	}

	// Legend
	lx := W - marginR + 20
	legH := float64(nOpts)*28 + 40
	sb.WriteString(roundedRect(lx-10, marginT, 200, legH, 8, cardBg, gridColor, 1))
	sb.WriteString(svgText(lx+85, marginT+22, "Optionen", "middle", textColor, "13px", "700"))
	for j, opt := range q.Options {
		color := palette[j%len(palette)]
		ly := marginT + 35 + float64(j)*26
		sb.WriteString(roundedRect(lx, ly, 14, 14, 3, color, "none", 0.9))
		lbl := opt
		if len(lbl) > 16 {
			lbl = lbl[:16] + "…"
		}
		sb.WriteString(svgText(lx+20, ly+11, lbl, "start", textColor, "11px", "400"))
	}

	sb.WriteString(`</svg>`)
	return os.WriteFile(filepath.Join(outDir, "02_stacked_100pct.svg"), []byte(sb.String()), 0644)
}

// ─── Chart 3: Gender Comparison Dot Plot ─────────────────────────────────────

func makeGenderDotPlot(q *Question, outDir string) error {
	classOrder := []string{}
	seen := map[string]bool{}
	for _, e := range q.Entries {
		if !seen[e.Class] {
			classOrder = append(classOrder, e.Class)
			seen[e.Class] = true
		}
	}
	if len(classOrder) == 0 {
		return nil
	}

	// For each class compute dominant option per gender
	type Point struct {
		Class  string
		Girl   int // dominant option index
		Boy    int
		GScore float64 // percentage
		BScore float64
	}

	var points []Point
	for _, cl := range classOrder {
		var gEntry, bEntry *Entry
		for i := range q.Entries {
			e := &q.Entries[i]
			if e.Class != cl {
				continue
			}
			if e.Gender == "Mädchen" {
				gEntry = e
			} else {
				bEntry = e
			}
		}
		p := Point{Class: cl, Girl: -1, Boy: -1}
		if gEntry != nil {
			best, bv, total := 0, -1, 0
			for i, v := range gEntry.Votes {
				if v > bv {
					bv = v
					best = i
				}
				if v >= 0 {
					total += v
				}
			}
			if total > 0 {
				p.Girl = best
				p.GScore = float64(bv) / float64(total) * 100
			}
		}
		if bEntry != nil {
			best, bv, total := 0, -1, 0
			for i, v := range bEntry.Votes {
				if v > bv {
					bv = v
					best = i
				}
				if v >= 0 {
					total += v
				}
			}
			if total > 0 {
				p.Boy = best
				p.BScore = float64(bv) / float64(total) * 100
			}
		}
		if p.Girl >= 0 || p.Boy >= 0 {
			points = append(points, p)
		}
	}

	nOpts := len(q.Options)
	W, H := 900.0, float64(len(points))*50+200
	if H < 400 {
		H = 400
	}
	marginL, marginR, marginT, marginB := 80.0, 200.0, 100.0, 60.0
	chartW := W - marginL - marginR
	chartH := H - marginT - marginB
	rowH := chartH / float64(len(points)+1)

	var sb strings.Builder
	sb.WriteString(svgHeader)
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, W, H, W, H))
	sb.WriteString(fmt.Sprintf(`<rect width="%.0f" height="%.0f" fill="%s"/>`, W, H, bgColor))

	title := fmt.Sprintf("Frage %d – Mädchen vs. Jungen Dotplot", q.Number)
	sb.WriteString(svgText(W/2, 45, title, "middle", textColor, "22px", "700"))
	if q.Text != "" {
		sb.WriteString(svgText(W/2, 68, q.Text, "middle", mutedColor, "14px", "400"))
	}

	// X-axis: option indices
	if nOpts > 0 {
		denom := float64(nOpts) - 0.5
		for j := 0; j < nOpts; j++ {
			x := marginL + (float64(j)/denom)*chartW
			sb.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="0.5" stroke-dasharray="3,3"/>`,
				x, marginT, x, marginT+chartH, gridColor))
			lbl := q.Options[j]
			if len(lbl) > 10 {
				lbl = lbl[:10] + "…"
			}
			sb.WriteString(svgText(x, marginT+chartH+20, lbl, "middle", mutedColor, "11px", "400"))
		}
	}

	for i, p := range points {
		y := marginT + float64(i)*rowH + rowH/2

		// Row stripe
		if i%2 == 0 {
			sb.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" opacity="0.3"/>`,
				marginL, y-rowH/2, chartW, rowH, cardBg))
		}

		// Class label
		sb.WriteString(svgText(marginL-8, y+5, p.Class, "end", textColor, "12px", "600"))

		denom := float64(nOpts) - 0.5

		// Connector line between dots
		if p.Girl >= 0 && p.Boy >= 0 && nOpts > 0 {
			gx := marginL + (float64(p.Girl)/denom)*chartW
			bx := marginL + (float64(p.Boy)/denom)*chartW
			color := "#FFFFFF"
			if p.Girl == p.Boy {
				color = "#22C55E"
			} else if math.Abs(float64(p.Girl-p.Boy)) >= 2 {
				color = "#EF4444"
			}
			sb.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5" opacity="0.4"/>`,
				gx, y, bx, y, color))
		}

		// Girl dot
		if p.Girl >= 0 && nOpts > 0 {
			x := marginL + (float64(p.Girl)/denom)*chartW
			r := 6 + p.GScore/20
			sb.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" opacity="0.9"/>`, x, y, r, girlColor))
		}

		// Boy dot
		if p.Boy >= 0 && nOpts > 0 {
			x := marginL + (float64(p.Boy)/denom)*chartW
			r := 6 + p.BScore/20
			sb.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" opacity="0.9"/>`, x, y, r, boyColor))
		}
	}

	// Axes
	sb.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5"/>`,
		marginL, marginT+chartH, marginL+chartW, marginT+chartH, mutedColor))

	// Legend
	lx := W - marginR + 20
	sb.WriteString(roundedRect(lx-10, marginT, 175, 130, 8, cardBg, gridColor, 1))
	sb.WriteString(svgText(lx+75, marginT+22, "Legende", "middle", textColor, "13px", "700"))
	sb.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="8" fill="%s"/>`, lx+7, marginT+45, girlColor))
	sb.WriteString(svgText(lx+20, marginT+50, "Mädchen (Kreis∝%)", "start", textColor, "11px", "400"))
	sb.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="8" fill="%s"/>`, lx+7, marginT+70, boyColor))
	sb.WriteString(svgText(lx+20, marginT+75, "Jungen (Kreis∝%)", "start", textColor, "11px", "400"))
	sb.WriteString(roundedRect(lx, marginT+90, 14, 4, 2, "#22C55E", "none", 0.8))
	sb.WriteString(svgText(lx+20, marginT+98, "Gleiche Wahl", "start", textColor, "11px", "400"))
	sb.WriteString(roundedRect(lx, marginT+110, 14, 4, 2, "#EF4444", "none", 0.8))
	sb.WriteString(svgText(lx+20, marginT+118, "Große Differenz", "start", textColor, "11px", "400"))

	sb.WriteString(`</svg>`)
	return os.WriteFile(filepath.Join(outDir, "03_gender_dotplot.svg"), []byte(sb.String()), 0644)
}

// ─── Chart 4: Heatmap – Klasse × Option ──────────────────────────────────────

func makeHeatmap(q *Question, outDir string) error {
	classOrder := []string{}
	seen := map[string]bool{}
	for _, e := range q.Entries {
		if !seen[e.Class] {
			classOrder = append(classOrder, e.Class)
			seen[e.Class] = true
		}
	}
	if len(classOrder) == 0 || len(q.Options) == 0 {
		return nil
	}
	nOpts := len(q.Options)
	nClass := len(classOrder)

	// Build total votes matrix
	matrix := make([][]float64, nClass)
	rowMax := make([]float64, nClass)
	globalMax := 0.0
	for i, cl := range classOrder {
		matrix[i] = make([]float64, nOpts)
		for _, e := range q.Entries {
			if e.Class != cl {
				continue
			}
			for j := 0; j < nOpts && j < len(e.Votes); j++ {
				if e.Votes[j] >= 0 {
					matrix[i][j] += float64(e.Votes[j])
				}
			}
		}
		for _, v := range matrix[i] {
			if v > rowMax[i] {
				rowMax[i] = v
			}
			if v > globalMax {
				globalMax = v
			}
		}
	}

	cellW := 90.0
	cellH := 40.0
	marginL := 80.0
	marginT := 130.0
	marginB := 80.0
	W := marginL + float64(nOpts)*cellW + 40
	H := marginT + float64(nClass)*cellH + marginB

	var sb strings.Builder
	sb.WriteString(svgHeader)
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, W, H, W, H))
	sb.WriteString(fmt.Sprintf(`<rect width="%.0f" height="%.0f" fill="%s"/>`, W, H, bgColor))

	title := fmt.Sprintf("Frage %d – Heatmap Klasse × Antwort", q.Number)
	sb.WriteString(svgText(W/2, 45, title, "middle", textColor, "22px", "700"))
	if q.Text != "" {
		sb.WriteString(svgText(W/2, 68, q.Text, "middle", mutedColor, "14px", "400"))
	}

	// Column headers
	for j, opt := range q.Options {
		x := marginL + float64(j)*cellW + cellW/2
		lbl := opt
		if len(lbl) > 10 {
			lbl = lbl[:10] + "…"
		}
		sb.WriteString(svgText(x, marginT-10, lbl, "middle", textColor, "12px", "600"))
	}

	// Cells
	for i, cl := range classOrder {
		y := marginT + float64(i)*cellH
		sb.WriteString(svgText(marginL-8, y+cellH/2+5, cl, "end", textColor, "12px", "600"))

		for j := 0; j < nOpts; j++ {
			x := marginL + float64(j)*cellW
			v := matrix[i][j]
			intensity := 0.0
			if globalMax > 0 {
				intensity = v / globalMax
			}
			// Color interpolation: dark blue → bright pink
			r := int(30 + intensity*225)
			g := int(10 + intensity*20)
			b := int(80 + intensity*75)
			fill := fmt.Sprintf("#%02x%02x%02x", r, g, b)
			sb.WriteString(roundedRect(x+1, y+1, cellW-2, cellH-2, 4, fill, "none", 1))
			if v > 0 {
				textFill := textColor
				if intensity < 0.4 {
					textFill = mutedColor
				}
				sb.WriteString(svgText(x+cellW/2, y+cellH/2+5, fmt.Sprintf("%.0f", v), "middle", textFill, "13px", "700"))
			}
		}
	}

	// Color scale legend
	lx := marginL
	ly := H - marginB + 20
	sb.WriteString(svgText(lx, ly+12, "Intensität:", "start", mutedColor, "11px", "400"))
	for k := 0; k <= 10; k++ {
		intensity := float64(k) / 10
		r := int(30 + intensity*225)
		g := int(10 + intensity*20)
		b := int(80 + intensity*75)
		fill := fmt.Sprintf("#%02x%02x%02x", r, g, b)
		sx := lx + 80 + float64(k)*20
		sb.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="20" height="14" fill="%s"/>`, sx, ly, fill))
	}
	sb.WriteString(svgText(lx+80, ly+26, "0", "middle", mutedColor, "10px", "400"))
	sb.WriteString(svgText(lx+280, ly+26, fmt.Sprintf("%.0f", globalMax), "middle", mutedColor, "10px", "400"))

	sb.WriteString(`</svg>`)
	return os.WriteFile(filepath.Join(outDir, "04_heatmap.svg"), []byte(sb.String()), 0644)
}

// ─── Chart 5: Radial / Polar Area Chart – total votes per option ─────────────

func makeRadialChart(q *Question, outDir string) error {
	if len(q.Options) == 0 {
		return nil
	}
	nOpts := len(q.Options)
	totals := make([]float64, nOpts)
	max := 0.0
	for _, e := range q.Entries {
		for j := 0; j < nOpts && j < len(e.Votes); j++ {
			if e.Votes[j] >= 0 {
				totals[j] += float64(e.Votes[j])
			}
		}
	}
	for _, v := range totals {
		if v > max {
			max = v
		}
	}
	if max == 0 {
		return nil
	}

	W, H := 700.0, 700.0
	cx, cy := W/2, H/2+20
	maxR := 250.0

	var sb strings.Builder
	sb.WriteString(svgHeader)
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, W, H, W, H))
	sb.WriteString(fmt.Sprintf(`<rect width="%.0f" height="%.0f" fill="%s"/>`, W, H, bgColor))

	title := fmt.Sprintf("Frage %d – Polar Area", q.Number)
	sb.WriteString(svgText(W/2, 40, title, "middle", textColor, "22px", "700"))
	if q.Text != "" {
		sb.WriteString(svgText(W/2, 62, q.Text, "middle", mutedColor, "13px", "400"))
	}

	// Grid circles
	for k := 1; k <= 4; k++ {
		r := maxR * float64(k) / 4
		sb.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="%s" stroke-width="0.5" stroke-dasharray="4,4"/>`,
			cx, cy, r, gridColor))
		sb.WriteString(svgText(cx+r+4, cy-4, fmt.Sprintf("%.0f", max*float64(k)/4), "start", mutedColor, "10px", "400"))
	}

	angleStep := 2 * math.Pi / float64(nOpts)

	for j := 0; j < nOpts; j++ {
		startAngle := float64(j)*angleStep - math.Pi/2
		endAngle := startAngle + angleStep
		r := (totals[j] / max) * maxR

		x1 := cx + math.Cos(startAngle)*r
		y1 := cy + math.Sin(startAngle)*r
		x2 := cx + math.Cos(endAngle)*r
		y2 := cy + math.Sin(endAngle)*r

		largeArcInt := 0
		if angleStep > math.Pi {
			largeArcInt = 1
		}

		color := palette[j%len(palette)]
		path := fmt.Sprintf("M %.1f %.1f L %.1f %.1f A %.1f %.1f 0 %d 1 %.1f %.1f Z",
			cx, cy, x1, y1, r, r, largeArcInt, x2, y2)
		sb.WriteString(fmt.Sprintf(`<path d="%s" fill="%s" opacity="0.75"/>`, path, color))
		sb.WriteString(fmt.Sprintf(`<path d="%s" fill="none" stroke="%s" stroke-width="1"/>`, path, color))

		// Label
		midAngle := startAngle + angleStep/2
		lx := cx + math.Cos(midAngle)*(maxR+30)
		ly := cy + math.Sin(midAngle)*(maxR+30)
		anchor := "middle"
		if math.Cos(midAngle) > 0.1 {
			anchor = "start"
		} else if math.Cos(midAngle) < -0.1 {
			anchor = "end"
		}
		lbl := q.Options[j]
		if len(lbl) > 12 {
			lbl = lbl[:12] + "…"
		}
		sb.WriteString(svgText(lx, ly, lbl, anchor, textColor, "12px", "600"))
		sb.WriteString(svgText(lx, ly+14, fmt.Sprintf("(%.0f)", totals[j]), anchor, mutedColor, "11px", "400"))
	}

	sb.WriteString(`</svg>`)
	return os.WriteFile(filepath.Join(outDir, "05_polar_area.svg"), []byte(sb.String()), 0644)
}

// ─── Chart 6: Grade-Level Summary (5,6,7,8,9,10 aggregated) ─────────────────

func makeGradeLevelChart(q *Question, outDir string) error {
	if len(q.Options) == 0 {
		return nil
	}
	nOpts := len(q.Options)

	// Extract grade from class string
	gradeOf := func(cl string) string {
		cl = strings.ToLower(cl)
		for _, g := range []string{"5", "6", "7", "8", "9", "10"} {
			if strings.HasPrefix(cl, g) {
				return g
			}
		}
		if strings.Contains(cl, "lehrer") {
			return "L"
		}
		return "?"
	}

	type GenderGrade struct {
		Grade  string
		Gender string
	}

	agg := map[GenderGrade][]float64{}
	for _, e := range q.Entries {
		g := gradeOf(e.Class)
		key := GenderGrade{g, e.Gender}
		if _, ok := agg[key]; !ok {
			agg[key] = make([]float64, nOpts)
		}
		for j := 0; j < nOpts && j < len(e.Votes); j++ {
			if e.Votes[j] >= 0 {
				agg[key][j] += float64(e.Votes[j])
			}
		}
	}

	grades := []string{"5", "6", "7", "8", "9", "10", "L"}
	genders := []string{"Mädchen", "Jungen"}
	genderColors := []string{girlColor, boyColor}

	W, H := 1300.0, 600.0
	marginL, marginR, marginT, marginB := 80.0, 220.0, 90.0, 120.0
	chartW := W - marginL - marginR
	chartH := H - marginT - marginB

	nGrades := len(grades)
	groupW := chartW / float64(nGrades)
	barW := groupW / float64(len(genders)+1)

	maxVal := 0.0
	for _, v := range agg {
		for _, x := range v {
			if x > maxVal {
				maxVal = x
			}
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	var sb strings.Builder
	sb.WriteString(svgHeader)
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, W, H, W, H))
	sb.WriteString(fmt.Sprintf(`<rect width="%.0f" height="%.0f" fill="%s"/>`, W, H, bgColor))

	title := fmt.Sprintf("Frage %d – Jahrgangsstufen Übersicht", q.Number)
	sb.WriteString(svgText(W/2, 45, title, "middle", textColor, "22px", "700"))
	if q.Text != "" {
		sb.WriteString(svgText(W/2, 68, q.Text, "middle", mutedColor, "14px", "400"))
	}

	// Grid
	for k := 0; k <= 5; k++ {
		yv := float64(k) / 5 * maxVal
		yPos := marginT + chartH - (yv/maxVal)*chartH
		sb.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="0.5" stroke-dasharray="4,4"/>`,
			marginL, yPos, marginL+chartW, yPos, gridColor))
		sb.WriteString(svgText(marginL-8, yPos+4, fmt.Sprintf("%.0f", yv), "end", mutedColor, "11px", "400"))
	}

	for gi, grade := range grades {
		cx := marginL + float64(gi)*groupW + groupW/2
		for oi, opt := range q.Options {
			for gdi, gender := range genders {
				key := GenderGrade{grade, gender}
				vals, ok := agg[key]
				if !ok {
					continue
				}
				v := vals[oi]
				if v <= 0 {
					continue
				}
				xOff := float64(gdi)*barW - barW*float64(len(genders))/2 + float64(oi)*0.5
				bx := cx + xOff
				bh := (v / maxVal) * chartH
				by := marginT + chartH - bh
				color := genderColors[gdi]
				_ = opt
				sb.WriteString(roundedRect(bx, by, barW*0.8, bh, 3, color, "none", 0.6+float64(oi)*0.1))
			}
		}
		sb.WriteString(svgText(cx, marginT+chartH+20, "Klasse "+grade, "middle", textColor, "12px", "600"))
	}

	// Axes
	sb.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5"/>`,
		marginL, marginT, marginL, marginT+chartH, mutedColor))
	sb.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5"/>`,
		marginL, marginT+chartH, marginL+chartW, marginT+chartH, mutedColor))

	// Legend
	lx := W - marginR + 20
	legH := float64((nOpts+len(genders))*26) + 60
	sb.WriteString(roundedRect(lx-10, marginT, 200, legH, 8, cardBg, gridColor, 1))
	sb.WriteString(svgText(lx+85, marginT+22, "Legende", "middle", textColor, "13px", "700"))
	for gdi, gender := range genders {
		ly := marginT + 35 + float64(gdi)*26
		sb.WriteString(roundedRect(lx, ly, 14, 14, 3, genderColors[gdi], "none", 0.9))
		sb.WriteString(svgText(lx+20, ly+11, gender, "start", textColor, "11px", "600"))
	}
	for oi, opt := range q.Options {
		ly := marginT + 35 + float64(len(genders)+oi)*26
		sb.WriteString(roundedRect(lx, ly, 14, 14, 3, palette[oi%len(palette)], "none", 0.7))
		lbl := opt
		if len(lbl) > 16 {
			lbl = lbl[:16] + "…"
		}
		sb.WriteString(svgText(lx+20, ly+11, lbl, "start", mutedColor, "10px", "400"))
	}

	sb.WriteString(`</svg>`)
	return os.WriteFile(filepath.Join(outDir, "06_grade_level.svg"), []byte(sb.String()), 0644)
}

// ─── Chart 7: Agreement Index – how consistent each class is ─────────────────

func makeAgreementChart(q *Question, outDir string) error {
	classOrder := []string{}
	seen := map[string]bool{}
	for _, e := range q.Entries {
		if !seen[e.Class] {
			classOrder = append(classOrder, e.Class)
			seen[e.Class] = true
		}
	}
	if len(classOrder) == 0 || len(q.Options) == 0 {
		return nil
	}
	nOpts := len(q.Options)

	type Segment struct {
		Class      string
		Uniformity float64 // 0=total disagreement, 1=everyone same
		WinnerPct  float64
		WinnerLbl  string
	}
	var segs []Segment
	for _, cl := range classOrder {
		totals := make([]float64, nOpts)
		sum := 0.0
		for _, e := range q.Entries {
			if e.Class != cl {
				continue
			}
			for j := 0; j < nOpts && j < len(e.Votes); j++ {
				if e.Votes[j] >= 0 {
					totals[j] += float64(e.Votes[j])
					sum += float64(e.Votes[j])
				}
			}
		}
		if sum == 0 {
			continue
		}
		best, bv := 0, 0.0
		for j, v := range totals {
			if v > bv {
				bv = v
				best = j
			}
		}
		pct := bv / sum
		lbl := ""
		if best < len(q.Options) {
			lbl = q.Options[best]
		}
		segs = append(segs, Segment{cl, pct, pct * 100, lbl})
	}

	W, H := 900.0, 500.0
	marginL, marginR, marginT, marginB := 80.0, 200.0, 90.0, 60.0
	chartW := W - marginL - marginR
	chartH := H - marginT - marginB

	barH := chartH / float64(len(segs)+1) * 0.7
	gap := chartH / float64(len(segs)+1) * 0.3

	var sb strings.Builder
	sb.WriteString(svgHeader)
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, W, H, W, H))
	sb.WriteString(fmt.Sprintf(`<rect width="%.0f" height="%.0f" fill="%s"/>`, W, H, bgColor))

	title := fmt.Sprintf("Frage %d – Einigkeit der Klassen", q.Number)
	sb.WriteString(svgText(W/2, 45, title, "middle", textColor, "22px", "700"))
	if q.Text != "" {
		sb.WriteString(svgText(W/2, 68, q.Text, "middle", mutedColor, "14px", "400"))
	}

	// X-axis pct labels
	for k := 0; k <= 4; k++ {
		x := marginL + float64(k)/4*chartW
		sb.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="0.5" stroke-dasharray="4,4"/>`,
			x, marginT, x, marginT+chartH, gridColor))
		sb.WriteString(svgText(x, marginT+chartH+18, fmt.Sprintf("%d%%", k*25), "middle", mutedColor, "11px", "400"))
	}

	for i, s := range segs {
		y := marginT + float64(i)*(barH+gap)

		// Background bar
		sb.WriteString(roundedRect(marginL, y, chartW, barH, 4, cardBg, "none", 1))

		// Filled bar (gradient via opacity layers)
		w := s.Uniformity * chartW
		// Color: green for high agreement, red for low
		r := int(255 * (1 - s.Uniformity))
		g2 := int(255 * s.Uniformity)
		fill := fmt.Sprintf("#%02x%02x%02x", r, g2, 60)
		sb.WriteString(roundedRect(marginL, y, w, barH, 4, fill, "none", 0.85))

		// Class label
		sb.WriteString(svgText(marginL-8, y+barH/2+4, s.Class, "end", textColor, "12px", "600"))

		// Pct label
		sb.WriteString(svgText(marginL+w+6, y+barH/2+4, fmt.Sprintf("%.0f%%", s.WinnerPct), "start", textColor, "12px", "700"))

		// Winner label
		lbl := s.WinnerLbl
		if len(lbl) > 10 {
			lbl = lbl[:10] + "…"
		}
		if w > 50 {
			sb.WriteString(svgText(marginL+w/2, y+barH/2+4, lbl, "middle", bgColor, "11px", "600"))
		}
	}

	// Legend
	lx := W - marginR + 20
	sb.WriteString(roundedRect(lx-10, marginT, 175, 90, 8, cardBg, gridColor, 1))
	sb.WriteString(svgText(lx+75, marginT+22, "Einigkeit", "middle", textColor, "13px", "700"))
	// Gradient swatch
	for k := 0; k <= 5; k++ {
		t := float64(k) / 5
		r := int(255 * (1 - t))
		g2 := int(255 * t)
		fill := fmt.Sprintf("#%02x%02x%02x", r, g2, 60)
		sb.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="24" height="18" fill="%s"/>`,
			lx+float64(k)*24, marginT+35, fill))
	}
	sb.WriteString(svgText(lx, marginT+68, "Uneinig", "start", mutedColor, "10px", "400"))
	sb.WriteString(svgText(lx+144, marginT+68, "Einig", "end", mutedColor, "10px", "400"))
	sb.WriteString(svgText(lx+75, marginT+85, "← Einigkeitsgrad →", "middle", mutedColor, "10px", "400"))

	sb.WriteString(`</svg>`)
	return os.WriteFile(filepath.Join(outDir, "07_agreement.svg"), []byte(sb.String()), 0644)
}


func diagramsOfQuestions() {
	csvPath := "assets/csv/IMP_Projekt_Fragebogen_Beispiel_Frage.csv"

	questions, err := parseCSV(csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading CSV: %v\n", err)
		os.Exit(1)
	}
	if len(questions) == 0 {
		fmt.Fprintln(os.Stderr, "No questions found in CSV.")
		os.Exit(1)
	}

	fmt.Printf("Found %d question(s).\n", len(questions))

	for i := range questions {
		q := &questions[i]
		dir := fmt.Sprintf("question_%d", q.Number)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", dir, err)
			continue
		}
		fmt.Printf("\nQuestion %d: %q  (%d options, %d entries)\n",
			q.Number, q.Text, len(q.Options), len(q.Entries))

		type chartFn struct {
			name string
			fn   func(*Question, string) error
		}
		charts := []chartFn{
			{"01_grouped_bars", makeGroupedBarChart},
			{"02_stacked_100pct", makeStackedBarChart},
			{"03_gender_dotplot", makeGenderDotPlot},
			{"04_heatmap", makeHeatmap},
			{"05_polar_area", makeRadialChart},
			{"06_grade_level", makeGradeLevelChart},
			{"07_agreement", makeAgreementChart},
		}
		for _, c := range charts {
			if err := c.fn(q, dir); err != nil {
				fmt.Fprintf(os.Stderr, "  [WARN] %s: %v\n", c.name, err)
			} else {
				fmt.Printf(" %s/%s.svg\n", dir, c.name)
			}
		}
	}
	fmt.Println("\nDone! Open the .svg files in any browser.")
}