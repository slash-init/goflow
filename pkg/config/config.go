package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	AppPort string
	AppEnv  string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	RedisAddr string
}

func Load() *Config {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv() // reads from real environment variables too

	if err := viper.ReadInConfig(); err != nil {
		// .env file is optional — in Docker we get config from env vars
		log.Println("No .env file found, using environment variables")
	}

	cfg := &Config{
		AppPort:    viper.GetString("APP_PORT"),
		AppEnv:     viper.GetString("APP_ENV"),
		DBHost:     viper.GetString("DB_HOST"),
		DBPort:     viper.GetString("DB_PORT"),
		DBUser:     viper.GetString("DB_USER"),
		DBPassword: viper.GetString("DB_PASSWORD"),
		DBName:     viper.GetString("DB_NAME"),
		RedisAddr:  viper.GetString("REDIS_ADDR"),
	}

	// Fail fast if critical config is missing
	if cfg.AppPort == "" {
		log.Fatal("APP_PORT is required")
	}
	if cfg.DBHost == "" {
		log.Fatal("DB_HOST is required")
	}
	if cfg.RedisAddr == "" {
		log.Fatal("REDIS_ADDR is required")
	}

	return cfg
}
