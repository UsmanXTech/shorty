package config

import "os"

type Config struct {
	Address string
	Database string
	GeoIPDB string
	BaseURL string
}

func Load() Config {
	address := os.Getenv("SHORTY_ADDR")
	if address == "" {
		address = ":8080"
	}
	database := os.Getenv("SHORTY_DB")
	if database == "" {
		database = "shorty.db"
	}
	return Config{
		Address: address,
		Database: database,
		GeoIPDB: os.Getenv("SHORTY_GEOIP_DB"),
		BaseURL: os.Getenv("SHORTY_BASE_URL"),
	}
}
