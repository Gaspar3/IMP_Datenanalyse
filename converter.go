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

	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
		}
	}(db)

	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS electricity_usage (
		id INTEGER PRIMARY KEY,
		i_internat INTEGER,
		i_uz_aula INTEGER,
		l_internat INTEGER,
		l_gemeinschaft INTEGER,
		p_uzp2 INTEGER,
		p_uzp3 INTEGER,
		p_uzp4 INTEGER,
		s_uzp4 INTEGER,
		s_bhkw INTEGER
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatal(err)
	}

	f, err := excelize.OpenFile("assets/xlsx/Strom_Gas_Wasserverbrauch_2013.xlsx")

	rows, err := f.GetRows("Verbrauch Strom")
	if err != nil {
		fmt.Println(err)
		return
	}

	lastYear := 0

	for _, row := range rows {
		index := row[0]

		if len(index) == 4 && strings.HasPrefix(index, "20") {
			tmpLastYear, err := strconv.Atoi(index)
			if err == nil {
				lastYear = tmpLastYear
			}
		}

		for month := 0; month < 12; month++ {
			i, err := strconv.Atoi(row[4+month])
			if err == nil {
				fmt.Printf("%v: %v\n", lastYear, i)
			}
		}

	}
}
