package parser

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

func FindDomainFiles(root string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && d.Name() == DomainConfigFileName {
			paths = append(paths, path)
		}

		return nil
	})

	return paths, err
}

func getDomainName(path string) string {
	splitPath := strings.Split(path, "/")
	domainName := splitPath[len(splitPath)-2]
	return domainName
}

func GetAppConfig(root string) (AppConfig, error) {
	domainPaths, err := FindDomainFiles(root)
	if err != nil {
		return AppConfig{}, err
	}

	domains := []DomainConfig{}

	for _, domainPath := range domainPaths {
		domainFile, err := os.ReadFile(domainPath)
		if err != nil {
			fmt.Println(err)
			continue
		}

		var domainConfig DomainConfig
		ymlParseErr := yaml.Unmarshal(domainFile, &domainConfig)

		if ymlParseErr != nil {
			fmt.Println(ymlParseErr)
			continue
		}

		domainDir := filepath.Dir(domainPath)
		relativeDomainPath, err := filepath.Rel(root, domainDir)
		if err != nil {
			fmt.Printf("Error getting relative path: %v\n", err)
			continue
		}

		domainConfig.ViewPath = filepath.Join(relativeDomainPath, "views")
		domainConfig.Name = getDomainName(domainPath)
		domainConfig.Path = domainPath

		for i := range domainConfig.Logic.HTTP.Routes {
			route := &domainConfig.Logic.HTTP.Routes[i]
			route.ViewPath = filepath.Join(domainConfig.ViewPath, route.View)
		}

		domains = append(domains, domainConfig)
	}

	mainConfigFile, err := os.ReadFile(root + "/" + DomainConfigFileName)
	if err != nil {
		return AppConfig{}, fmt.Errorf("failed to read main config file: %w", err)
	}

	var appConfig AppConfig

	ymlParseErr := yaml.Unmarshal(mainConfigFile, &appConfig)
	if ymlParseErr != nil {
		return AppConfig{}, fmt.Errorf("failed to parse main config file: %w", ymlParseErr)
	}
	appConfig.Domains = domains
	appConfig.Path = root

	return appConfig, nil
}
