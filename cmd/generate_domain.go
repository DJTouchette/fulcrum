package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var domainPath string

// generateDomainCmd generates a new domain
var generateDomainCmd = &cobra.Command{
	Use:   "domain [name] [field:type...]",
	Short: "Generate a new domain",
	Long: `Generate a new domain with the basic CRUD structure and optional fields.

Usage:
  fulcrum generate domain users name:string email:string

This will create a new directory under 'domains/' with the specified name and populate it with the basic CRUD structure and fields.`,
	Args: cobra.MinimumNArgs(1),
	Run:  runGenerateDomain,
}

func init() {
	generateCmd.AddCommand(generateDomainCmd)
	generateDomainCmd.Flags().StringVar(&domainPath, "path", "", "Path to generate the domain in")
}

func pluralize(s string) string {
	if strings.HasSuffix(s, "y") {
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}

func titleize(s string) string {
	return strings.Title(s)
}

type Field struct {
	Name string
	Type string
}

func runGenerateDomain(cmd *cobra.Command, args []string) {
	domainName := args[0]
	var fields []Field

	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, ":", 2)
		if len(parts) != 2 {
			log.Fatalf("Invalid field format: %s. Expected format: name:type", arg)
		}
		fields = append(fields, Field{Name: parts[0], Type: parts[1]})
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// Use the provided path or the current working directory
	basePath := cwd
	if domainPath != "" {
		basePath = domainPath
	}

	// Create the domain directory
	domainAbsPath := filepath.Join(basePath, "domains", domainName)
	if err := os.MkdirAll(domainAbsPath, 0755); err != nil {
		log.Fatalf("Failed to create domain directory: %v", err)
	}

	// Create the fulcrum.yml file
	fulcrumYmlPath := filepath.Join(domainAbsPath, "fulcrum.yml")
	if err := os.WriteFile(fulcrumYmlPath, []byte("# Domain configuration for "+domainName), 0644); err != nil {
		log.Fatalf("Failed to create fulcrum.yml: %v", err)
	}

	// Generate migration
	migrationsDir := filepath.Join(domainAbsPath, "migrations")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		log.Fatalf("Failed to create migrations directory: %v", err)
	}

	// Get next version number (simplified for now)
	nextVersion := 1
	// TODO: Implement proper versioning based on existing migrations

	migrationFileName := fmt.Sprintf("%03d_create_%s_table.yml", nextVersion, pluralize(domainName))
	migrationFilePath := filepath.Join(migrationsDir, migrationFileName)
	migrationContent := generateMigrationContent(domainName, fields)
	if err := os.WriteFile(migrationFilePath, []byte(migrationContent), 0644); err != nil {
		log.Fatalf("Failed to write migration file: %v", err)
	}
	fmt.Printf("✅ Created migration: %s\n", migrationFilePath)

	// Create the action directories and files
	actions := map[string]string{
		"index":  "get",
		"new":    "get",
		"create": "post",
		"show":   "get",
		"edit":   "get",
		"update": "post",
	}

	for action, method := range actions {
		var actionPath string
		var htmlTemplateFileName string
		var sqlTemplateFileName string
		var redirectTemplateFileName string

		if action == "show" || action == "edit" || action == "update" {
			actionPath = filepath.Join(domainAbsPath, fmt.Sprintf("[%s_id]", domainName), action)
		} else {
			actionPath = filepath.Join(domainAbsPath, action)
		}

		if err := os.MkdirAll(actionPath, 0755); err != nil {
			log.Fatalf("Failed to create action directory: %v", err)
		}

		htmlTemplateFileName = fmt.Sprintf("%s.html.hbs", action)
		sqlTemplateFileName = fmt.Sprintf("%s.sql.hbs", action)
		redirectTemplateFileName = "redirect.yaml.hbs"

		htmlHbsPath := filepath.Join(actionPath, fmt.Sprintf("%s.html.hbs", method))
		sqlHbsPath := filepath.Join(actionPath, fmt.Sprintf("%s.sql.hbs", method))
		redirectYamlPath := filepath.Join(actionPath, "redirect.yaml")

		// Read HTML template content
		htmlContent, err := os.ReadFile(filepath.Join(cwd, "cmd", "templates", htmlTemplateFileName))
		if err != nil {
			log.Fatalf("Failed to read HTML template: %v", err)
		}
		processedHtmlContent := strings.ReplaceAll(string(htmlContent), "{{pluralize .DomainName}}", pluralize(domainName))
		processedHtmlContent = strings.ReplaceAll(processedHtmlContent, "{{titleize .DomainName}}", titleize(domainName))

		// Dynamically generate form fields for new and edit actions
		if action == "new" || action == "edit" {
			formFields := generateFormFields(fields)
			processedHtmlContent = strings.ReplaceAll(processedHtmlContent, "<!-- FORM_FIELDS_PLACEHOLDER -->", formFields)
		}

		// Write HTML file
		if err := os.WriteFile(htmlHbsPath, []byte(processedHtmlContent), 0644); err != nil {
			log.Fatalf("Failed to write HTML file: %v", err)
		}

		// Read SQL template content
		sqlContent, err := os.ReadFile(filepath.Join(cwd, "cmd", "templates", sqlTemplateFileName))
		if err != nil {
			log.Fatalf("Failed to read SQL template: %v", err)
		}
		processedSqlContent := strings.ReplaceAll(string(sqlContent), "{{pluralize .DomainName}}", pluralize(domainName))
		processedSqlContent = strings.ReplaceAll(processedSqlContent, "{{titleize .DomainName}}", titleize(domainName))

		// Dynamically generate SQL columns/values/setters for create and update actions
		if action == "create" {
			columns := generateSqlColumns(fields)
			values := generateSqlValues(fields)
			processedSqlContent = strings.ReplaceAll(processedSqlContent, "{{columns}}", columns)
			processedSqlContent = strings.ReplaceAll(processedSqlContent, "{{values}}", values)
		} else if action == "update" {
			processedSqlContent = strings.ReplaceAll(processedSqlContent, "{{setters}}", generateSqlSetters(fields))
		}

		// Write SQL file
		if err := os.WriteFile(sqlHbsPath, []byte(processedSqlContent), 0644); err != nil {
			log.Fatalf("Failed to write SQL file: %v", err)
		}

		// Execute Redirect YAML template for create action
		if action == "create" {
			redirectContent, err := os.ReadFile(filepath.Join(cwd, "cmd", "templates", redirectTemplateFileName))
			if err != nil {
				log.Fatalf("Failed to read redirect YAML template: %v", err)
			}
			processedRedirectContent := strings.ReplaceAll(string(redirectContent), "{{pluralize .DomainName}}", pluralize(domainName))
			processedRedirectContent = strings.ReplaceAll(processedRedirectContent, "{{id}}", "{{id}}")

			if err := os.WriteFile(redirectYamlPath, []byte(processedRedirectContent), 0644); err != nil {
				log.Fatalf("Failed to write redirect YAML file: %v", err)
			}
		}
	}

	fmt.Printf("✅ Created domain: %s in %s\n", domainName, domainAbsPath)
}

