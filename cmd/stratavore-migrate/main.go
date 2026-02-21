package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	v2Dir string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "stratavore-migrate",
		Short: "Lex V2 → Stratavore V3 migration tool",
		Long: `Migration tool for importing Lex V2 state into Stratavore V3.

Provides commands to analyze, import, validate, and rollback V2 data migration.`,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&v2Dir, "v2-dir", "", "Path to V2 lex-internal/state directory (required)")

	// Add subcommands
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(rollbackCmd)
	rootCmd.AddCommand(syncCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
