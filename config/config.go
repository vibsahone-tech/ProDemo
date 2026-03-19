package config

import (
	"encoding/json"
	"log"
	"os"
)

type Config struct {
	DataStore DataStoreConfig `json:"DataStore"`
	Server    ServerConfig    `json:"Server"`
	Upload    UploadConfig    `json:"Upload"`
}

type DataStoreConfig struct {
	URL                   string `json:"URL"`
	DB                    string `json:"DB"`
	RegisterCollection    string `json:"RegisterCollection"`
	ProtocolCollection    string `json:"ProtocolCollection"`
	TimeoutSec            int    `json:"TimeoutSec"`
	TransactionTimeoutSec int    `json:"TransactionTimeoutSec"`
}

type ServerConfig struct {
	Port            string `json:"Port"`
	MaxUploadSizeMB int    `json:"MaxUploadSizeMB"`
}

type UploadConfig struct {
	MaxRows int `json:"MaxRows"`
}

// Load reads config.json from the given path and returns a parsed Config.
// Calls log.Fatal if the file cannot be read or parsed.
func Load(path string) *Config {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("config: cannot read %s: %v", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("config: cannot parse %s: %v", path, err)
	}
	return &cfg
}
