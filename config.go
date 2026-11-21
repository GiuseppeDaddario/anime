package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type userConfig struct {
	UserID    int64  `json:"user_id"`
	Token     string `json:"token,omitempty"`
	APIURL    string `json:"api_url,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func userConfigPath() (string, error) {
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".config")
	}
	return filepath.Join(root, "animewatcher", "config.json"), nil
}

func writeUserConfig(config userConfig) error {
	path, err := userConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	return nil
}

func readUserConfig() (userConfig, error) {
	path, err := userConfigPath()
	if err != nil {
		return userConfig{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return userConfig{}, nil
	}
	if err != nil {
		return userConfig{}, err
	}
	var config userConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("Account settings are unreadable; link your Telegram account again")
	}
	return config, nil
}
