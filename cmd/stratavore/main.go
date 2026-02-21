package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/meridian-lex/stratavore/internal/session"
	"github.com/meridian-lex/stratavore/internal/storage"
	"github.com/meridian-lex/stratavore/internal/ui"
	"github.com/meridian-lex/stratavore/pkg/api"
	"github.com/meridian-lex/stratavore/pkg/client"
	"github.com/meridian-lex/stratavore/pkg/config"
	"github.com/meridian-lex/stratavore/pkg/types"
	"github.com/spf13/cobra"
)

// getAPIClient creates configured API client.
// The --grpc flag is reserved for a future native gRPC client; for now the
// HTTP client is always used. Using --grpc with the gRPC port (50051) sends
// HTTP/1.1 to an HTTP/2 endpoint and fails with a protocol error.
func getAPIClient() *client.Client {
	cfg, _ := config.LoadConfig()

	httpPort := cfg.Daemon.Port_HTTP
	if httpPort == 0 {
		httpPort = 8080 // fallback default
	}
	return client.NewClient("localhost", httpPort, 1)
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
	rootCmd.AddCommand(tasksCmd)
	rootCmd.AddCommand(modeCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(projectsCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(attachCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(continueCmd)
	rootCmd.AddCommand(fleetCmd)
	rootCmd.AddCommand(stateCmd)
	rootCmd.AddCommand(completionCmd)

	// Register projects sub-commands
	projectsCmd.AddCommand(projectsDeleteCmd)
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
		showInteractiveLauncher()
		return
	}

	// Smart launch with project name
	projectName := args[0]
	smartLaunch(projectName)
}

// showInteractiveLauncher displays the interactive TUI menu
func showInteractiveLauncher() {
	for {
		fmt.Println("\nStratavore Launcher")
		fmt.Println("──────────────────")
		fmt.Println("")

		menuItems := []string{
			"── Context ──",
			"Select Project",
			"Select Project (Full Access)",
			"New Project",
			"",
			"── Information ──",
			"Projects Overview",
			"Active Runners",
			"State (not implemented)",
			"Task Queue (not implemented)",
			"",
			"── Configuration ──",
			"Show Config (not implemented)",
			"Operational Mode (not implemented)",
			"Token Budget (not implemented)",
			"",
			"Exit",
		}

		prompt := promptui.Select{
			Label: "Choose action",
			Items: menuItems,
			Size:  20,
			Templates: &promptui.SelectTemplates{
				Label:    "{{ . }}",
				Active:   "\U0001F680 {{ . | cyan }}",
				Inactive: "  {{ . }}",
				Selected: "\U0001F680 {{ . | green }}",
			},
		}

		idx, result, err := prompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed: %v\n", err)
			return
		}

		// Skip separator rows
		if result == "" || strings.HasPrefix(result, "──") {
			continue
		}

		// Route selection
		switch result {
		case "Select Project":
			selectProject(false)
		case "Select Project (Full Access)":
			selectProject(true)
		case "New Project":
			fmt.Print("\nProject name: ")
			var name string
			fmt.Scanln(&name)
			if name != "" {
				newCmd.Run(newCmd, []string{name})
			}
		case "Projects Overview":
			projectsCmd.Run(projectsCmd, []string{})
		case "Active Runners":
			runnersCmd.Run(runnersCmd, []string{})
		case "State (not implemented)", "Task Queue (not implemented)",
		     "Show Config (not implemented)", "Operational Mode (not implemented)",
		     "Token Budget (not implemented)":
			fmt.Printf("\n%s - coming in Phase 2 Tier 2\n", result)
		case "Exit":
			fmt.Println("\nExiting Stratavore launcher.")
			return
		default:
			if idx == len(menuItems)-1 {
				return
			}
		}
	}
}

// selectProject shows project picker and launches selected project
func selectProject(godMode bool) {
	apiClient := getAPIClient()
	ctx := context.Background()

	resp, err := apiClient.ListProjects(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
		return
	}

	if len(resp.Projects) == 0 {
		fmt.Println("\nNo projects found")
		fmt.Println("Create one with: New Project")
		return
	}

	projectNames := make([]string, len(resp.Projects))
	for i, p := range resp.Projects {
		projectNames[i] = fmt.Sprintf("%-20s [%s, %d runners]",
			truncate(p.Name, 20), p.Status, p.ActiveRunners)
	}

	prompt := promptui.Select{
		Label: "Select project to launch",
		Items: projectNames,
		Size:  10,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return
	}

	selectedProject := resp.Projects[idx].Name
	fmt.Printf("\nLaunching: %s\n", selectedProject)

	// Use smart launch
	smartLaunch(selectedProject)
}

// smartLaunch implements smart launch with resume detection (Task 20 logic)
func smartLaunch(projectName string) {
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
		// No runners → launch new
		fmt.Printf("Launching new runner for project: %s\n", projectName)
		launchNewRunner(ctx, db, projectName, cfg)
	} else if len(runners) == 1 {
		// One runner → offer attach or new
		showSingleRunnerChoice(runners[0], projectName, ctx, db, cfg)
	} else {
		// Multiple runners → show picker
		showRunnerPicker(runners, projectName, ctx, db, cfg)
	}
}

