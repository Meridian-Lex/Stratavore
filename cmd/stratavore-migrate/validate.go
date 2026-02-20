package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate migrated data integrity",
	Long:  "TODO: Full validation implementation (Task 14)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("validate command not yet implemented (Task 14)")
	},
}
