package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/UsmanXTech/shorty/internal/cli"
	"github.com/UsmanXTech/shorty/internal/database"
	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/teams"
)

func main() {
	baseURL := os.Getenv("SHORTY_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	client := cli.NewClient(baseURL).WithAPIKey(os.Getenv("SHORTY_API_KEY"))
	switch os.Args[1] {
	case "create":
		create(client, os.Args[2:])
	case "protect":
		protect(client, os.Args[2:])
	case "stats":
		stats(client, os.Args[2:])
	case "import":
		importCSV(client, os.Args[2:])
	case "export":
		exportCSV(client, os.Args[2:])
	case "variant":
		variantCmd(client, os.Args[2:])
	case "domain":
		domainCmd(client, os.Args[2:])
	case "webhook":
		webhookCmd(client, os.Args[2:])
	case "team":
		teamCmd(client, os.Args[2:])
	case "key":
		keyCmd(client, os.Args[2:])
	case "bootstrap-admin":
		bootstrapAdmin(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func create(client *cli.Client, args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	rawURL := fs.String("url", "", "destination URL (required)")
	slug := fs.String("slug", "", "custom slug")
	expires := fs.String("expires-at", "", "expiration time in RFC3339")
	maxClicks := fs.Int64("max-clicks", 0, "maximum number of clicks")
	password := fs.String("password", "", "password protecting the link")
	domain := fs.String("domain", "", "custom domain serving the link")
	fs.Parse(args)
	if *rawURL == "" {
		fmt.Fprintln(os.Stderr, "create: --url is required")
		os.Exit(2)
	}
	link := links.Link{URL: *rawURL, Slug: *slug}
	if *expires != "" {
		t, err := time.Parse(time.RFC3339, *expires)
		if err != nil {
			fmt.Fprintln(os.Stderr, "create: --expires-at must be RFC3339")
			os.Exit(2)
		}
		link.ExpiresAt = &t
	}
	if *maxClicks > 0 {
		link.MaxClicks = maxClicks
	} else if *maxClicks < 0 {
		fmt.Fprintln(os.Stderr, "create: --max-clicks must be positive")
		os.Exit(2)
	}
	// Resolve --domain for both the plain and password-protected paths.
	if *domain != "" {
		domains, derr := client.ListDomains()
		if derr != nil {
			fmt.Fprintln(os.Stderr, derr)
			os.Exit(1)
		}
		found := false
		for _, d := range domains {
			if strings.EqualFold(d.Domain, *domain) {
				id := d.ID
				link.DomainID = &id
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintln(os.Stderr, "create: unknown domain (add it first with: shorty-cli domain add --domain "+*domain+")")
			os.Exit(2)
		}
	}
	var out links.Link
	var err error
	if *password != "" {
		out, err = client.CreateProtectedLink(link, *password)
	} else {
		out, err = client.CreateLink(link)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	printJSON(out)
}

func protect(client *cli.Client, args []string) {
	fs := flag.NewFlagSet("protect", flag.ExitOnError)
	id := fs.Int64("id", 0, "link ID (required)")
	password := fs.String("password", "", "password (empty clears protection)")
	fs.Parse(args)
	if *id <= 0 {
		fmt.Fprintln(os.Stderr, "protect: --id is required")
		os.Exit(2)
	}
	out, err := client.SetLinkPassword(*id, *password)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	printJSON(out)
}

func stats(client *cli.Client, args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	id := fs.Int64("id", 0, "link ID (required)")
	from := fs.String("from", "", "start time in RFC3339")
	to := fs.String("to", "", "end time in RFC3339")
	interval := fs.String("interval", "hour", "aggregation interval: hour or day")
	fs.Parse(args)
	if *id <= 0 {
		fmt.Fprintln(os.Stderr, "stats: --id is required")
		os.Exit(2)
	}
	parse := func(raw string) time.Time {
		if raw == "" {
			return time.Time{}
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "stats: dates must be RFC3339")
			os.Exit(2)
		}
		return t
	}
	out, err := client.Stats(*id, parse(*from), parse(*to), *interval)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	printJSON(out)
}

func importCSV(client *cli.Client, args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	file := fs.String("file", "", "CSV file to import (required)")
	fs.Parse(args)
	if *file == "" {
		fmt.Fprintln(os.Stderr, "import: --file is required")
		os.Exit(2)
	}
	out, err := client.ImportCSV(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	printJSON(out)
}

func exportCSV(client *cli.Client, args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	file := fs.String("file", "", "write CSV to file (default: stdout)")
	fs.Parse(args)
	out := os.Stdout
	if *file != "" {
		f, err := os.Create(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		out = f
	}
	if err := client.ExportCSV(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func variantCmd(client *cli.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "variant: add|list|rm")
		os.Exit(2)
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("variant add", flag.ExitOnError)
		id := fs.Int64("id", 0, "link ID (required)")
		rawURL := fs.String("url", "", "variant destination URL (required)")
		weight := fs.Int("weight", 1, "traffic weight")
		fs.Parse(args[1:])
		if *id <= 0 || *rawURL == "" {
			fmt.Fprintln(os.Stderr, "variant add: --id and --url are required")
			os.Exit(2)
		}
		out, err := client.AddVariant(*id, *rawURL, *weight)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printJSON(out)
	case "list":
		fs := flag.NewFlagSet("variant list", flag.ExitOnError)
		id := fs.Int64("id", 0, "link ID (required)")
		fs.Parse(args[1:])
		if *id <= 0 {
			fmt.Fprintln(os.Stderr, "variant list: --id is required")
			os.Exit(2)
		}
		out, err := client.ListVariants(*id)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printJSON(out)
	case "rm":
		fs := flag.NewFlagSet("variant rm", flag.ExitOnError)
		id := fs.Int64("id", 0, "link ID (required)")
		vid := fs.Int64("variant-id", 0, "variant ID (required)")
		fs.Parse(args[1:])
		if *id <= 0 || *vid <= 0 {
			fmt.Fprintln(os.Stderr, "variant rm: --id and --variant-id are required")
			os.Exit(2)
		}
		if err := client.RemoveVariant(*id, *vid); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("variant removed")
	default:
		fmt.Fprintln(os.Stderr, "variant: add|list|rm")
		os.Exit(2)
	}
}

func domainCmd(client *cli.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "domain: add|list|rm")
		os.Exit(2)
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("domain add", flag.ExitOnError)
		domain := fs.String("domain", "", "custom domain (required)")
		fs.Parse(args[1:])
		if *domain == "" {
			fmt.Fprintln(os.Stderr, "domain add: --domain is required")
			os.Exit(2)
		}
		out, err := client.AddDomain(*domain)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printJSON(out)
	case "list":
		out, err := client.ListDomains()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printJSON(out)
	case "rm":
		fs := flag.NewFlagSet("domain rm", flag.ExitOnError)
		id := fs.Int64("id", 0, "domain ID (required)")
		fs.Parse(args[1:])
		if *id <= 0 {
			fmt.Fprintln(os.Stderr, "domain rm: --id is required")
			os.Exit(2)
		}
		if err := client.RemoveDomain(*id); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("domain removed")
	default:
		fmt.Fprintln(os.Stderr, "domain: add|list|rm")
		os.Exit(2)
	}
}

func webhookCmd(client *cli.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "webhook: add|list|rm|deliveries")
		os.Exit(2)
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("webhook add", flag.ExitOnError)
		url := fs.String("url", "", "webhook URL (required)")
		events := fs.String("events", "", "comma-separated events (default: all)")
		fs.Parse(args[1:])
		if *url == "" {
			fmt.Fprintln(os.Stderr, "webhook add: --url is required")
			os.Exit(2)
		}
		var ev []string
		if *events != "" {
			ev = strings.Split(*events, ",")
		}
		out, err := client.AddWebhook(*url, ev)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printJSON(out)
	case "list":
		out, err := client.ListWebhooks()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printJSON(out)
	case "rm":
		fs := flag.NewFlagSet("webhook rm", flag.ExitOnError)
		id := fs.Int64("id", 0, "webhook ID (required)")
		fs.Parse(args[1:])
		if *id <= 0 {
			fmt.Fprintln(os.Stderr, "webhook rm: --id is required")
			os.Exit(2)
		}
		if err := client.RemoveWebhook(*id); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("webhook removed")
	case "deliveries":
		fs := flag.NewFlagSet("webhook deliveries", flag.ExitOnError)
		id := fs.Int64("id", 0, "webhook ID (required)")
		fs.Parse(args[1:])
		if *id <= 0 {
			fmt.Fprintln(os.Stderr, "webhook deliveries: --id is required")
			os.Exit(2)
		}
		out, err := client.ListDeliveries(*id)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printJSON(out)
	default:
		fmt.Fprintln(os.Stderr, "webhook: add|list|rm|deliveries")
		os.Exit(2)
	}
}

func teamCmd(client *cli.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "team: create|list")
		os.Exit(2)
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("team create", flag.ExitOnError)
		name := fs.String("name", "", "team name (required)")
		fs.Parse(args[1:])
		if *name == "" {
			fmt.Fprintln(os.Stderr, "team create: --name is required")
			os.Exit(2)
		}
		out, err := client.CreateTeam(*name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printJSON(out)
	case "list":
		out, err := client.ListTeams()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printJSON(out)
	default:
		fmt.Fprintln(os.Stderr, "team: create|list")
		os.Exit(2)
	}
}

func keyCmd(client *cli.Client, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "key: create|list|rm")
		os.Exit(2)
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("key create", flag.ExitOnError)
		teamID := fs.Int64("team-id", 0, "team ID (required)")
		name := fs.String("name", "", "key name (required)")
		admin := fs.Bool("admin", false, "mint an admin key (requires an admin API key)")
		fs.Parse(args[1:])
		if *teamID <= 0 || *name == "" {
			fmt.Fprintln(os.Stderr, "key create: --team-id and --name are required")
			os.Exit(2)
		}
		out, err := client.CreateKey(*teamID, *name, *admin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printJSON(out)
	case "list":
		fs := flag.NewFlagSet("key list", flag.ExitOnError)
		teamID := fs.Int64("team-id", 0, "team ID (required)")
		fs.Parse(args[1:])
		if *teamID <= 0 {
			fmt.Fprintln(os.Stderr, "key list: --team-id is required")
			os.Exit(2)
		}
		out, err := client.ListKeys(*teamID)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printJSON(out)
	case "rm":
		fs := flag.NewFlagSet("key rm", flag.ExitOnError)
		teamID := fs.Int64("team-id", 0, "team ID (required)")
		keyID := fs.Int64("id", 0, "key ID (required)")
		fs.Parse(args[1:])
		if *teamID <= 0 || *keyID <= 0 {
			fmt.Fprintln(os.Stderr, "key rm: --team-id and --id are required")
			os.Exit(2)
		}
		if err := client.DeleteKey(*teamID, *keyID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("api key removed")
	default:
		fmt.Fprintln(os.Stderr, "key: create|list|rm")
		os.Exit(2)
	}
}

func printJSON(value any) {
	data, _ := json.MarshalIndent(value, "", "  ")
	fmt.Println(string(data))
}

// bootstrapAdmin mints the first admin API key directly against the database.
// Team creation and listing require an admin key, so this command solves the
// chicken-and-egg problem for fresh installs. It needs filesystem access to
// the database, not an API key.
func bootstrapAdmin(args []string) {
	fs := flag.NewFlagSet("bootstrap-admin", flag.ExitOnError)
	name := fs.String("name", "bootstrap", "key name")
	teamID := fs.Int64("team-id", teams.DefaultTeamID, "team ID for the admin key")
	dbPath := fs.String("db", "", "database path (default: SHORTY_DB or shorty.db)")
	fs.Parse(args)

	path := *dbPath
	if path == "" {
		path = os.Getenv("SHORTY_DB")
		if path == "" {
			path = "shorty.db"
		}
	}
	db, err := database.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not open database:", err)
		os.Exit(1)
	}
	defer db.Close()

	store := teams.NewSQLiteStore(db.DB)
	if _, err := store.GetTeam(*teamID); err != nil {
		fmt.Fprintln(os.Stderr, "could not create admin key: team does not exist (create it after bootstrapping, or use the default team)")
		os.Exit(1)
	}
	key, err := store.CreateKey(*teamID, *name, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not create admin key:", err)
		os.Exit(1)
	}
	fmt.Println("Admin API key created (shown once — store it safely):")
	fmt.Println("  " + key.Plaintext)
	fmt.Println("Use it via SHORTY_API_KEY to manage teams and keys.")
}

func usage() {
	fmt.Println("Shorty CLI - REST client for the Shorty server")
	fmt.Println("Usage:")
	fmt.Println("  shorty-cli create --url URL [--slug SLUG] [--expires-at RFC3339] [--max-clicks N] [--password PW] [--domain DOMAIN]")
	fmt.Println("  shorty-cli protect --id ID [--password PW]   # empty password clears protection")
	fmt.Println("  shorty-cli stats --id ID [--from RFC3339] [--to RFC3339] [--interval hour|day]")
	fmt.Println("  shorty-cli import --file links.csv")
	fmt.Println("  shorty-cli export [--file links.csv]")
	fmt.Println("  shorty-cli variant add --id ID --url URL [--weight N]")
	fmt.Println("  shorty-cli variant list --id ID")
	fmt.Println("  shorty-cli variant rm --id ID --variant-id VID")
	fmt.Println("  shorty-cli domain add --domain example.com")
	fmt.Println("  shorty-cli domain list")
	fmt.Println("  shorty-cli domain rm --id ID")
	fmt.Println("  shorty-cli webhook add --url URL [--events link.created,link.clicked]")
	fmt.Println("  shorty-cli webhook list")
	fmt.Println("  shorty-cli webhook rm --id ID")
	fmt.Println("  shorty-cli webhook deliveries --id ID")
	fmt.Println("  shorty-cli team create --name NAME")
	fmt.Println("  shorty-cli team list")
	fmt.Println("  shorty-cli key create --team-id ID --name NAME [--admin]")
	fmt.Println("  shorty-cli key list --team-id ID")
	fmt.Println("  shorty-cli key rm --team-id ID --id KEYID")
	fmt.Println("  shorty-cli bootstrap-admin [--name NAME] [--team-id ID] [--db PATH]  # first admin key, direct DB access")
	fmt.Println("Environment: SHORTY_URL (default http://localhost:8080), SHORTY_API_KEY, SHORTY_DB")
}
