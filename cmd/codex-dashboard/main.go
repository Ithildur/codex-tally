package main

import (
	"log"
	"os"

	"github.com/Ithildur/codex-tally/internal/dashboard"
)

// Set by release builds with -ldflags "-X main.buildVersion=...".
var buildVersion = "0.0.0-dev"

func main() {
	if err := dashboard.Run(os.Args[1:], buildVersion); err != nil {
		log.Fatal(err)
	}
}