// showSingleRunnerChoice presents attach vs new options for single runner
func showSingleRunnerChoice(runner *types.Runner, projectName string, ctx context.Context, db *storage.PostgresClient, cfg *config.Config) {
	fmt.Printf("\nFound 1 active runner for %s\n", projectName)
	fmt.Printf("  Runner ID: %s (started %v)\n", runner.ID, runner.StartedAt)

	options := []string{
		"Attach to existing runner",
		"Launch new runner",
	}

	prompt := promptui.Select{
		Label: "Choose action",
		Items: options,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return
	}

	if idx == 0 {
		// Attach
		attachCmd.Run(attachCmd, []string{runner.ID})
	} else {
		// Launch new
		fmt.Printf("Launching new runner for project: %s\n", projectName)
		launchNewRunner(ctx, db, projectName, cfg)
	}
}

// showRunnerPicker shows multi-runner picker (Task 22 implementation)
func showRunnerPicker(runners []*types.Runner, projectName string, ctx context.Context, db *storage.PostgresClient, cfg *config.Config) {
	fmt.Printf("\nFound %d active runners for %s:\n", len(runners), projectName)

	runnerLabels := make([]string, len(runners)+1)
	for i, r := range runners {
		uptime := time.Since(r.StartedAt).Round(time.Second)
		runnerLabels[i] = fmt.Sprintf("%s (started %v ago)",
			truncate(r.ID, 30), uptime)
	}
	runnerLabels[len(runners)] = "Launch new runner"

	prompt := promptui.Select{
		Label: "Select runner to attach or launch new",
		Items: runnerLabels,
		Size:  10,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return
	}

	if idx == len(runners) {
		// Launch new
		fmt.Printf("Launching new runner for project: %s\n", projectName)
		launchNewRunner(ctx, db, projectName, cfg)
	} else {
		// Attach to selected
		attachCmd.Run(attachCmd, []string{runners[idx].ID})
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

var tasksCmd = &cobra.Command{
	Use:   "tasks [project]",
	Short: "Display session queue (task list)",
	Run: func(cmd *cobra.Command, args []string) {
		apiClient := getAPIClient()
		ctx := context.Background()

		projectName := ""
		if len(args) > 0 {
			projectName = args[0]
		}

		resp, err := apiClient.ListSessions(ctx, projectName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		if len(resp.Sessions) == 0 {
			if projectName != "" {
				fmt.Printf("No sessions found for project: %s\n", projectName)
			} else {
				fmt.Println("No sessions found")
			}
			return
		}

		fmt.Printf("Sessions (%d):\n\n", len(resp.Sessions))
		fmt.Println("ID        PROJECT              STATUS    STARTED              DURATION   TOKENS     SUMMARY")
		fmt.Println("──────────────────────────────────────────────────────────────────────────────────────────────────────────")

		for _, s := range resp.Sessions {
			// Calculate status
			status := "active"
			if s.EndedAt != nil {
				status = "complete"
			}

			// Calculate duration
			var duration string
			if s.EndedAt != nil {
				dur := s.EndedAt.Sub(s.StartedAt)
				duration = formatDuration(dur)
			} else {
				dur := time.Since(s.StartedAt)
				duration = formatDuration(dur) + "*"
			}

			// Format started time
			startedStr := s.StartedAt.Format("2006-01-02 15:04")

			// Truncate summary
			summary := truncate(s.Summary, 40)
			if summary == "" {
				summary = "-"
			}

			fmt.Printf("%-8s  %-20s %-9s %-20s %-10s %-10s %s\n",
				truncate(s.ID, 8),
				truncate(s.ProjectName, 20),
				status,
				startedStr,
				duration,
				formatNumber(s.TokensUsed),
				summary)
		}

		fmt.Println("\n* = active (duration still accumulating)")
	},
}

var modeCmd = &cobra.Command{
	Use:   "mode [get|set]",
	Short: "Manage operational mode",
	Long: `Get or set the operational mode.

Modes:
  IDLE           - Default state, awaiting instructions
  AUTONOMOUS     - Self-directed operation within defined parameters
  DIRECTED       - Following explicit instructions from commander
  COLLABORATIVE  - Interactive partnership mode

Usage:
  stratavore mode              Show current mode
  stratavore mode get          Show current mode
  stratavore mode set <MODE>   Set new operational mode`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 || args[0] == "get" {
			// Get mode
			apiClient := getAPIClient()
			ctx := context.Background()

			resp, err := apiClient.GetMode(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if resp.Error != "" {
				fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
				os.Exit(1)
			}

			fmt.Printf("Operational Mode: %s\n", resp.Mode)
			if resp.Description != "" {
				fmt.Printf("Description: %s\n", resp.Description)
			}
		} else if args[0] == "set" {
			if len(args) < 2 {
				fmt.Fprintf(os.Stderr, "Error: mode required\n")
				fmt.Fprintf(os.Stderr, "Usage: stratavore mode set <MODE> [description]\n")
				os.Exit(1)
			}

			mode := args[1]
			description := ""
			if len(args) > 2 {
				description = strings.Join(args[2:], " ")
			}

			apiClient := getAPIClient()
			ctx := context.Background()

			resp, err := apiClient.SetMode(ctx, mode, description)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if resp.Error != "" {
				fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
				os.Exit(1)
			}

			if resp.Success {
				fmt.Printf("Operational mode set to: %s\n", resp.Mode)
				if description != "" {
					fmt.Printf("Description: %s\n", description)
				}
			} else {
				fmt.Fprintf(os.Stderr, "Failed to set mode\n")
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", args[0])
			fmt.Fprintf(os.Stderr, "Use 'stratavore mode get' or 'stratavore mode set <MODE>'\n")
			os.Exit(1)
		}
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Display Stratavore configuration",
	Long: `Show the current Stratavore configuration.

Displays sanitized configuration values including:
  - Database connection settings (host, port, database)
  - Daemon ports (HTTP and gRPC)
  - Observability settings (log level)

Passwords and sensitive values are masked for security.`,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient := getAPIClient()
		ctx := context.Background()

		resp, err := apiClient.GetConfig(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		// Display configuration with formatted output
		fmt.Println("Stratavore Configuration")
		fmt.Println("════════════════════════")
		fmt.Println("Database:")
		fmt.Printf("  Host: %s\n", resp.Database.Host)
		fmt.Printf("  Port: %d\n", resp.Database.Port)
		fmt.Printf("  Database: %s\n", resp.Database.Database)
		fmt.Println()
		fmt.Println("Daemon:")
		fmt.Printf("  HTTP Port: %d\n", resp.Daemon.HTTPPort)
		fmt.Printf("  gRPC Port: %d\n", resp.Daemon.GRPCPort)
		fmt.Println()
		fmt.Println("Observability:")
		fmt.Printf("  Log Level: %s\n", resp.Observability.LogLevel)
	},
}

var attachCmd = &cobra.Command{
	Use:   "attach <runner-id>",
	Short: "Attach to running instance via tmux",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runnerID := args[0]
		apiClient := getAPIClient()
		ctx := context.Background()

		// Get runner details
		resp, err := apiClient.GetRunner(ctx, runnerID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting runner: %v\n", err)
			os.Exit(1)
		}

		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		if resp.Runner == nil {
			fmt.Fprintf(os.Stderr, "Runner not found: %s\n", runnerID)
			os.Exit(1)
		}

		// Extract tmux session from environment
		tmuxSession, ok := resp.Runner.Environment["tmux_session"]
		if !ok || tmuxSession == "" {
			fmt.Fprintf(os.Stderr, "Runner %s does not have a tmux session configured\n", runnerID)
			fmt.Fprintf(os.Stderr, "Environment: %v\n", resp.Runner.Environment)
			os.Exit(1)
		}

		// Attach to tmux session
		fmt.Printf("Attaching to tmux session: %s\n", tmuxSession)
		attachCmd := exec.Command("tmux", "attach", "-t", tmuxSession)
		attachCmd.Stdin = os.Stdin
		attachCmd.Stdout = os.Stdout
		attachCmd.Stderr = os.Stderr

		if err := attachCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error attaching to tmux: %v\n", err)
			os.Exit(1)
		}
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

var projectsDeleteCmd = &cobra.Command{
	Use:   "delete <project-name>",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectName := args[0]

		// Confirmation prompt
		prompt := promptui.Prompt{
			Label:     fmt.Sprintf("Delete project '%s'? This cannot be undone", projectName),
			IsConfirm: true,
		}

		result, err := prompt.Run()
		if err != nil || result != "y" {
			fmt.Println("Deletion cancelled")
			return
		}

		apiClient := getAPIClient()
		ctx := context.Background()

		resp, err := apiClient.DeleteProject(ctx, projectName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		if resp.Success {
			fmt.Printf("Project '%s' deleted successfully\n", projectName)
		} else {
			fmt.Fprintf(os.Stderr, "Failed to delete project\n")
			os.Exit(1)
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

var tokensCmd = &cobra.Command{
	Use:   "tokens",
	Short: "Display token usage metrics",
	Long: `Show token usage across all projects.

Displays total tokens used, daily limit, and per-project breakdown.`,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient := getAPIClient()
		ctx := context.Background()

		// Check if daemon is running
		if err := apiClient.Ping(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Daemon not running. Start with: stratavored\n")
			os.Exit(1)
		}

		resp, err := apiClient.GetTokens(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		// Display token usage
		fmt.Println("Token Usage")
		fmt.Println("═══════════")

		fmt.Printf("Total Used: %s\n", formatNumber(resp.TotalTokensUsed))
		fmt.Printf("Daily Limit: %s\n", formatNumber(resp.DailyLimit))

		// Display usage percentage
		usageStatus := "normal"
		if resp.UsagePercentage > 100 {
			usageStatus = "over limit"
		}
		fmt.Printf("Usage: %.1f%% (%s)\n", resp.UsagePercentage, usageStatus)

		if len(resp.TokensByProject) > 0 {
			fmt.Println("\nBy Project:")
			// Sort projects by name for consistent output
			var projects []string
			for p := range resp.TokensByProject {
				projects = append(projects, p)
			}
			sort.Strings(projects)

			for _, p := range projects {
				fmt.Printf("  %-20s %s\n", truncate(p, 20), formatNumber(resp.TokensByProject[p]))
			}
		}
	},
}

var daemonCmd = &cobra.Command{
	Use:   "daemon <start|stop|status|attach>",
	Short: "Manage the stratavored daemon",
	Long: `Control the stratavore daemon lifecycle.

  start   Launch stratavored. Wraps in a tmux session when tmux is available,
          recording the session name and launch flags so the daemon can be
          resumed after a disconnect.

  stop    Send SIGTERM to the running daemon.

  status  Show whether the daemon is running and how to reach it.

  attach  Re-attach to the daemon's tmux session (same as 'stratavore resume').`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "start":
			return daemonStart(cmd)
		case "stop":
			return daemonStop()
		case "status":
			return daemonStatus()
		case "attach":
			return daemonAttach()
		default:
			return fmt.Errorf("unknown action %q — use start, stop, status, or attach", args[0])
		}
	},
}

// daemonStart launches stratavored, optionally inside a tmux session.
func daemonStart(cmd *cobra.Command) error {
	// Check for an existing live session first.
	existing, err := session.LoadDaemonSession()
	if err == nil && existing != nil && existing.Alive() {
		fmt.Println("Daemon is already running.")
		if existing.SessionName != "" {
			fmt.Printf("  Session: %s\n", existing.SessionName)
			fmt.Printf("  Attach:  stratavore resume\n")
		} else {
			fmt.Printf("  PID: %d\n", session.ReadPIDFile())
		}
		return nil
	}

	// Collect the flags that were passed so we can replay them on resume.
	var launchFlags []string
	if godMode {
		launchFlags = append(launchFlags, "--god")
	}
	if preset != "" {
		launchFlags = append(launchFlags, "--preset", preset)
	}
	if configFile != "" {
		launchFlags = append(launchFlags, "--config", configFile)
	}

	// Resolve stratavored binary (same dir as this binary, fallback to PATH).
	daemonBin, err := resolveDaemonPath()
	if err != nil {
		return fmt.Errorf("cannot find stratavored binary: %w", err)
	}

	rec := &session.DaemonSession{
		StartedAt:  time.Now(),
		Flags:      launchFlags,
		ConfigFile: configFile,
	}

	if session.TmuxAvailable() {
		rec.SessionName = session.DaemonSessionName

		// Build: tmux new-session -d -s stratavore-daemon -- stratavored [flags...]
		tmuxArgs := []string{"new-session", "-d", "-s", rec.SessionName, "--", daemonBin}
		// Pass through config/preset flags to stratavored if applicable.
		if configFile != "" {
			tmuxArgs = append(tmuxArgs, "--config", configFile)
		}

		launchCmd := exec.Command("tmux", tmuxArgs...)
		launchCmd.Stdout = os.Stdout
		launchCmd.Stderr = os.Stderr
		if err := launchCmd.Run(); err != nil {
			return fmt.Errorf("failed to launch daemon in tmux: %w", err)
		}

		if err := rec.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save session record: %v\n", err)
		}

		fmt.Printf("Daemon started in tmux session %q.\n", rec.SessionName)
		fmt.Printf("  Flags:  %s\n", flagSummary(launchFlags))
		fmt.Printf("  Attach: stratavore resume\n")
		return nil
	}

	// No tmux — launch stratavored directly in the background.
	daemonArgs := []string{}
	if configFile != "" {
		daemonArgs = append(daemonArgs, "--config", configFile)
	}
	proc := exec.Command(daemonBin, daemonArgs...)
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	if err := proc.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	rec.PID = proc.Process.Pid
	if err := rec.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save session record: %v\n", err)
	}

	fmt.Printf("Daemon started (PID %d). No tmux available — session will not survive disconnect.\n", proc.Process.Pid)
	fmt.Println("Install tmux for persistent session support.")
	return nil
}

// daemonStop sends SIGTERM to the running daemon.
func daemonStop() error {
	rec, err := session.LoadDaemonSession()
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if rec == nil {
		fmt.Println("No session record found. Daemon may not be running.")
		return nil
	}
	if !rec.Alive() {
		fmt.Println("Daemon is not running.")
		session.DeleteDaemonSession()
		return nil
	}

	// Kill tmux session — this sends SIGHUP to stratavored, triggering graceful shutdown.
	if rec.SessionName != "" && session.TmuxSessionAlive(rec.SessionName) {
		if err := exec.Command("tmux", "kill-session", "-t", rec.SessionName).Run(); err != nil {
			return fmt.Errorf("kill tmux session: %w", err)
		}
		fmt.Printf("Daemon stopped (tmux session %q terminated).\n", rec.SessionName)
		return nil
	}

	// No tmux — signal by PID.
	pid := rec.PID
	if pid == 0 {
		pid = session.ReadPIDFile()
	}
	if pid == 0 {
		return fmt.Errorf("no PID available to stop daemon")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal daemon: %w", err)
	}
	fmt.Printf("SIGTERM sent to daemon (PID %d).\n", pid)
	return nil
}

// daemonStatus prints the current daemon state.
func daemonStatus() error {
	rec, err := session.LoadDaemonSession()
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	if rec == nil || !rec.Alive() {
		fmt.Println("Daemon: not running")
		if rec != nil {
			fmt.Println("  (stale session record found — daemon has exited)")
		}
		return nil
	}

	fmt.Println("Daemon: running")
	fmt.Printf("  Started:  %s\n", rec.StartedAt.Format(time.RFC3339))
	if rec.SessionName != "" {
		fmt.Printf("  Session:  %s\n", rec.SessionName)
		fmt.Printf("  Attach:   stratavore resume\n")
	} else {
		pid := rec.PID
		if pid == 0 {
			pid = session.ReadPIDFile()
		}
		fmt.Printf("  PID:      %d\n", pid)
	}
	if len(rec.Flags) > 0 {
		fmt.Printf("  Flags:    %s\n", strings.Join(rec.Flags, " "))
	}

	// Also ping the API if daemon is available.
	apiClient := getAPIClient()
	if err := apiClient.Ping(context.Background()); err == nil {
		fmt.Println("  API:      reachable")
	} else {
		fmt.Println("  API:      unreachable (daemon may be starting)")
	}

	return nil
}

// daemonAttach re-attaches to the daemon's tmux session.
func daemonAttach() error {
	rec, err := session.LoadDaemonSession()
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if rec == nil {
		return fmt.Errorf("no session record found — start the daemon with 'stratavore daemon start'")
	}
	if !rec.Alive() {
		fmt.Println("Daemon session is no longer alive.")
		if len(rec.Flags) > 0 {
			fmt.Printf("Relaunch with original flags: stratavore daemon start %s\n", strings.Join(rec.Flags, " "))
		} else {
			fmt.Println("Relaunch with: stratavore daemon start")
		}
		return nil
	}
	return rec.Attach()
}

// resumeCmd supports two modes:
// 1. No args: resume daemon tmux session (existing behavior)
// 2. With project name: resume a conversation session with interactive picker (Task 28)
var resumeCmd = &cobra.Command{
	Use:   "resume [project]",
	Short: "Resume daemon session or resume a project conversation",
	Long: `Resume operations in two ways:

With no arguments:
  Re-attach to the running stratavored daemon's tmux session.
  If the daemon is not running, prints the command to relaunch it.

With a project name (Task 28 - Resume picker):
  List resumable sessions for the project and show an interactive picker.
  Displays session ID, start time, tokens used, and summary.
  Launches runner with conversation_mode="resume" for selected session.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no args, resume daemon (original behavior)
		if len(args) == 0 {
			return daemonAttach()
		}

		// With project arg: resume a conversation session with picker
		projectName := args[0]
		return resumeConversationSession(projectName)
	},
}

// resumeConversationSession lists resumable sessions for a project and shows an interactive picker
// Implements Task 28 - Resume picker CLI enhancement
func resumeConversationSession(projectName string) error {
	apiClient := getAPIClient()
	ctx := context.Background()

	// List sessions for the project
	resp, err := apiClient.ListSessions(ctx, projectName)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if resp.Error != "" {
		return fmt.Errorf("error: %s", resp.Error)
	}

	// Filter for resumable sessions
	var resumableSessions []*types.Session
	for _, session := range resp.Sessions {
		if session.Resumable {
			resumableSessions = append(resumableSessions, session)
		}
	}

	// Handle different cases: 0, 1, or >1 sessions
	if len(resumableSessions) == 0 {
		fmt.Printf("No resumable sessions found for project '%s'\n", projectName)
		return nil
	}

	var selectedSession *types.Session

	if len(resumableSessions) == 1 {
		// Single session: resume directly without picker
		selectedSession = resumableSessions[0]
		fmt.Printf("Found 1 resumable session for project '%s'\n", projectName)
	} else {
		// Multiple sessions: show interactive promptui picker
		fmt.Printf("\nFound %d resumable sessions for project '%s':\n", len(resumableSessions), projectName)

		sessionLabels := make([]string, len(resumableSessions))
		for i, s := range resumableSessions {
			uptime := time.Since(s.StartedAt).Round(time.Second)
			summary := s.Summary
			if summary == "" {
				summary = "(no summary)"
			}
			// Format: ID (started X ago, tokens: Y, messages: Z) - summary
			label := fmt.Sprintf("%s (started %v ago, tokens: %d, messages: %d) - %s",
				truncate(s.ID, 20),
				formatDuration(uptime),
				s.TokensUsed,
				s.MessageCount,
				truncate(summary, 40))
			sessionLabels[i] = label
		}

		prompt := promptui.Select{
			Label: "Select session to resume",
			Items: sessionLabels,
			Size:  10,
		}

		idx, _, err := prompt.Run()
		if err != nil {
			return err
		}

		selectedSession = resumableSessions[idx]
	}

	// Launch runner with resume mode and selected session
	launchResumeRunner(ctx, projectName, selectedSession.ID)
	return nil
}

// launchResumeRunner launches a runner to resume a conversation session
func launchResumeRunner(ctx context.Context, projectName string, sessionID string) {
	apiClient := getAPIClient()

	req := &api.LaunchRunnerRequest{
		ProjectName:      projectName,
		ProjectPath:      "",
		ConversationMode: "resume",
		SessionID:        sessionID,
		RuntimeType:      "process",
	}

	fmt.Printf("Launching runner to resume session %s...\n", truncate(sessionID, 20))

	resp, err := apiClient.LaunchRunner(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
		os.Exit(1)
	}

	fmt.Printf("Runner started: %s\n", resp.Runner.ID)
	fmt.Printf("  Status: %s\n", resp.Runner.Status)
	fmt.Printf("  Project: %s\n", resp.Runner.ProjectName)
	fmt.Printf("  Session: %s\n", resp.Runner.SessionID)
	fmt.Printf("\nUse 'stratavore watch %s' to monitor\n", projectName)
}

// continueCmd resumes the most recent session for a project
var continueCmd = &cobra.Command{
	Use:   "continue <project-name>",
	Short: "Resume most recent session for a project",
	Long: `Resume the most recent session for the specified project.

Queries the API for resumable sessions and launches the most recent one
in conversation_mode="resume" to pick up where you left off.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		apiClient := getAPIClient()
		ctx := context.Background()

		projectName := args[0]

		// Check if daemon is running
		if err := apiClient.Ping(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Daemon not running. Start with: stratavored\n")
			os.Exit(1)
		}

		// Get list of sessions for the project
		sessionsResp, err := apiClient.ListSessions(ctx, projectName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing sessions: %v\n", err)
			os.Exit(1)
		}

		if sessionsResp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", sessionsResp.Error)
			os.Exit(1)
		}

		if len(sessionsResp.Sessions) == 0 {
			fmt.Fprintf(os.Stderr, "No resumable sessions found for project '%s'\n", projectName)
			os.Exit(1)
		}

		// Get the most recent session (first in list)
		session := sessionsResp.Sessions[0]

		// Launch runner with resume mode for this session
		req := &api.LaunchRunnerRequest{
			ProjectName:      projectName,
			ProjectPath:      "", // Will be looked up from project
			ConversationMode: "resume",
			SessionID:        session.ID,
			RuntimeType:      "process",
		}

		fmt.Printf("Resuming most recent session for project '%s'...\n", projectName)
		fmt.Printf("  Session ID: %s\n", session.ID)
		fmt.Printf("  Messages: %d\n", session.MessageCount)
		fmt.Printf("  Last message: %v\n", session.LastMessageAt)

		runnerResp, err := apiClient.LaunchRunner(ctx, req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if runnerResp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", runnerResp.Error)
			os.Exit(1)
		}

		fmt.Printf("✓ Runner started: %s\n", runnerResp.Runner.ID)
		fmt.Printf("  Status: %s\n", runnerResp.Runner.Status)
		fmt.Printf("  Mode: resume\n")
		fmt.Printf("\nUse 'stratavore watch %s' to monitor\n", projectName)
	},
}

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Display daemon and resource state",
	Long: `Display the current operational state of the Stratavore daemon.

Shows daemon status, operational mode, resource counts, and uptime.`,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient := getAPIClient()
		ctx := context.Background()

		// Check if daemon is running
		if err := apiClient.Ping(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Daemon not running. Start with: stratavored\n")
			os.Exit(1)
		}

		// Get state
		resp, err := apiClient.GetState(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		// Display in formatted table
		fmt.Println()
		fmt.Println("Stratavore State")
		fmt.Println("════════════════════════════════════════════")
		fmt.Printf("Operational Mode: %s\n", resp.OperationalMode)
		fmt.Printf("Daemon Status:    %s\n", resp.DaemonStatus)
		fmt.Printf("Uptime:           %s\n", resp.Uptime)
		fmt.Println()
		fmt.Println("Resources:")
		fmt.Printf("  Active Runners:  %d\n", resp.ActiveRunners)
		fmt.Printf("  Total Projects:  %d\n", resp.TotalProjects)
		fmt.Printf("  Total Sessions:  %d\n", resp.TotalSessions)
		fmt.Printf("  Tokens Used:     %s\n", formatNumber(resp.TokensUsed))
		fmt.Println()
	},
}

// resolveDaemonPath returns the path to the stratavored binary.
func resolveDaemonPath() (string, error) {
	// Try same directory as this binary first.
	exePath, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exePath), "stratavored")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	// Fallback to PATH.
	return exec.LookPath("stratavored")
}

// flagSummary formats a flag list for display, or "(none)" if empty.
func flagSummary(flags []string) string {
	if len(flags) == 0 {
		return "(none)"
	}
	return strings.Join(flags, " ")
}
