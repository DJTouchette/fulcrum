package parser

import (
	"fmt"

	views "fulcrum/lib/views"

	"gopkg.in/yaml.v2"
)

type AppConfig struct {
	Domains []DomainConfig
	DB      DBConfig `yaml:"db"`
	Path    string
	Views   *views.TemplateRenderer
}

type DBConfig struct {
	Type string `yaml:"type"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type DomainConfig struct {
	Models   []ModelDefinition `yaml:"models"`
	Logic    LogicConfig       `yaml:"logic"`
	Name     string
	Path     string
	ViewPath string
}

type ModelDefinition map[string]Model

type Model map[string]Field

type Field struct {
	Type        string       `yaml:"type"`
	Validations []Validation `yaml:"validations"`
}

type Validation map[string]any

type LogicConfig struct {
	HTTP HTTPConfig `yaml:"http"`
}

type HTTPConfig struct {
	Restful bool    `yaml:"restful"`
	Routes  []Route `yaml:"routes"`
}

type Route struct {
	Method   string `yaml:"method"`
	Link     string `yaml:"link"`
	View     string `yaml:"view"`
	ViewPath string
}

func (dc *DomainConfig) GetModel(name string) (Model, bool) {
	for _, modelDef := range dc.Models {
		if model, exists := modelDef[name]; exists {
			return model, true
		}
	}
	return nil, false
}

func (m Model) GetField(fieldName string) (Field, bool) {
	field, exists := m[fieldName]
	return field, exists
}

func (f Field) GetValidation(validationType string) (any, bool) {
	for _, validation := range f.Validations {
		if val, exists := validation[validationType]; exists {
			return val, true
		}
	}
	return nil, false
}

func (f Field) IsNullable() bool {
	if nullable, exists := f.GetValidation(Nullable); exists {
		if val, ok := nullable.(bool); ok {
			return val
		}
	}
	return true
}

func (f Field) GetLengthConstraints() (min, max int, hasConstraints bool) {
	if lengthVal, exists := f.GetValidation(ValidateLength); exists {
		if lengthMap, ok := lengthVal.(map[string]any); ok {
			var minVal, maxVal int
			var hasMin, hasMax bool

			if minInterface, exists := lengthMap[ValidateLengthMin]; exists {
				if minInt, ok := minInterface.(int); ok {
					minVal = minInt
					hasMin = true
				}
			}

			if maxInterface, exists := lengthMap[ValidateLengthMax]; exists {
				if maxInt, ok := maxInterface.(int); ok {
					maxVal = maxInt
					hasMax = true
				}
			}

			if hasMin || hasMax {
				return minVal, maxVal, true
			}
		}
	}
	return 0, 0, false
}

func (dc *AppConfig) PrintYAML() {
	yamlData, err := yaml.Marshal(dc)
	if err != nil {
		fmt.Printf("Error marshaling YAML: %v", err)
		return
	}
	fmt.Print(string(yamlData))
}
