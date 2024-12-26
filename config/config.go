package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	GitHubAPIURL string `yaml:"github_api_url"`
	RepoOwner    string `yaml:"repo_owner"`
	RepoName     string `yaml:"repo_name"`
	GitHubToken  string `yaml:"github_token"`
	BranchPrefix string `yaml:"branch_prefix"`
}

func LoadConfig() (*Config, error) {
	file, err := os.Open("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to open config.yaml: %w", err)
	}
	defer file.Close()

	var config Config

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config.yaml: %w", err)
	}

	return &config, nil
}
