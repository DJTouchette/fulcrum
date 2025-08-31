package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// generateActionCmd generates a new action in a domain
var generateActionCmd = &cobra.Command{
	Use:   "action [domain] [action]",
	Short: "Generate a new action in a domain",
	Long: `Generate a new action in a domain.

Usage:
  fulcrum generate action users index

This will create a new directory under 'domains/users/' with the specified name and populate it with the basic action structure.`,
	Args: cobra.ExactArgs(2),
	Run:  runGenerateAction,
}



func runGenerateAction(cmd *cobra.Command, args []string) {
	domainName := args[0]
	actionName := args[1]

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// Create the action directory
	actionPath := filepath.Join(cwd, "domains", domainName, actionName)
	if err := os.MkdirAll(actionPath, 0755); err != nil {
		log.Fatalf("Failed to create action directory: %v", err)
	}

	// Create placeholder files
	getHtmlHbsPath := filepath.Join(actionPath, "get.html.hbs")
	getSqlHbsPath := filepath.Join(actionPath, "get.sql.hbs")

	if actionName == "create" || actionName == "update" {
		getHtmlHbsPath = filepath.Join(actionPath, "post.html.hbs")
		getSqlHbsPath = filepath.Join(actionPath, "post.sql.hbs")
	}

	if err := os.WriteFile(getHtmlHbsPath, []byte(""), 0644); err != nil {
		log.Fatalf("Failed to create html.hbs file: %v", err)
	}
	if err := os.WriteFile(getSqlHbsPath, []byte(""), 0644); err != nil {
		log.Fatalf("Failed to create sql.hbs file: %v", err)
	}

	fmt.Printf("✅ Created action: %s in domain: %s\n", actionName, domainName)
}
