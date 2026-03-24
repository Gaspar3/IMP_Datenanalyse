package main

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	_ "modernc.org/sqlite"
)

const (
	January int = iota + 4
	February
	March
	April
	May
	June
	July
	August
	September
	October
	November
	December
)

func numberToLetter(n int) string {
	if n < 1 || n > 26 {
		return ""
	}
	return string(rune('A' + n - 1))
}

func convertEUseToSQL() {
	db, err := sql.Open("sqlite", "assets/data/datenbank.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS electricity_usage (
		month_key INTEGER PRIMARY KEY NOT NULL UNIQUE, -- Format: YYYYMM, z.B. 201401 = Januar 2014
		i_internat     REAL,
		i_uz_aula      REAL,
		l_internat     REAL,
		l_gemeinschaft REAL,
		p_uzp2         REAL,
		p_uzp3         REAL,
		p_uzp4         REAL,
		s_uzp4         REAL,
		s_bhkw         REAL
	);`

	if _, err = db.Exec(createTableSQL); err != nil {
		log.Fatal(err)
	}

	f, err := excelize.OpenFile("assets/xlsx/Strom_Gas_Wasserverbrauch_2013.xlsx")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	rows, err := f.GetRows("Verbrauch Strom")
	if err != nil {
		fmt.Println(err)
		return
	}

	// Zeilenindizes der Bereiche pro Jahresblock (0-basiert):
	// Jahreszeile: +0 (z.B. "2013")
	// Internat:        +3
	// UZ Aula:         +4
	// L13 Internat:    +5
	// L13 Gemeinschaft:+6
	// UZ P002:         +9
	// UZ P003:         +10
	// UZ P004:         +11
	// Sporthalle UZ P4:+16
	// Strom BHKW:      +17
	//
	// Monatswerte stehen in Spalten 3–14 (Jan–Dez), 0-basiert.

	parseFloat := func(s string) float64 {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0
		}

		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return v
	}

	getCell := func(rowIdx, colIdx int) float64 {
		if rowIdx >= len(rows) {
			return 0
		}
		row := rows[rowIdx]
		if colIdx >= len(row) {
			return 0
		}
		return parseFloat(row[colIdx])
	}

	insertSQL := `
	INSERT OR REPLACE INTO electricity_usage
		(month_key, i_internat, i_uz_aula, l_internat, l_gemeinschaft,
		 p_uzp2, p_uzp3, p_uzp4, s_uzp4, s_bhkw)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	for i, row := range rows {
		if len(row) == 0 {
			continue
		}
		yearStr := strings.TrimSpace(row[0])
		year, err := strconv.Atoi(yearStr)
		if err != nil || year < 2000 || year > 2100 {
			continue
		}

		// Monatsoffsets: Jan=3, Feb=4, ..., Dez=14 (Spaltenindex)
		for monthIdx := 0; monthIdx < 12; monthIdx++ {
			col := 3 + monthIdx
			monthKey := year*100 + monthIdx + 1 // z.B. 201301 für Jan 2013

			_, err = db.Exec(insertSQL,
				monthKey,
				getCell(i+3, col),  // Internat
				getCell(i+4, col),  // UZ Aula
				getCell(i+5, col),  // L13 Internat
				getCell(i+6, col),  // L13 Gemeinschaft
				getCell(i+9, col),  // UZ P002
				getCell(i+10, col), // UZ P003
				getCell(i+11, col), // UZ P004
				getCell(i+16, col), // Sporthalle UZ P4
				getCell(i+17, col), // Strom BHKW
			)
			if err != nil {
				log.Printf("Insert fehlgeschlagen für %d: %v", monthKey, err)
			}
		}
	}

	fmt.Println("electricity_usage erfolgreich befüllt.")
}
