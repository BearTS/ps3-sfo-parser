package main

import (
	"fmt"
	"os"

	"github.com/bearts/ps3-sfo-parser/sfo"
)

func main() {
	parser, err := sfo.NewSFOParser("PARAM.sfo")
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	for i := 0; i < parser.GetLength(); i++ {
		key, err := parser.GetKeyByIndex(i)
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}

		value, err := parser.GetValue(key)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		fmt.Printf("%s: %s\n", key, value)
	}
}
