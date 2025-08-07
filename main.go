package main

import (
	"Discord_bot_v1/bot"
	"Discord_bot_v1/config"
	"Discord_bot_v1/llm_utils"
	"os"
)

func main() {
	// Load application configurations
	cfg := config.LoadConfig()

	// load llm config
	MyLLM := llm_utils.LLMService{
		APIKey: os.Getenv("GEMINI_CREDS"),
	}

	// Start the bot
	bot.Start(cfg.Token, MyLLM)
}
