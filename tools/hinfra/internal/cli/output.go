package cli

import (
	"encoding/json"
	"fmt"
	"os"
)

var jsonOutput bool

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printOrJSON(v interface{}, human func()) error {
	if jsonOutput {
		return printJSON(v)
	}
	human()
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
