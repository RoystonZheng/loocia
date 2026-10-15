// discovertools runs AI Tool discovery jobs.
//
//	AI_TOOL_DATABASE_URL=postgres://... AI_TOOL_GITHUB_TOKEN=... go run ./cmd/discovertools scheduled
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"aihot-server/internal/db"
	"aihot-server/internal/llm"
	"aihot-server/internal/tools"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "ai tool:", err)
		os.Exit(1)
	}
}

func run(command string, args []string) error {
	ctx := context.Background()
	store, discoverer, closeFn, err := newDiscoverer(ctx)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := store.EnsureSchema(ctx); err != nil {
		return fmt.Errorf("schema: %w", err)
	}

	switch command {
	case "scheduled":
		fs := flag.NewFlagSet("scheduled", flag.ExitOnError)
		actor := fs.String("actor", "cron", "operator name recorded on discovery runs")
		if err := fs.Parse(args); err != nil {
			return err
		}
		now := time.Now()
		if discoverer.Now != nil {
			now = discoverer.Now()
		}
		sum, err := discoverer.RunScheduled(ctx, now, *actor)
		if err != nil {
			return err
		}
		fmt.Printf("ai tool scheduled: checked=%d skipped=%d attempted=%d succeeded=%d partial=%d failed=%d\n",
			sum.Checked, sum.Skipped, sum.Attempted, sum.Succeeded, sum.Partial, sum.Failed)
		return scheduledSummaryError(sum)
	case "snapshot":
		fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
		if err := fs.Parse(args); err != nil {
			return err
		}
		snap, err := discoverer.SnapshotActiveTools(ctx, discoverer.StarSnapshotLimit)
		if err != nil {
			return err
		}
		fmt.Printf("ai tool snapshots: attempted=%d saved=%d skipped=%d failed=%d rate_limited=%t\n",
			snap.Attempted, snap.Saved, snap.Skipped, snap.Failed, snap.RateLimited)
		return starSnapshotSummaryError(snap)
	case "weekly":
		fs := flag.NewFlagSet("weekly", flag.ExitOnError)
		actor := fs.String("actor", "cron", "operator name recorded on discovery runs")
		if err := fs.Parse(args); err != nil {
			return err
		}
		sum, err := discoverer.RunWeekly(ctx, *actor)
		if err != nil {
			return err
		}
		fmt.Printf("ai tool weekly: attempted=%d succeeded=%d partial=%d failed=%d\n",
			sum.Attempted, sum.Succeeded, sum.Partial, sum.Failed)
		snap, err := discoverer.SnapshotActiveTools(ctx, discoverer.StarSnapshotLimit)
		if err != nil {
			return err
		}
		fmt.Printf("ai tool snapshots: attempted=%d saved=%d skipped=%d failed=%d rate_limited=%t\n",
			snap.Attempted, snap.Saved, snap.Skipped, snap.Failed, snap.RateLimited)
		if err := weeklySummaryError(sum); err != nil {
			return err
		}
		return starSnapshotSummaryError(snap)
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		configID := fs.String("config", "", "tool discovery config id")
		actor := fs.String("actor", "", "operator name recorded on discovery runs")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *configID == "" {
			return fmt.Errorf("-config is required")
		}
		result, err := discoverer.RunConfig(ctx, *configID, tools.TriggerManual, *actor)
		if err != nil {
			return err
		}
		printRunResult("run", result)
		return nil
	case "manual":
		fs := flag.NewFlagSet("manual", flag.ExitOnError)
		repoURL := fs.String("repo", "", "GitHub repository URL")
		actor := fs.String("actor", "", "operator name recorded on the manual source")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *repoURL == "" {
			return fmt.Errorf("-repo is required")
		}
		result, err := discoverer.ManualAdd(ctx, *repoURL, *actor)
		if err != nil {
			return err
		}
		fmt.Printf("ai tool manual: tool=%s created=%t duplicate=%t\n",
			result.Tool.GitHubFullName, result.Created, result.Duplicate)
		return nil
	case "reclassify-purpose":
		fs := flag.NewFlagSet("reclassify-purpose", flag.ExitOnError)
		limit := fs.Int("limit", 0, "maximum tools to reclassify; 0 means no limit")
		if err := fs.Parse(args); err != nil {
			return err
		}
		sum, err := discoverer.ReclassifyPurposeTags(ctx, *limit)
		if err != nil {
			return err
		}
		fmt.Printf("ai tool purpose reclassify: attempted=%d updated=%d skipped=%d failed=%d\n",
			sum.Attempted, sum.Updated, sum.Skipped, sum.Failed)
		return purposeReclassifySummaryError(sum)
	default:
		usage()
		return fmt.Errorf("unknown command %q", command)
	}
}

