package config

import (
	"flag"
	"os"
	"testing"
)

func TestLoad_WithEnvironmentVariables(t *testing.T) {
	// Set environment variables
	os.Setenv("SONARQUBE_URL", "https://sonar.example.com")
	os.Setenv("SONARQUBE_TOKEN", "test-token")
	os.Setenv("EXPORTER_HOST", "127.0.0.1")
	os.Setenv("EXPORTER_PORT", "8080")
	defer func() {
		os.Unsetenv("SONARQUBE_URL")
		os.Unsetenv("SONARQUBE_TOKEN")
		os.Unsetenv("EXPORTER_HOST")
		os.Unsetenv("EXPORTER_PORT")
	}()

	// Create a new flag set for testing
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	cfg, err := LoadWithFlagSet(fs, []string{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.SonarQubeURL != "https://sonar.example.com" {
		t.Errorf("Expected SonarQubeURL to be 'https://sonar.example.com', got: %s", cfg.SonarQubeURL)
	}

	if cfg.SonarQubeToken != "test-token" {
		t.Errorf("Expected SonarQubeToken to be 'test-token', got: %s", cfg.SonarQubeToken)
	}

	if cfg.Host != "127.0.0.1" {
		t.Errorf("Expected Host to be '127.0.0.1', got: %s", cfg.Host)
	}

	if cfg.Port != "8080" {
		t.Errorf("Expected Port to be '8080', got: %s", cfg.Port)
	}
}

func TestLoad_MissingSonarQubeURL(t *testing.T) {
	// Unset all environment variables
	os.Unsetenv("SONARQUBE_URL")
	os.Unsetenv("SONARQUBE_TOKEN")
	defer func() {
		os.Unsetenv("SONARQUBE_URL")
		os.Unsetenv("SONARQUBE_TOKEN")
	}()

	// Create a new flag set for testing
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	_, err := LoadWithFlagSet(fs, []string{})
	if err == nil {
		t.Fatal("Expected error for missing SonarQube URL, got nil")
	}
}

func TestAddress(t *testing.T) {
	cfg := &Config{
		Host: "localhost",
		Port: "9090",
	}

	expected := "localhost:9090"
	if cfg.Address() != expected {
		t.Errorf("Expected address to be '%s', got: %s", expected, cfg.Address())
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "environment variable exists",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "custom",
			expected:     "custom",
		},
		{
			name:         "environment variable does not exist",
			key:          "NON_EXISTENT_VAR",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := getEnv(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected '%s', got: '%s'", tt.expected, result)
			}
		})
	}
}
