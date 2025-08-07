package bot

import (
	"Discord_bot_v1/llm_utils"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

var llmService *llm_utils.LLMService

// Start initializes and runs the Discord bot.
func Start(token string, service llm_utils.LLMService) {
	// 1. CREATE DISCORD SESSION
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// setting llm service to facilitate llm operations
	llmService = &service

	// 2. DEFINE INTENTS
	// We need IntentsGuildMessages to receive message events.
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	// 3. ADD EVENT HANDLERS
	// Add a handler for the Ready event, which fires when the bot is connected.
	dg.AddHandler(ready)
	// Add a handler for the MessageCreate event, which fires every time a new message is created.
	// This is how the bot "waits for" and reacts to incoming messages.
	dg.AddHandler(messageCreate)

	// 4. OPEN WEBSOCKET CONNECTION
	err = dg.Open()
	if err != nil {
		log.Fatalf("Error opening connection: %v", err)
	}
	defer dg.Close()

	// 5. WAIT FOR SHUTDOWN SIGNAL
	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	fmt.Println("Shutting down bot...")

}

// ready is called when the bot has successfully connected to Discord.
func ready(s *discordgo.Session, event *discordgo.Ready) {
	fmt.Printf("Logged in as: %v#%v\n", s.State.User.Username, s.State.User.Discriminator)

	// fire up the todoapp instance

	fmt.Println("Bot is ready to receive commands.")

	// ✅ Set a custom status without "Playing"
	s.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: "your commands",
				Type: discordgo.ActivityTypeListening,
			},
		},
		Status: "online",
	})

}

// messageCreate is called every time a new message is created on any channel the bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	// This is to prevent the bot from replying to its own messages.
	if m.Author.ID == s.State.User.ID {
		return
	}

	// If the message content is "!ping", reply with "Pong!"
	if strings.HasPrefix(m.Content, "!ping ") {
		s.ChannelMessageSend(m.ChannelID, "Pong!")

		// DEBUG BLOCK. PLEASE COMMENT THIS LINE ON PRODUCTION
		msgBytes, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			fmt.Println("Error marshaling message:", err)
		} else {
			fmt.Println(string(msgBytes))
		}
		fmt.Printf("Responded to !ping from %s in channel %s\n", m.Author.Username, m.ChannelID)
	}

	// If the message content is "!hello", reply with a greeting.
	if strings.HasPrefix(m.Content, "!hello ") {
		reply := fmt.Sprintf("Hello, %s!", m.Author.Username)
		s.ChannelMessageSend(m.ChannelID, reply)
		fmt.Printf("Responded to !hello from %s in channel %s\n", m.Author.Username, m.ChannelID)
	}

	if strings.HasPrefix(m.Content, "!summarize ") {
		// Get the text after the command by removing the prefix.
		textToSummarize := strings.TrimPrefix(m.Content, "!summarize ")

		// Optional: Check if the user actually provided any text.
		if textToSummarize == "" {
			s.ChannelMessageSend(m.ChannelID, "Please provide some text to summarize after the command.")
			return
		}

		// You now have the text!
		fmt.Printf("User %s wants to summarize: '%s'\n", m.Author.Username, textToSummarize)

		// Here you would add your logic to process the text.
		// For now, let's just send it back.
		reply := fmt.Sprintf("Okay, I will summarize this for you in. Please wait")
		s.ChannelMessageSend(m.ChannelID, reply)
		summary, err := llmService.SummarizeFromText(textToSummarize)
		if err != nil {
			log.Printf("Error getting summary: %v", err)
			// Handle the error, maybe send a message back to Discord
		} else {
			reply := fmt.Sprintf(summary)
			s.ChannelMessageSend(m.ChannelID, reply)
		}
	}

	if strings.HasPrefix(m.Content, "!help") {
		reply := fmt.Sprintf("Hello %s, I am JanBot, currently under development. \n "+
			"available commands \n"+
			"**!ping** -> pinging the bot\n"+
			"**!hello** -> pinging the bot with hello\n"+
			"**!help** -> this\n"+
			"**!summarize** -> summarize a long text given by a user\n"+
			"**!summarize-link** -> summarize from a given link\n"+
			"\n"+
			"\n"+
			"**Todolist Commands **\n"+
			" **under development :warning: **\n "+
			"**!todo-register** -> register for the todo app. **MUST DO** if you want to use the todolist services", m.Author.GlobalName)
		s.ChannelMessageSend(m.ChannelID, reply)
		fmt.Printf("Responded to !help from %s in channel %s\n", m.Author.Username, m.ChannelID)
	}

	if strings.HasPrefix(m.Content, "!summarize-link ") {
		// 1. Get the URL from the message
		url := strings.TrimPrefix(m.Content, "!summarize-link ")
		if url == "" {
			s.ChannelMessageSend(m.ChannelID, "Tolong berikan URL yang valid.")
			return
		}

		s.ChannelMessageSend(m.ChannelID, "Mengakses halaman web... Mohon tunggu.")

		// 2. Call your function to get the webpage content
		pageContent, err := llmService.ReadWebPages(url)
		if err != nil {
			log.Printf("Error reading webpage: %v", err)
			s.ChannelMessageSend(m.ChannelID, "Maaf, gagal mengakses URL tersebut.")
			return
		}

		// This is a good check in case the page was empty
		if pageContent == "" {
			s.ChannelMessageSend(m.ChannelID, "Halaman web tersebut tidak memiliki konten yang bisa dibaca.")
			return
		}

		s.ChannelMessageSend(m.ChannelID, "Halaman berhasil diakses. Sekarang, saya akan meringkas isinya...")

		// 3. Feed the page content into your summarizer
		summary, err := llmService.SummarizeFromText(pageContent)
		if err != nil {
			log.Printf("Error from LLM service on webpage content: %v", err)
			s.ChannelMessageSend(m.ChannelID, "Maaf, terjadi kesalahan saat meringkas konten halaman web.")
			return
		}

		// 4. Send the final summary to the user
		s.ChannelMessageSend(m.ChannelID, "**Berikut ringkasan dari halaman web:**\n"+summary)
	}

	if strings.HasPrefix(m.Content, "!todo-register") {

	}
}
