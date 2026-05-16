package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Region       string `json:"region"`
	InstanceType string `json:"instance_type"`
	SOCKSPort    int    `json:"socks_port"`
}

func Default() Config {
	return Config{
		Region:       "eu-west-2",
		InstanceType: "t4g.nano",
		SOCKSPort:    9000,
	}
}

func Load() (Config, error) {
	path, err := filePath()
	if err != nil {
		return Default(), nil
	}
	return loadFrom(path)
}

func Save(cfg Config) error {
	path, err := filePath()
	if err != nil {
		return err
	}
	return saveTo(cfg, path)
}

func loadFrom(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

func saveTo(cfg Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func filePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "navsat", "config.json"), nil
}
