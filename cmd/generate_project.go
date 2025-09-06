package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

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
		"domains/auth/login",
		"domains/auth/register",
		"domains/auth/dashboard",
		"domains/auth/migrations",
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

	// Create auth domain templates (these can be overridden by users)
	createAuthDomainFiles(newProjectPath)

	fmt.Printf("✅ Created project: %s\n", newProjectPath)
	fmt.Printf("✅ Configured database driver: postgresql\n")
	fmt.Printf("✅ Created main.hbs layout\n")
	fmt.Printf("✅ Created auth domain with login, register, dashboard templates\n")
	fmt.Printf("\n💡 Auth templates can be customized in domains/auth/\n")
	fmt.Printf("💡 Run migrations with: fulcrum migrate up\n")
}

// createAuthDomainFiles creates the auth domain files by copying from lib/views/auth
func createAuthDomainFiles(projectPath string) {
	// Get the path to the fulcrum executable to find lib/views/auth
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatalf("Failed to get runtime caller info")
	}

	// Navigate from cmd/generate_project.go to lib/views/auth
	fulcrumRoot := filepath.Dir(filepath.Dir(filename)) // Go up two levels from cmd/
	libAuthPath := filepath.Join(fulcrumRoot, "lib", "views", "auth")

	// Copy auth templates to project
	authFiles := map[string]string{
		"login/get.html.hbs":              "domains/auth/login/get.html.hbs",
		"register/get.html.hbs":           "domains/auth/register/get.html.hbs",
		"dashboard/get.html.hbs":          "domains/auth/dashboard/get.html.hbs",
		"migrations/001_create_users_table.yml": "domains/auth/migrations/001_create_users_table.yml",
	}

	for srcFile, dstFile := range authFiles {
		srcPath := filepath.Join(libAuthPath, srcFile)
		dstPath := filepath.Join(projectPath, dstFile)

		if err := copyFile(srcPath, dstPath); err != nil {
			log.Printf("Warning: Failed to copy %s: %v", srcFile, err)
			// Don't fail the entire process, just warn
		}
	}
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create destination directory if it doesn't exist
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
