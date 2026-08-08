package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AllowedPaths   []string `yaml:"allowedPaths"`
}

func Load(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config Config

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
