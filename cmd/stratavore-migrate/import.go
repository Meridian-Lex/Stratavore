package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import V2 data into Stratavore V3",
	Long:  "TODO: Full import implementation (Task 13)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("import command not yet implemented (Task 13)")
	},
}
