// Generate the Cindy schema snapshot from the actual business contract.
// This command performs no project I/O and never starts a provider pipeline.
package main

import (
	"encoding/json"
	"os"

	"lumi/internal/textmcp"
)

func main() {
	if err := json.NewEncoder(os.Stdout).Encode(textmcp.Specs()); err != nil {
		panic(err)
	}
}
