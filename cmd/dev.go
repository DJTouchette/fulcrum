/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	parser "fulcrum/lib/parser"

	"github.com/spf13/cobra"

	adapters "fulcrum/lib/lang/adapters"
)

// devCmd represents the dev command
var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("dev called")
		appConfig, err := parser.GetAppConfig("/home/djtouchette/Documents/fulcrum/fulcrum-example")
		if err != nil {
			fmt.Println("Error getting app config:", err)
		}

		fmt.Println("=== Application Configuration ===")
		appConfig.PrintYAML()
		fmt.Println("================================")

		// Print discovered routes
		fmt.Println("=== Discovered Routes ===")
		for _, domain := range appConfig.Domains {
			fmt.Printf("Domain: %s\n", domain.Name)
			for _, route := range domain.Logic.HTTP.Routes {
				fmt.Printf("  %s %s -> %s (format: %s)\n",
					route.Method, route.Link, route.ViewPath, route.Format)
			}
		}
		fmt.Println("=========================")

		// adapters.StartBothServersWithConfig(&appConfig)
		adapters.StartBothServersWithProcessManager(&appConfig)
	},
}

func init() {
	rootCmd.AddCommand(devCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// devCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// devCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
