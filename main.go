package main

import (
	"flag"
	"os"
)

func main() {
	flag.Parse()
	switch os.Args[1] {
	case "convert":
		convertEUseToSQL()
		// add more later
	}
}
