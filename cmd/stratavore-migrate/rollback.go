package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	snapshotPath string
)

func init() {
	rollbackCmd.Flags().StringVar(&snapshotPath, "snapshot", "", "Path to pg_dump snapshot file")
}

var rollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback migration using pg_dump snapshot",
	Long:  "TODO: Full rollback implementation (Task 15)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("rollback command not yet implemented (Task 15)")
	},
}