func newDiscoverer(ctx context.Context) (*tools.Store, *tools.Discoverer, func(), error) {
	dsn := os.Getenv("AI_TOOL_DATABASE_URL")
	if dsn == "" {
		return nil, nil, nil, fmt.Errorf("AI_TOOL_DATABASE_URL not set")
	}
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("db: %w", err)
	}
	store := tools.New(pool)
	client := tools.NewHTTPGitHubClient(os.Getenv("AI_TOOL_GITHUB_TOKEN"))
	client.SetDefaultTokens(tools.SplitGitHubTokens(os.Getenv("AI_TOOL_GITHUB_TOKENS")))
	if baseURL := os.Getenv("AI_TOOL_GITHUB_BASE_URL"); baseURL != "" {
		client.BaseURL = baseURL
	}
	discoverer := tools.NewDiscoverer(store, client)
	discoverer.PurposeClassifier = newToolPurposeClassifier()
	if raw := os.Getenv("AI_TOOL_GITHUB_MAX_PAGES"); raw != "" {
		maxPages, err := strconv.Atoi(raw)
		if err != nil || maxPages <= 0 {
			pool.Close()
			return nil, nil, nil, fmt.Errorf("AI_TOOL_GITHUB_MAX_PAGES must be a positive integer")
		}
		discoverer.MaxPages = maxPages
	}
	if raw := os.Getenv("AI_TOOL_GITHUB_REQUEST_INTERVAL_MS"); raw != "" {
		ms, err := strconv.Atoi(raw)
		if err != nil || ms < 0 {
			pool.Close()
			return nil, nil, nil, fmt.Errorf("AI_TOOL_GITHUB_REQUEST_INTERVAL_MS must be a non-negative integer")
		}
		discoverer.RequestInterval = time.Duration(ms) * time.Millisecond
	}
	if raw := os.Getenv("AI_TOOL_STAR_SNAPSHOT_LIMIT"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			pool.Close()
			return nil, nil, nil, fmt.Errorf("AI_TOOL_STAR_SNAPSHOT_LIMIT must be a positive integer")
		}
		discoverer.StarSnapshotLimit = limit
	}
	return store, discoverer, pool.Close, nil
}

func newToolPurposeClassifier() tools.PurposeClassifier {
	client, err := llm.NewTermsClientFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ai tool: AI purpose classification disabled: %v\n", err)
		return nil
	}
	return tools.NewLLMPurposeClassifier(client)
}

func printRunResult(label string, result tools.RunResult) {
	fmt.Printf("ai tool %s: status=%s results=%d new=%d updated=%d skipped=%d pages=%d incomplete=%t truncated=%t",
		label, result.Status, result.ResultCount, result.NewCount, result.UpdatedCount,
		result.SkippedCount, result.PagesScanned, result.IncompleteResults, result.Truncated)
	if result.ErrorClass != "" {
		fmt.Printf(" error=%s", result.ErrorClass)
	}
	fmt.Println()
}

func scheduledSummaryError(sum tools.ScheduledSummary) error {
	if sum.Partial == 0 && sum.Failed == 0 {
		return nil
	}
	return fmt.Errorf("scheduled completed with partial=%d failed=%d", sum.Partial, sum.Failed)
}

func weeklySummaryError(sum tools.WeeklySummary) error {
	if sum.Partial == 0 && sum.Failed == 0 {
		return nil
	}
	return fmt.Errorf("weekly completed with partial=%d failed=%d", sum.Partial, sum.Failed)
}

func starSnapshotSummaryError(sum tools.StarSnapshotSummary) error {
	if sum.Failed == 0 && !sum.RateLimited {
		return nil
	}
	return fmt.Errorf("star snapshots completed with failed=%d rate_limited=%t", sum.Failed, sum.RateLimited)
}

func purposeReclassifySummaryError(sum tools.PurposeReclassifySummary) error {
	if sum.Failed == 0 {
		return nil
	}
	return fmt.Errorf("purpose reclassify completed with failed=%d", sum.Failed)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  discovertools scheduled [-actor cron]")
	fmt.Fprintln(os.Stderr, "  discovertools weekly [-actor cron]")
	fmt.Fprintln(os.Stderr, "  discovertools snapshot")
	fmt.Fprintln(os.Stderr, "  discovertools run -config <id> [-actor name]")
	fmt.Fprintln(os.Stderr, "  discovertools manual -repo <github-url> [-actor name]")
	fmt.Fprintln(os.Stderr, "  discovertools reclassify-purpose [-limit n]")
}
