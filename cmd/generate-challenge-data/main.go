// Command generate-challenge-data creates the deterministic v3 holdout files.
// It never calls an LLM and never changes the v2 regression dataset.
package main

import (
	"flag"
	"fmt"
	"log"

	"local-review-go/internal/evalchallenge"
)

func main() {
	root := flag.String("root", ".", "repository root")
	check := flag.Bool("check", false, "verify generated artifacts are already current")
	flag.Parse()

	paths, err := evalchallenge.Generate(*root, *check)
	if err != nil {
		log.Fatal(err)
	}
	if *check {
		fmt.Printf("challenge data is reproducible (%d artifacts)\n", len(paths))
		return
	}
	for _, path := range paths {
		fmt.Printf("wrote %s\n", path)
	}
}
