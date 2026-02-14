package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/meridian-lex/stratavore/internal/storage"
	"github.com/meridian-lex/stratavore/internal/ui"
	"github.com/meridian-lex/stratavore/pkg/api"
	"github.com/meridian-lex/stratavore/pkg/client"
	"github.com/meridian-lex/stratavore/pkg/config"
	"github.com/spf13/cobra"
)

// getAPIClient creates configured API client
func getAPIClient() *client.Client {
	cfg, _ := config.LoadConfig()

	if grpc {
		// gRPC client
		return client.NewClient("localhost", cfg.Daemon.Port_GRPC, 1)
	} else {
		// HTTP client
		httpPort := cfg.Daemon.Port_HTTP
		if httpPort == 0 {
			httpPort = 50049 // fallback default
		}
		return client.NewClient("localhost", httpPort, 1)
	}
}

var (
	Version   = "1.4.0"
	BuildTime = "unknown"
	Commit    = "unknown"
)

var (
	flagsVar   string
	godMode    bool
	grpc       bool
	preset     string
	configFile string
)

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file path")
	rootCmd.PersistentFlags().StringVar(&flagsVar, "flags", "", "Meridian Lex flags")
	rootCmd.PersistentFlags().BoolVar(&godMode, "god", false, "God mode (full access)")
	rootCmd.PersistentFlags().StringVar(&preset, "preset", "", "Use preset configuration")
	rootCmd.PersistentFlags().BoolVar(&grpc, "grpc", false, "Use gRPC client (default false)")

	// Sub-command flags
	newCmd.Flags().StringP("path", "p", "", "Project path (default: current directory)")
	newCmd.Flags().StringP("description", "d", "", "Project description")

	launchCmd.Flags().StringSliceP("flag", "f", nil, "Meridian Lex flags")
	launchCmd.Flags().StringSliceP("capability", "c", nil, "Capabilities to enable")

	killCmd.Flags().BoolP("force", "f", false, "Force kill (SIGKILL)")

	// Register all sub-commands (each added once)
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(launchCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(killCmd)
	rootCmd.AddCommand(runnersCmd)
	rootCmd.AddCommand(projectsCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(attachCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(completionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "stratavore [project]",
	Short: "AI Development Workspace Orchestrator",
	Long: `Stratavore manages multiple Meridian Lex sessions across projects,
providing global state visibility, session resumption, and resource management.`,
	Version: fmt.Sprintf("%s (built %s, commit %s)", Version, BuildTime, Commit),
	Run:     rootHandler,
}

func rootHandler(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		// Interactive launcher (TUI)
		fmt.Println("Interactive launcher not yet implemented")
		fmt.Println("Usage: stratavore <project-name>")
		os.Exit(1)
	}

	projectName := args[0]

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Connect to database
	ctx := context.Background()
	db, err := storage.NewPostgresClient(
		ctx,
		cfg.Database.PostgreSQL.GetConnectionString(),
		5, 1,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Check for existing runners
	runners, err := db.GetActiveRunners(ctx, projectName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking runners: %v\n", err)
		os.Exit(1)
	}

	if len(runners) == 0 {
		// Launch new runner
		fmt.Printf("Launching new runner for project: %s\n", projectName)
		launchNewRunner(ctx, db, projectName, cfg)
	} else if len(runners) == 1 {
		// Offer attach or new
		fmt.Printf("Found 1 active runner for %s\n", projectName)
		fmt.Printf("  Runner ID: %s (started %v)\n", runners[0].ID, runners[0].StartedAt)
		fmt.Println("\nOptions:")
		fmt.Println("  1. Attach to existing runner")
		fmt.Println("  2. Launch new runner")
		// TODO: Interactive choice
		fmt.Println("\nAttaching to existing runner...")
	} else {
		// Show picker
		fmt.Printf("Found %d active runners for %s:\n", len(runners), projectName)
		for i, r := range runners {
			fmt.Printf("  %d. %s (started %v)\n", i+1, r.ID, r.StartedAt)
		}
		// TODO: Interactive picker
	}
}

func launchNewRunner(ctx context.Context, db *storage.PostgresClient, projectName string, cfg *config.Config) {
	// This would typically communicate with the daemon via gRPC
	// For now, just show what would happen

	fmt.Println("Would launch runner with daemon...")
	fmt.Printf("  Project: %s\n", projectName)
	if godMode {
		fmt.Println("  Mode: GOD MODE (full access)")
	}
	if preset != "" {
		fmt.Printf("  Preset: %s\n", preset)
	}
}

var newCmd = &cobra.Command{
	Use:   "new <project-name>",
	Short: "Create a new project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		apiClient := getAPIClient()
		ctx := context.Background()

		projectPath, _ := cmd.Flags().GetString("path")
		description, _ := cmd.Flags().GetString("description")

		if projectPath == "" {
			cwd, _ := os.Getwd()
			projectPath = cwd
		}

		req := &api.CreateProjectRequest{
			Name:        args[0],
			Path:        projectPath,
			Description: description,
		}

		resp, err := apiClient.CreateProject(ctx, req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating project: %v\n", err)
			os.Exit(1)
		}

		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		fmt.Printf("✓ Project '%s' created at %s\n", resp.Project.Name, resp.Project.Path)
	},
}

var launchCmd = &cobra.Command{
	Use:   "launch <project-name>",
	Short: "Launch a runner for a project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		apiClient := getAPIClient()
		ctx := context.Background()

		// Check if daemon is running
		if err := apiClient.Ping(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Daemon not running. Start with: stratavored\n")
			os.Exit(1)
		}

		projectName := args[0]
		flags, _ := cmd.Flags().GetStringSlice("flag")
		capabilities, _ := cmd.Flags().GetStringSlice("capability")

		req := &api.LaunchRunnerRequest{
			ProjectName:      projectName,
			ProjectPath:      "", // Will be looked up from project
			Flags:            flags,
			Capabilities:     capabilities,
			ConversationMode: "new",
			RuntimeType:      "process",
		}

		fmt.Printf("🚀 Launching runner for project '%s'...\n", projectName)

		resp, err := apiClient.LaunchRunner(ctx, req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		fmt.Printf("✓ Runner started: %s\n", resp.Runner.ID)
		fmt.Printf("  Status: %s\n", resp.Runner.Status)
		fmt.Printf("  Project: %s\n", resp.Runner.ProjectName)
		fmt.Printf("\nUse 'stratavore watch %s' to monitor\n", projectName)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon and runner status",
	Run: func(cmd *cobra.Command, args []string) {
		apiClient := getAPIClient()
		ctx := context.Background()

		// Check daemon health
		if err := apiClient.Ping(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Daemon: Not running\n")
			fmt.Fprintf(os.Stderr, "   Start with: stratavored\n")
			os.Exit(1)
		}

		// Get status
		resp, err := apiClient.GetStatus(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("═══════════════════════════════════════════")
		fmt.Println("  STRATAVORE STATUS")
		fmt.Println("═══════════════════════════════════════════")
		fmt.Println()
		fmt.Printf("Daemon:    %s\n", boolToStatus(resp.Daemon.Healthy))
		fmt.Printf("Updated:   %s\n", resp.Daemon.LastHeartbeat)
		fmt.Println()
		fmt.Printf("Active Runners:  %d\n", resp.Metrics.ActiveRunners)
		fmt.Printf("Active Projects: %d\n", resp.Metrics.ActiveProjects)
		fmt.Printf("Total Sessions:  %d\n", resp.Metrics.TotalSessions)
		fmt.Printf("Tokens Used:     %d\n", resp.Metrics.TokensUsed)
	},
}

var killCmd = &cobra.Command{
	Use:   "kill <runner-id>",
	Short: "Stop a running runner",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		apiClient := getAPIClient()
		ctx := context.Background()

		runnerID := args[0]
		force, _ := cmd.Flags().GetBool("force")

		resp, err := apiClient.StopRunner(ctx, runnerID, force)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		if resp.Success {
			fmt.Printf("✓ Runner %s stopped\n", runnerID)
		} else {
			fmt.Fprintf(os.Stderr, "Failed to stop runner\n")
			os.Exit(1)
		}
	},
}

var runnersCmd = &cobra.Command{
	Use:   "runners [project]",
	Short: "List active runners",
	Run: func(cmd *cobra.Command, args []string) {
		apiClient := getAPIClient()
		ctx := context.Background()

		projectName := ""
		if len(args) > 0 {
			projectName = args[0]
		}

		resp, err := apiClient.ListRunners(ctx, projectName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		if len(resp.Runners) == 0 {
			fmt.Println("No active runners")
			return
		}

		fmt.Printf("Active Runners (%d):\n\n", resp.Total)
		fmt.Println("ID        PROJECT              STATUS    UPTIME     CPU%   MEM(MB)")
		fmt.Println("─────────────────────────────────────────────────────────────────────")

		for _, r := range resp.Runners {
			startTime, _ := api.ParseTime(r.StartedAt)
			uptime := formatDuration(time.Since(startTime))

			fmt.Printf("%-8s  %-20s %-9s %-10s %5.1f  %7d\n",
				r.ID[:8],
				truncate(r.ProjectName, 20),
				r.Status,
				uptime,
				r.CPUPercent,
				r.MemoryMB)
		}
	},
}

var attachCmd = &cobra.Command{
	Use:   "attach <runner-id>",
	Short: "Attach to a running tmux session",
	Long: `Re-attach to the tmux session wrapping a running runner.

The runner must have been launched while tmux was available on the daemon host.
Use this after an SSH disconnect to resume the session.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		runnerID := args[0]
		ctx := context.Background()

		apiClient := getAPIClient()

		resp, err := apiClient.GetRunner(ctx, runnerID)
		if err != nil {
			return fmt.Errorf("failed to retrieve runner: %w", err)
		}
		if resp.Error != "" {
			return fmt.Errorf("runner not found: %s", resp.Error)
		}
		if resp.Runner == nil {
			return fmt.Errorf("runner %s not found", runnerID)
		}

		sessionName := resp.Runner.SessionName
		if sessionName == "" {
			return fmt.Errorf("runner %s has no tmux session (launched without tmux or session not recorded)", runnerID)
		}

		// Verify the tmux session is still alive
		checkCmd := exec.Command("tmux", "has-session", "-t", sessionName)
		if err := checkCmd.Run(); err != nil {
			return fmt.Errorf("tmux session %q not found — runner may have exited (runner status: %s)", sessionName, resp.Runner.Status)
		}

		fmt.Printf("Attaching to tmux session %q for runner %s...\n", sessionName, runnerID[:8])

		// Replace current process with tmux attach — stdin/stdout/stderr pass through
		attachExec := exec.Command("tmux", "attach-session", "-t", sessionName)
		attachExec.Stdin = os.Stdin
		attachExec.Stdout = os.Stdout
		attachExec.Stderr = os.Stderr
		return attachExec.Run()
	},
}

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List all projects",
	Run: func(cmd *cobra.Command, args []string) {
		apiClient := getAPIClient()
		ctx := context.Background()

		resp, err := apiClient.ListProjects(ctx, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		if len(resp.Projects) == 0 {
			fmt.Println("No projects found")
			fmt.Println("Create one with: stratavore new <project-name>")
			return
		}

		fmt.Printf("Projects (%d):\n\n", len(resp.Projects))
		fmt.Println("NAME                 STATUS    RUNNERS  SESSIONS  TOKENS")
		fmt.Println("──────────────────────────────────────────────────────────")

		for _, p := range resp.Projects {
			fmt.Printf("%-20s %-9s %2d       %4d      %s\n",
				truncate(p.Name, 20),
				p.Status,
				p.ActiveRunners,
				p.TotalSessions,
				formatNumber(p.TotalTokens))
		}
	},
}

// Helper functions

func boolToStatus(b bool) string {
	if b {
		return "✓ Running"
	}
	return "✗ Stopped"
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	} else if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	} else if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}

var watchCmd = &cobra.Command{
	Use:   "watch [project]",
	Short: "Live monitor of runners",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.LoadConfig()
		ctx := context.Background()
		db, err := storage.NewPostgresClient(
			ctx,
			cfg.Database.PostgreSQL.GetConnectionString(),
			5, 1,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Database error: %v\n", err)
			return
		}
		defer db.Close()

		monitor := ui.NewLiveMonitor(db, 2*time.Second)

		// Setup signal handler
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		go func() {
			<-sigCh
			cancel()
		}()

		if len(args) > 0 {
			// Watch specific project runners
			monitor.DisplayRunners(ctx, args[0])
		} else {
			// Watch all projects
			monitor.Display(ctx)
		}
	},
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for stratavore.

Bash:
  # Add to ~/.bashrc or ~/.bash_profile:
  source <(stratavore completion bash)

  # Or write to a file and source from profile:
  stratavore completion bash > ~/.stratavore-completion.bash
  echo 'source ~/.stratavore-completion.bash' >> ~/.bashrc

Zsh:
  # Add to ~/.zshrc (ensure compinit is enabled):
  source <(stratavore completion zsh)

  # Or with oh-my-zsh:
  stratavore completion zsh > "${fpath[1]}/_stratavore"

Fish:
  stratavore completion fish > ~/.config/fish/completions/stratavore.fish

PowerShell:
  stratavore completion powershell | Out-String | Invoke-Expression
`,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		switch args[0] {
		case "bash":
			err = rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			err = rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			err = rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			err = rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			fmt.Fprintf(os.Stderr, "Unsupported shell: %s\n", args[0])
			os.Exit(1)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating completion: %v\n", err)
			os.Exit(1)
		}
	},
}

var daemonCmd = &cobra.Command{
	Use:   "daemon [start|stop|status]",
	Short: "Manage daemon",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		action := args[0]
		switch action {
		case "start":
			fmt.Println("Starting daemon...")
			fmt.Println("(Would start stratavored)")
		case "stop":
			fmt.Println("Stopping daemon...")
		case "status":
			fmt.Println("Daemon status: Unknown")
		default:
			fmt.Printf("Unknown action: %s\n", action)
		}
	},
}
