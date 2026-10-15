// discover-sources finds reviewable RSS, Atom, JSON API, and HTML candidates
// for a user's information need. It is intentionally bounded and credential
// free; Codex can use the output to prepare a source-plan.json.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"aihot-server/internal/ingest"
)

type seedFlags []string

func (s *seedFlags) String() string { return strings.Join(*s, ",") }
func (s *seedFlags) Set(value string) error {
	value = strings.TrimSpace(value)
	if value != "" {
		*s = append(*s, value)
	}
	return nil
}

func main() {
	var seeds seedFlags
	request := flag.String("request", "", "plain-language information need")
	outPath := flag.String("out", "source-candidates.json", "output JSON path; '-' prints to stdout")
	timeout := flag.Duration("timeout", 15*time.Second, "per-request timeout")
	max := flag.Int("max", 20, "maximum candidates per seed")
	flag.Var(&seeds, "seed", "official site or endpoint seed URL; repeatable")
	flag.Parse()
	if len(seeds) == 0 {
		fmt.Fprintln(os.Stderr, "at least one -seed URL is required")
		os.Exit(2)
	}

	ctx := context.Background()
	results := make([]ingest.DiscoveryResult, 0, len(seeds))
	for _, seed := range seeds {
		result, err := ingest.DiscoverSources(ctx, seed, *request, *timeout, *max)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", seed, err)
			continue
		}
		results = append(results, result)
	}
	encoded, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
	encoded = append(encoded, '\n')
	if *outPath == "-" {
		_, _ = os.Stdout.Write(encoded)
		return
	}
	if err := os.WriteFile(*outPath, encoded, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", *outPath)
}
