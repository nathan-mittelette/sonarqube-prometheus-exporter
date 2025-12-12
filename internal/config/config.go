package config

import (
	"flag"
	"fmt"
	"os"
)

// Config holds the application configuration
type Config struct {
	// Server configuration
	Host string
	Port string

	// SonarQube configuration
	SonarQubeURL   string
	SonarQubeToken string
}

// Load loads configuration from environment variables and CLI flags
func Load() (*Config, error) {
	return LoadWithFlagSet(flag.CommandLine, os.Args[1:])
}

// LoadWithFlagSet loads configuration with a custom flag set (useful for testing)
func LoadWithFlagSet(fs *flag.FlagSet, args []string) (*Config, error) {
	cfg := &Config{}

	// Define CLI flags
	fs.StringVar(&cfg.Host, "host", getEnv("EXPORTER_HOST", "0.0.0.0"), "Host to bind the exporter server")
	fs.StringVar(&cfg.Port, "port", getEnv("EXPORTER_PORT", "9090"), "Port to bind the exporter server")
	fs.StringVar(&cfg.SonarQubeURL, "sonarqube-url", getEnv("SONARQUBE_URL", ""), "SonarQube server URL")
	fs.StringVar(&cfg.SonarQubeToken, "sonarqube-token", getEnv("SONARQUBE_TOKEN", ""), "SonarQube authentication token")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Validate required fields
	if cfg.SonarQubeURL == "" {
		return nil, fmt.Errorf("sonarqube-url is required (set via flag or SONARQUBE_URL env var)")
	}
	if cfg.SonarQubeToken == "" {
		return nil, fmt.Errorf("sonarqube-token is required (set via flag or SONARQUBE_TOKEN env var)")
	}

	return cfg, nil
}

// getEnv returns the value of an environment variable or a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Address returns the full address (host:port) to bind the server
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}
