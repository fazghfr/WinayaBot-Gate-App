package bot

import (
	"Discord_bot_v1/llm_utils"
	todo_utils "Discord_bot_v1/todo-utils"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

var llmService *llm_utils.LLMService
var TodoApp *todo_utils.TodoApp

// ConversationState keeps track of where the user is in the flow
type ConversationState struct {
	Step      int
	TaskTitle string
}

// PaginationState keeps track of the current page for each user
type PaginationState struct {
	Page int
}

// userStates stores ongoing conversations per user
var userStates = make(map[string]*ConversationState)

// userPagination stores pagination state for todo lists per user
var userPagination = make(map[string]*PaginationState)

// Start initializes and runs the Discord bot.
func Start(token string, service llm_utils.LLMService) {
	// 1. CREATE DISCORD SESSION
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// setting llm service to facilitate llm operations
	llmService = &service

	// initialize todoapp
	client := &http.Client{}

	// Initialize your TodoApp instance
	TodoApp = todo_utils.InitTodoAPP(client, "http://localhost:8080/api")

	// 2. DEFINE INTENTS
	// We need IntentsGuildMessages to receive message events.
	// We also need IntentsGuildMessageReactions for button interactions.
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent | discordgo.IntentsGuildMessageReactions

	// 3. ADD EVENT HANDLERS
	// Add a handler for the Ready event, which fires when the bot is connected.
	dg.AddHandler(ready)
	// Add a handler for the MessageCreate event, which fires every time a new message is created.
	// This is how the bot "waits for" and reacts to incoming messages.
	dg.AddHandler(messageCreate)

	// Add a handler for the InteractionCreate event, which fires when a user interacts with components.
	dg.AddHandler(interactionCreate)

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

	// Check if this user is in a conversation
	// TODO : move into direct messages
	if state, exists := userStates[m.Author.ID]; exists {
		switch state.Step {

		// Step 1: Get Title
		case 1:
			state.TaskTitle = m.Content
			state.Step = 2
			s.ChannelMessageSend(m.ChannelID, "Got it ✅ Now, what’s the status? (backlog, in-progress, done)")

		// Step 2: Get Status & Create Task
		case 2:
			status := m.Content

			response, err := TodoApp.CreateTask(state.TaskTitle, status, m.Author.ID)
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ %v", err))
				s.ChannelMessageSend(m.ChannelID, "Try Again")

			} else {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ Task Created: %s \n", state.TaskTitle))
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(":ledger: Task Status: %s \n", status))

				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(":debug response: %s \n", response))

			}

			// End conversation
			delete(userStates, m.Author.ID)
		}
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
		reply := fmt.Sprintf("Hello %s, I am WinayaBot, currently under development. \n"+
			"available commands \n"+
			"**!ping** -> pinging the bot\n"+
			"**!hello** -> pinging the bot with hello\n"+
			"**!help** -> this\n"+
			"**!summarize** -> summarize a long text given by a user\n"+
			"**!summarize-link** -> summarize from a given link\n"+
			"\n"+
			"\n"+
			"**Todolist Commands **\n"+
			"**!todo-create** -> create a new task\n"+
			"**!todo-list** -> view your tasks (interactive pagination)\n", m.Author.GlobalName)
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

	if strings.HasPrefix(m.Content, "!todo-create") {
		// Start the conversation
		userStates[m.Author.ID] = &ConversationState{Step: 1}
		s.ChannelMessageSend(m.ChannelID, "📝 Let's create a new task! What's the title?")
		return
	}

	if strings.HasPrefix(m.Content, "!todo-list") {
		// Set default page to 1
		page := 1

		// Check if user has an existing pagination state
		if state, exists := userPagination[m.Author.ID]; exists {
			page = state.Page
		} else {
			// Initialize pagination state
			userPagination[m.Author.ID] = &PaginationState{Page: 1}
		}

		// Fetch tasks from API with default limit of 5
		taskResponse, err := TodoApp.GetTasks(m.Author.ID, page, 5)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Error fetching tasks: %v", err))
			return
		}

		// Format the response message
		if len(taskResponse.Tasks) == 0 {
			s.ChannelMessageSend(m.ChannelID, "📭 You have no tasks yet. Use `!todo-create` to add some!")
			return
		}

		// Build the task list message
		message := fmt.Sprintf("**📋 Your Todo List (Page %d/%d)**\n\n", taskResponse.Page, taskResponse.TotalPages)

		for i, task := range taskResponse.Tasks {
			// Add emoji based on status
			statusEmoji := "📝"
			switch task.Status {
			case "done":
				statusEmoji = "✅"
			case "in-progress":
				statusEmoji = "🔄"
			case "backlog":
				statusEmoji = "📥"
			}

			message += fmt.Sprintf("`%d.` %s **%s** (%s)\n",
				(i+1)+((page-1)*5),
				statusEmoji,
				task.Title,
				task.Status)
		}

		message += fmt.Sprintf("\n📄 Page %d of %d | Total tasks: %d\n", taskResponse.Page, taskResponse.TotalPages, taskResponse.Total)

		// Add navigation buttons
		components := []discordgo.MessageComponent{}

		// Show previous button unless we're on the first page
		if taskResponse.Page > 1 {
			components = append(components, discordgo.Button{
				Label:    "⬅️ Previous",
				Style:    discordgo.PrimaryButton,
				CustomID: fmt.Sprintf("todo_prev_%d", page-1),
			})
		}

		// Show next button unless we're on the last page
		if taskResponse.Page < taskResponse.TotalPages {
			components = append(components, discordgo.Button{
				Label:    "Next ➡️",
				Style:    discordgo.PrimaryButton,
				CustomID: fmt.Sprintf("todo_next_%d", page+1),
			})
		}

		// Create actions row if we have buttons
		actions := []discordgo.MessageComponent{}
		if len(components) > 0 {
			actions = append(actions, discordgo.ActionsRow{
				Components: components,
			})
		}

		// Send message with navigation buttons
		if len(actions) > 0 {
			s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
				Content:    message,
				Components: actions,
			})
		} else {
			s.ChannelMessageSend(m.ChannelID, message)
		}
	}

}