func generateMigrationContent(domainName string, fields []Field) string {
	pluralDomainName := pluralize(domainName)

	columnsYaml := ""
	for _, field := range fields {
		columnType := field.Type
		if field.Type == "string" {
			columnType = "varchar(255)"
		} else if field.Type == "text" {
			columnType = "text"
		} else if field.Type == "integer" {
			columnType = "integer"
		} else if field.Type == "boolean" {
			columnType = "boolean"
		}
		columnsYaml += fmt.Sprintf(`
        - name: %s
          type: %s
          nullable: true`, field.Name, columnType)
	}

	return fmt.Sprintf(`version: 1
name: create_%s_table
description: "Create %s table"

up:
  - create_table:
      name: %s
      columns:
        - name: id
          type: serial
          primary_key: true
        - name: created_at
          type: timestamp
          nullable: false
          default: "NOW()"
        - name: updated_at
          type: timestamp
          nullable: false
          default: "NOW()"%s

down:
  - drop_table:
      name: %s
`, pluralDomainName, pluralDomainName, pluralDomainName, columnsYaml, pluralDomainName)
}

func generateFormFields(fields []Field) string {
	formFieldsHtml := ""
	for _, field := range fields {
		inputTag := ""
		switch field.Type {
		case "string":
			inputTag = fmt.Sprintf(`<input type="text" name="%s" id="%s" class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-300 focus:ring focus:ring-indigo-200 focus:ring-opacity-50">`, field.Name, field.Name)
		case "text":
			inputTag = fmt.Sprintf(`<textarea name="%s" id="%s" rows="3" class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-300 focus:ring focus:ring-indigo-200 focus:ring-opacity-50"></textarea>`, field.Name, field.Name)
		case "integer":
			inputTag = fmt.Sprintf(`<input type="number" name="%s" id="%s" class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-300 focus:ring focus:ring-indigo-200 focus:ring-opacity-50">`, field.Name, field.Name)
		case "boolean":
			inputTag = fmt.Sprintf(`<input type="checkbox" name="%s" id="%s" class="rounded border-gray-300 text-indigo-600 shadow-sm focus:border-indigo-300 focus:ring focus:ring-indigo-200 focus:ring-opacity-50">`, field.Name, field.Name)
		default:
			inputTag = fmt.Sprintf(`<input type="text" name="%s" id="%s" class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-300 focus:ring focus:ring-indigo-200 focus:ring-opacity-50">`, field.Name, field.Name)
		}
		formFieldsHtml += fmt.Sprintf(`
            <div>
                <label for="%s" class="block text-sm font-medium text-gray-700">%s</label>
                %s
            </div>`, field.Name, strings.Title(field.Name), inputTag)
	}
	return formFieldsHtml
}

func generateSqlColumns(fields []Field) string {
	columns := []string{}
	for _, field := range fields {
		columns = append(columns, field.Name)
	}
	return strings.Join(columns, ", ")
}

func generateSqlValues(fields []Field) string {
	values := []string{}
	for _, field := range fields {
		values = append(values, fmt.Sprintf("{{%s}}", field.Name))
	}
	return strings.Join(values, ", ")
}

func generateSqlSetters(fields []Field) string {
	setters := []string{}
	for _, field := range fields {
		setters = append(setters, fmt.Sprintf("%s = {{%s}}", field.Name, field.Name))
	}
	return strings.Join(setters, ", ")
}

