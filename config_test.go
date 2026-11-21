package main

import (
	"os"
	"testing"
)

func isolatedUserConfig(t *testing.T) { t.Helper(); t.Setenv("XDG_CONFIG_HOME", t.TempDir()) }
func TestSessionSettingsArePrivateAndPersistAcrossReads(t *testing.T) {
	isolatedUserConfig(t)
	expected := userConfig{UserID: 12345, Token: "private-session", APIURL: "https://api.example.com", ExpiresAt: "2026-12-09T12:00:00Z"}
	if err := writeUserConfig(expected); err != nil {
		t.Fatal(err)
	}
	actual, err := readUserConfig()
	if err != nil || actual != expected {
		t.Fatal("Session did not persist")
	}
	path, _ := userConfigPath()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("Session must be readable only by its local user")
	}
}
func TestMissingOrMalformedAccountSettings(t *testing.T) {
	isolatedUserConfig(t)
	config, err := readUserConfig()
	if err != nil || config.Token != "" {
		t.Fatal("New installs must have no session")
	}
	writeUserConfig(userConfig{})
	path, _ := userConfigPath()
	os.WriteFile(path, []byte("invalid JSON"), 0600)
	if _, err := readUserConfig(); err == nil {
		t.Fatal("Invalid settings must be rejected")
	}
}
