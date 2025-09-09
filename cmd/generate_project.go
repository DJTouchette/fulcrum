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
	fulcrumYmlContent := `db:
  driver: postgresql
  host: localhost
  port: 5432
  database: fulcrum_dev
  username: fulcrum
  password: fulcrum_pass
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 10
  conn_max_lifetime_minutes: 5

root: /auth/dashboard
`
	if err := os.WriteFile(fulcrumYmlPath, []byte(fulcrumYmlContent), 0644); err != nil {
		log.Fatalf("Failed to write fulcrum.yml: %v", err)
	}

	// Create the main.hbs layout
	mainHbsPath := filepath.Join(newProjectPath, "shared", "views", "layouts", "main.hbs")
	mainHbsContent := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{#if pageTitle}}{{pageTitle}} - {{/if}}Fulcrum</title>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    <script src="https://cdn.tailwindcss.com"></script>
    {{#if additionalCSS}}{{{additionalCSS}}}{{/if}}
</head>
<body class="min-h-screen bg-gradient-to-br from-purple-50 via-pink-50 to-indigo-50">
    <!-- Header -->
    <header class="bg-white/90 backdrop-blur-sm border-b border-purple-200/50 shadow-lg sticky top-0 z-50">
        <div class="max-w-7xl mx-auto px-6 py-4">
            <div class="flex items-center justify-between">
                <a href="/" class="text-3xl font-bold bg-gradient-to-r from-purple-600 via-pink-600 to-indigo-600 bg-clip-text text-transparent hover:scale-105 transition-transform duration-200">
                    Fulcrum
                </a>
                
                {{#if navigation}}
                <nav class="hidden md:flex space-x-8">
                    {{#each navigation}}
                    <a href="{{url}}" class="text-gray-700 hover:text-purple-600 font-medium transition-colors duration-200 relative group">
                        {{label}}
                        <span class="absolute -bottom-1 left-0 w-0 h-0.5 bg-gradient-to-r from-purple-500 to-pink-500 group-hover:w-full transition-all duration-300"></span>
                    </a>
                    {{/each}}
                </nav>
                
                <!-- Mobile menu button -->
                <button class="md:hidden p-2 rounded-lg hover:bg-purple-100 transition-colors duration-200" onclick="toggleMobileMenu()">
                    <svg class="w-6 h-6 text-gray-700" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
                    </svg>
                </button>
                {{/if}}
            </div>
            
            {{#if navigation}}
            <!-- Mobile menu -->
            <div id="mobileMenu" class="hidden md:hidden mt-4 pb-4 border-t border-purple-200">
                <nav class="flex flex-col space-y-3 pt-4">
                    {{#each navigation}}
                    <a href="{{url}}" class="text-gray-700 hover:text-purple-600 font-medium transition-colors duration-200 py-2">
                        {{label}}
                    </a>
                    {{/each}}
                </nav>
            </div>
            {{/if}}
        </div>
    </header>
    
    <!-- Main Content Container -->
    <div class="flex-1">
        {{#if pageTitle}}
        <div class="max-w-7xl mx-auto px-6 py-8">
            <div class="text-center mb-8">
                <h1 class="text-4xl md:text-5xl font-bold bg-gradient-to-r from-purple-600 via-pink-600 to-indigo-600 bg-clip-text text-transparent mb-4">
                    {{pageTitle}}
                </h1>
                <div class="w-24 h-1 bg-gradient-to-r from-purple-500 via-pink-500 to-indigo-500 rounded-full mx-auto"></div>
            </div>
        </div>
        {{/if}}
        
        <!-- Flash Messages -->
        {{#if flash}}
        <div class="max-w-7xl mx-auto px-6 mb-6">
            {{#if flash.success}}
            <div class="bg-emerald-50/90 backdrop-blur-sm border border-emerald-200 text-emerald-800 px-6 py-4 rounded-xl shadow-lg mb-4">
                <div class="flex items-center">
                    <svg class="w-5 h-5 mr-3 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                    </svg>
                    {{flash.success}}
                </div>
            </div>
            {{/if}}
            {{#if flash.error}}
            <div class="bg-red-50/90 backdrop-blur-sm border border-red-200 text-red-800 px-6 py-4 rounded-xl shadow-lg mb-4">
                <div class="flex items-center">
                    <svg class="w-5 h-5 mr-3 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                    </svg>
                    {{flash.error}}
                </div>
            </div>
            {{/if}}
        </div>
        {{/if}}
        
        <!-- Main Content -->
        <main class="flex-1">
            {{{body}}}
        </main>
    </div>
    
    <!-- Footer -->
    <footer class="bg-white/80 backdrop-blur-sm border-t border-purple-200/50 mt-16">
        <div class="max-w-7xl mx-auto px-6 py-8">
            <div class="text-center">
                <p class="text-gray-600">
                    &copy; {{currentYear}} {{siteName}} &bull; 
                    <span class="bg-gradient-to-r from-purple-600 to-pink-600 bg-clip-text text-transparent font-medium">
                        All rights reserved
                    </span>
                </p>
                <div class="mt-4">
                    <div class="w-16 h-0.5 bg-gradient-to-r from-purple-400 via-pink-400 to-indigo-400 rounded-full mx-auto"></div>
                </div>
            </div>
        </div>
    </footer>
    
    {{#if additionalJS}}{{{additionalJS}}}{{/if}}
    
    <script>
        function toggleMobileMenu() {
            const menu = document.getElementById('mobileMenu');
            menu.classList.toggle('hidden');
        }
        
        // Auto-dismiss flash messages after 5 seconds
        setTimeout(() => {
            const flashMessages = document.querySelectorAll('[class*="bg-emerald-50"], [class*="bg-red-50"]');
            flashMessages.forEach(msg => {
                msg.style.transition = 'opacity 0.5s ease-out';
                msg.style.opacity = '0';
                setTimeout(() => msg.remove(), 500);
            });
        }, 5000);
    </script>
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
		"login/get.html.hbs":                    "domains/auth/login/get.html.hbs",
		"register/get.html.hbs":                 "domains/auth/register/get.html.hbs",
		"dashboard/get.html.hbs":                "domains/auth/dashboard/get.html.hbs",
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
