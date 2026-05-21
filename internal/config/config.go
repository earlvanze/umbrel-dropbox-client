package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Root                string `json:"root"`
	DBPath              string `json:"db_path"`
	RemotePath          string `json:"remote_path"`
	RemoteDelta         bool   `json:"remote_delta"`
	TokenFile           string `json:"token_file"`
	Watch               bool   `json:"watch"`
	WatchDebounceMs     int    `json:"watch_debounce_ms"`
	UploadWorkers       int    `json:"upload_workers"`
	DownloadWorkers     int    `json:"download_workers"`
	ScanIntervalSeconds int    `json:"scan_interval_seconds"`
	DryRun              bool   `json:"dry_run"`
	AllowLive           bool   `json:"allow_live"`
	HealthAddr          string `json:"health_addr"`
}

func Default(root string) Config {
	return Config{Root: root, RemotePath: "", Watch: true, WatchDebounceMs: 1500, UploadWorkers: 4, DownloadWorkers: 4, ScanIntervalSeconds: 300, DryRun: true, HealthAddr: "127.0.0.1:0"}
}
func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}
func Save(path string, c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}
