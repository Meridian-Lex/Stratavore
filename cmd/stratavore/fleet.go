package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/meridian-lex/stratavore/pkg/api"
	"github.com/meridian-lex/stratavore/pkg/config"
	"github.com/spf13/cobra"
)

var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Fleet-wide operations",
}

var fleetPRsJsonFlag bool
var fleetPRsRefreshFlag bool

var fleetPRsCmd = &cobra.Command{
	Use:   "prs",
	Short: "List open pull requests across all fleet repositories",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFleetPRs()
	},
}

func init() {
	fleetPRsCmd.Flags().BoolVar(&fleetPRsJsonFlag, "json", false, "Output as JSON")
	fleetPRsCmd.Flags().BoolVar(&fleetPRsRefreshFlag, "refresh", false, "Bypass cache and force re-fetch")
	fleetCmd.AddCommand(fleetPRsCmd)
}

func runFleetPRs() error {
	cfg, _ := config.LoadConfig()
	port := cfg.Daemon.Port_HTTP
	if port == 0 {
		port = 50049
	}

	url := fmt.Sprintf("http://localhost:%d/api/v1/fleet/prs", port)
	if fleetPRsRefreshFlag {
		url += "?refresh=true"
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to reach stratavored: %w", err)
	}
	defer resp.Body.Close()

	var data api.FleetPRsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if fleetPRsJsonFlag {
		return json.NewEncoder(os.Stdout).Encode(data)
	}

	if len(data.PRs) == 0 {
		fmt.Println("No open PRs across fleet.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REPO\t#\tTITLE\tAUTHOR\tAGE\tDRAFT")
	for _, pr := range data.PRs {
		age := formatAge(pr.CreatedAt)
		draft := ""
		if pr.Draft {
			draft = "draft"
		}
		title := pr.Title
		if len(title) > 60 {
			title = title[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t#%d\t%s\t%s\t%s\t%s\n",
			pr.Repo, pr.Number, title, pr.Author, age, draft)
	}
	w.Flush()
	fmt.Printf("\nTotal: %d  |  Cached at: %s\n", data.Total, data.CachedAt.Format(time.RFC3339))
	return nil
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
