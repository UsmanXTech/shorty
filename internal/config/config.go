package config

import "os"

type Config struct {
	Address  string
	Database string
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
	return Config{Address: address, Database: database}
}
