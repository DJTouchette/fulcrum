package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// generateProjectCmd generates a new project
var generateProjectCmd = &cobra.Command{
	Use:   "project [name]",
	Short: "Generate a new project from the example",
	Long: `Generate a new project from the example.

Usage:
  fulcrum generate project my-new-app

This will create a new directory with the specified name and populate it with the example project structure.`,
	Args: cobra.ExactArgs(1),
	Run:  runGenerateProject,
}






func runGenerateProject(cmd *cobra.Command, args []string) {
	projectName := args[0]

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// Path to the new project
	newProjectPath := filepath.Join(cwd, projectName)

	// Check if the new project directory already exists
	if _, err := os.Stat(newProjectPath); !os.IsNotExist(err) {
		log.Fatalf("Directory '%s' already exists.", newProjectPath)
	}

	// Create the new project directory
	if err := os.MkdirAll(newProjectPath, 0755); err != nil {
		log.Fatalf("Failed to create project directory: %v", err)
	}

	// Create the basic directory structure
	dirs := []string{
		"domains",
		"shared/views/layouts",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(newProjectPath, dir), 0755); err != nil {
			log.Fatalf("Failed to create directory: %v", err)
		}
	}

	// Create the fulcrum.yml file
	fulcrumYmlPath := filepath.Join(newProjectPath, "fulcrum.yml")
	fulcrumYmlContent := `database:
  driver: postgresql
`
	if err := os.WriteFile(fulcrumYmlPath, []byte(fulcrumYmlContent), 0644); err != nil {
		log.Fatalf("Failed to write fulcrum.yml: %v", err)
	}

	// Create the main.hbs layout
	mainHbsPath := filepath.Join(newProjectPath, "shared", "views", "layouts", "main.hbs")
	mainHbsContent := `<!DOCTYPE html>
<html>
<head>
	<title>{{title}}</title>
</head>
<body>
	{{{body}}}
</body>
</html>`
	if err := os.WriteFile(mainHbsPath, []byte(mainHbsContent), 0644); err != nil {
		log.Fatalf("Failed to write main.hbs: %v", err)
	}

	fmt.Printf("✅ Created project: %s\n", newProjectPath)
	fmt.Printf("✅ Configured database driver: postgresql\n")
	fmt.Printf("✅ Created main.hbs layout\n")
}

