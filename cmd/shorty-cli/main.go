package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/UsmanXTech/shorty/internal/cli"
	"github.com/UsmanXTech/shorty/internal/links"
)

func main() {
	baseURL := os.Getenv("SHORTY_URL")
	if baseURL == "" { baseURL = "http://localhost:8080" }
	if len(os.Args) < 2 { usage(); os.Exit(2) }

	client := cli.NewClient(baseURL)
	switch os.Args[1] {
	case "create":
		create(client, os.Args[2:])
	case "stats":
		stats(client, os.Args[2:])
	default:
		usage(); os.Exit(2)
	}
}

func create(client *cli.Client, args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	rawURL := fs.String("url", "", "destination URL (required)")
	slug := fs.String("slug", "", "custom slug")
	expires := fs.String("expires-at", "", "expiration time in RFC3339")
	maxClicks := fs.Int64("max-clicks", 0, "maximum number of clicks")
	fs.Parse(args)
	if *rawURL == "" { fmt.Fprintln(os.Stderr, "create: --url is required"); os.Exit(2) }
	link := links.Link{URL: *rawURL, Slug: *slug}
	if *expires != "" { t, err := time.Parse(time.RFC3339, *expires); if err != nil { fmt.Fprintln(os.Stderr, "create: --expires-at must be RFC3339"); os.Exit(2) }; link.ExpiresAt = &t }
	if *maxClicks > 0 { link.MaxClicks = maxClicks } else if *maxClicks < 0 { fmt.Fprintln(os.Stderr, "create: --max-clicks must be positive"); os.Exit(2) }
	out, err := client.CreateLink(link); if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	printJSON(out)
}

func stats(client *cli.Client, args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	id := fs.Int64("id", 0, "link ID (required)")
	from := fs.String("from", "", "start time in RFC3339")
	to := fs.String("to", "", "end time in RFC3339")
	interval := fs.String("interval", "hour", "aggregation interval: hour or day")
	fs.Parse(args)
	if *id <= 0 { fmt.Fprintln(os.Stderr, "stats: --id is required"); os.Exit(2) }
	parse := func(raw string) time.Time { if raw == "" { return time.Time{} }; t, err := time.Parse(time.RFC3339, raw); if err != nil { fmt.Fprintln(os.Stderr, "stats: dates must be RFC3339"); os.Exit(2) }; return t }
	out, err := client.Stats(*id, parse(*from), parse(*to), *interval); if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	printJSON(out)
}

func printJSON(value any) { data, _ := json.MarshalIndent(value, "", "  "); fmt.Println(string(data)) }

func usage() {
	fmt.Println("Shorty CLI - REST client for the Shorty server")
	fmt.Println("Usage:")
	fmt.Println("  shorty-cli create --url URL [--slug SLUG] [--expires-at RFC3339] [--max-clicks N]")
	fmt.Println("  shorty-cli stats --id ID [--from RFC3339] [--to RFC3339] [--interval hour|day]")
	fmt.Println("Environment: SHORTY_URL (default http://localhost:8080)")
	_ = strconv.IntSize
}
