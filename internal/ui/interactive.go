package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/meridian-lex/stratavore/pkg/api"
	"github.com/meridian-lex/stratavore/pkg/types"
)

// Style definitions using lipgloss
var (
	activeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))  // Cyan
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))  // Green
	labelStyle    = lipgloss.NewStyle().Bold(true)
)

// MenuSelector creates a generic menu selector using huh.
// Returns the selected index and any error that occurred.
func MenuSelector(label string, items []string, size int) (int, string, error) {
	if len(items) == 0 {
		return -1, "", fmt.Errorf("no items provided")
	}

	var selected string
	options := make([]huh.Option[string], len(items))
	for i, item := range items {
		options[i] = huh.NewOption(item, item)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(label).
				Options(options...).
				Value(&selected).
				Height(size),
		),
	)

	err := form.Run()
	if err != nil {
		return -1, "", err
	}

	// Find the index of the selected item
	selectedIdx := -1
	for i, item := range items {
		if item == selected {
			selectedIdx = i
			break
		}
	}

	return selectedIdx, selected, nil
}

// ProjectSelector creates an interactive project picker.
// Returns the selected project index and any error.
// Accepts api.Project to match API response types.
func ProjectSelector(projects []*api.Project) (int, error) {
	if len(projects) == 0 {
		return -1, fmt.Errorf("no projects available")
	}

	projectLabels := make([]string, len(projects))
	for i, p := range projects {
		projectLabels[i] = fmt.Sprintf("%-20s [%s, %d runners]",
			uiTruncate(p.Name, 20), p.Status, p.ActiveRunners)
	}

	var selected string
	options := make([]huh.Option[string], len(projectLabels))
	for i, label := range projectLabels {
		options[i] = huh.NewOption(label, label)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select project to launch").
				Options(options...).
				Value(&selected).
				Height(10),
		),
	)

	err := form.Run()
	if err != nil {
		return -1, err
	}

	// Find the index
	for i, label := range projectLabels {
		if label == selected {
			return i, nil
		}
	}

	return -1, fmt.Errorf("selection not found")
}

// RunnerActionSelector shows a choice between attaching to an existing runner or launching a new one.
// Returns 0 for attach, 1 for new, or error.
func RunnerActionSelector() (int, error) {
	options := []string{
		"Attach to existing runner",
		"Launch new runner",
	}

	var selected string
	huhOptions := make([]huh.Option[string], len(options))
	for i, opt := range options {
		huhOptions[i] = huh.NewOption(opt, opt)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Choose action").
				Options(huhOptions...).
				Value(&selected).
				Height(5),
		),
	)

	err := form.Run()
	if err != nil {
		return -1, err
	}

	// Return index based on selection
	for i, opt := range options {
		if opt == selected {
			return i, nil
		}
	}

	return -1, fmt.Errorf("selection not found")
}

// RunnerSelector creates an interactive runner picker with option to launch new.
// Returns the selected runner index (or len(runners) for "launch new") and any error.
func RunnerSelector(runners []*types.Runner, projectName string) (int, error) {
	if len(runners) == 0 {
		return -1, fmt.Errorf("no runners available")
	}

	runnerLabels := make([]string, len(runners)+1)
	for i, r := range runners {
		uptime := time.Since(r.StartedAt).Round(time.Second)
		runnerLabels[i] = fmt.Sprintf("%s (started %v ago)",
			uiTruncate(r.ID, 30), uptime)
	}
	runnerLabels[len(runners)] = "Launch new runner"

	var selected string
	options := make([]huh.Option[string], len(runnerLabels))
	for i, label := range runnerLabels {
		options[i] = huh.NewOption(label, label)
	}

	title := fmt.Sprintf("Select runner to attach or launch new (%s)", projectName)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(options...).
				Value(&selected).
				Height(10),
		),
	)

	err := form.Run()
	if err != nil {
		return -1, err
	}

	// Find the index
	for i, label := range runnerLabels {
		if label == selected {
			return i, nil
		}
	}

	return -1, fmt.Errorf("selection not found")
}

// ConfirmationPrompt creates a yes/no confirmation prompt.
// Returns true if confirmed (y), false otherwise.
func ConfirmationPrompt(label string) (bool, error) {
	var confirmed bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(label).
				Value(&confirmed),
		),
	)

	err := form.Run()
	if err != nil {
		return false, err
	}

	return confirmed, nil
}

// SessionSelector creates an interactive session picker for resumption.
// Returns the selected session index and any error.
func SessionSelector(sessions []*types.Session) (int, error) {
	if len(sessions) == 0 {
		return -1, fmt.Errorf("no sessions available")
	}

	sessionLabels := make([]string, len(sessions))
	for i, s := range sessions {
		uptime := time.Since(s.CreatedAt)
		summary := s.Summary
		if summary == "" {
			summary = "(no summary)"
		}

		label := fmt.Sprintf("%-20s | %8s | Tokens: %5d | Msgs: %3d | %s",
			uiTruncate(s.ID, 20),
			uiFormatDuration(uptime),
			s.TokensUsed,
			s.MessageCount,
			uiTruncate(summary, 40))
		sessionLabels[i] = label
	}

	var selected string
	options := make([]huh.Option[string], len(sessionLabels))
	for i, label := range sessionLabels {
		options[i] = huh.NewOption(label, label)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select session to resume").
				Options(options...).
				Value(&selected).
				Height(10),
		),
	)

	err := form.Run()
	if err != nil {
		return -1, err
	}

	// Find the index
	for i, label := range sessionLabels {
		if label == selected {
			return i, nil
		}
	}

	return -1, fmt.Errorf("selection not found")
}

// MainMenuSelector creates the main interactive launcher menu.
// Returns the selected menu item string and any error.
func MainMenuSelector() (string, error) {
	menuItems := []string{
		"── Projects ──",
		"Launch Project",
		"New Project",
		"List Projects",
		"Delete Project (not implemented)",
		"",
		"── Runners ──",
		"List Runners",
		"Attach to Runner (not implemented)",
		"Kill Runner (not implemented)",
		"",
		"── Sessions ──",
		"Resume Session",
		"List Sessions (not implemented)",
		"Export Session (not implemented)",
		"",
		"── Configuration ──",
		"Show Config (not implemented)",
		"Operational Mode (not implemented)",
		"Token Budget (not implemented)",
		"",
		"Exit",
	}

	// Filter out separators and empty lines for actual selectable options
	// We'll use a different approach: include all items but handle separators after selection
	var selected string
	options := make([]huh.Option[string], len(menuItems))
	for i, item := range menuItems {
		options[i] = huh.NewOption(item, item)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Choose action").
				Options(options...).
				Value(&selected).
				Height(20),
		),
	)

	err := form.Run()
	if err != nil {
		return "", err
	}

	return selected, nil
}

// Helper functions

// uiTruncate truncates a string to a maximum length, adding "..." if truncated.
// Renamed to avoid collision with monitor.go's truncate function.
func uiTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// uiFormatDuration formats a duration into a human-readable string.
// Renamed to avoid collision with monitor.go's formatDuration function.
func uiFormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// IsSeparatorOrEmpty checks if a menu item is a separator or empty line.
func IsSeparatorOrEmpty(item string) bool {
	return item == "" || strings.HasPrefix(item, "──")
}