// interactionCreate handles button interactions for pagination
func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if the interaction is a button click
	if i.Type == discordgo.InteractionMessageComponent {
		customID := i.MessageComponentData().CustomID

		// Check if it's a todo pagination button
		if strings.HasPrefix(customID, "todo_") {
			// Extract action and page number
			parts := strings.Split(customID, "_")
			if len(parts) != 3 {
				return
			}

			// action := parts[1] // "prev" or "next" (not used)
			page, err := strconv.Atoi(parts[2])
			if err != nil {
				return
			}

			// Update user's pagination state
			userPagination[i.Member.User.ID] = &PaginationState{Page: page}

			// Fetch tasks from API
			taskResponse, err := TodoApp.GetTasks(i.Member.User.ID, page, 5)
			if err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("❌ Error fetching tasks: %v", err),
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
				return
			}

			// Format the response message
			if len(taskResponse.Tasks) == 0 {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Content: "📭 You have no tasks yet. Use `!todo-create` to add some!",
					},
				})
				return
			}

			// Build the task list message
			message := fmt.Sprintf("**📋 Your Todo List (Page %d/%d)**\n\n", taskResponse.Page, taskResponse.TotalPages)

			for i, task := range taskResponse.Tasks {
				// Add emoji based on status
				statusEmoji := "📝"
				switch task.Status {
				case "done":
					statusEmoji = "✅"
				case "in-progress":
					statusEmoji = "🔄"
				case "backlog":
					statusEmoji = "📥"
				}

				message += fmt.Sprintf("`%d.` %s **%s** (%s)\n",
					(i+1)+((page-1)*5),
					statusEmoji,
					task.Title,
					task.Status)
			}

			message += fmt.Sprintf("\n📄 Page %d of %d | Total tasks: %d\n", taskResponse.Page, taskResponse.TotalPages, taskResponse.Total)

			// Add navigation buttons
			components := []discordgo.MessageComponent{}

			// Show previous button unless we're on the first page
			if taskResponse.Page > 1 {
				components = append(components, discordgo.Button{
					Label:    "⬅️ Previous",
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("todo_prev_%d", page-1),
				})
			}

			// Show next button unless we're on the last page
			if taskResponse.Page < taskResponse.TotalPages {
				components = append(components, discordgo.Button{
					Label:    "Next ➡️",
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("todo_next_%d", page+1),
				})
			}

			// Create actions row if we have buttons
			actions := []discordgo.MessageComponent{}
			if len(components) > 0 {
				actions = append(actions, discordgo.ActionsRow{
					Components: components,
				})
			}

			// Respond to the interaction with updated message
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Content:    message,
					Components: actions,
				},
			})
		}
	}
}
