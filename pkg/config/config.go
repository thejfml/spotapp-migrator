package config

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	SpotifyClientID     string
	SpotifyClientSecret string
	SpotifyRedirectURI string
	AppleMusicUserToken string
	AppleStorefront     string
	AppleDeveloperJWT   string
}

// LoadConfig reads from .env file if it exists, falling back to OS environment variables.
func LoadConfig(envPath string) (*Config, error) {
	if envPath == "" {
		envPath = ".env"
	}
	loadEnvFile(envPath)

	cfg := &Config{
		SpotifyClientID:     getEnv("SPOTIFY_CLIENT_ID", ""),
		SpotifyClientSecret: getEnv("SPOTIFY_CLIENT_SECRET", ""),
		SpotifyRedirectURI: getEnv("SPOTIFY_REDIRECT_URI", "http://localhost:8080/callback"),
		AppleMusicUserToken: getEnv("APPLE_MUSIC_USER_TOKEN", ""),
		AppleStorefront:     getEnv("APPLE_STOREFRONT", "us"),
		AppleDeveloperJWT:   getEnv("APPLE_DEVELOPER_JWT", ""),
	}

	return cfg, nil
}

func loadEnvFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return // .env file optional
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"'`)
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
