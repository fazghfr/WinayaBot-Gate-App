package config

import (
	"github.com/joho/godotenv"
	"os"
)

type AppConfig struct {
	Token     string
	ChannelID string
}

func LoadConfig() *AppConfig {
	err := godotenv.Load()
	if err != nil {
		return &AppConfig{
			"",
			"",
		}
	}

	return &AppConfig{
		Token:     os.Getenv("BOT_API_TOKEN"),
		ChannelID: os.Getenv("CHANNEL_ID"),
	}
}
