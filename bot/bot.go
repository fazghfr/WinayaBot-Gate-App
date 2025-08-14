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
	Step       int
	TaskTitle  string
	TaskID     string
	TaskStatus string
	Action     string // "create", "update", or "delete"
	Attempts   int    // Number of attempts for update/delete operations
	TaskNumber int    // The friendly number the user provided
}

// PaginationState keeps track of the current page for each user
type PaginationState struct {
	Page int
	// TaskIDMap maps friendly numbers to actual task IDs
	TaskIDMap map[int]string
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
		switch state.Action {
		case "create":
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
			
		case "update":
			switch state.Step {
			// Step 1: Get Title
			case 1:
				if strings.ToLower(m.Content) != "skip" {
					state.TaskTitle = m.Content
				}
				state.Step = 2
				s.ChannelMessageSend(m.ChannelID, "Got it ✅ Now, what’s the status? (backlog, in-progress, done) (Type 'skip' to keep the current status)")

			// Step 2: Get Status & Update Task
			case 2:
				if strings.ToLower(m.Content) != "skip" {
					state.TaskStatus = m.Content
				}
				state.Step = 3

				// Look up the actual task ID using the friendly number
				paginationState, exists := userPagination[m.Author.ID]
				if !exists || paginationState.TaskIDMap == nil {
					s.ChannelMessageSend(m.ChannelID, "❌ Error: Task list not found. Please run `!todo-list` first.")
					delete(userStates, m.Author.ID)
					return
				}

				taskID, taskExists := paginationState.TaskIDMap[state.TaskNumber]
				if !taskExists {
					state.Attempts++
					if state.Attempts >= 3 {
						s.ChannelMessageSend(m.ChannelID, "❌ Too many invalid attempts. Please run `!todo-list` to see the current task numbers.")
						delete(userStates, m.Author.ID)
						return
					}
					s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Invalid task number. Please try again. (%d/3 attempts)", state.Attempts))
					state.Step = 1 // Reset to step 1 to ask for title again
					s.ChannelMessageSend(m.ChannelID, "📝 Let's update your task! What's the new title? (Type 'skip' to keep the current title)")
					return
				}

				// Use existing title/status if not provided
				title := state.TaskTitle
				status := state.TaskStatus
				
				// If title or status is empty, we need to get the current values
				// For simplicity, we'll just use empty strings and let the API handle defaults
				// In a production app, you might want to fetch the current task details first
				
				response, err := TodoApp.UpdateTask(taskID, title, status, m.Author.ID)
				if err != nil {
					s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ %v", err))
					s.ChannelMessageSend(m.ChannelID, "Try Again")

				} else {
					s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ Task Updated: %s \n", title))
					s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(":ledger: New Task Status: %s \n", status))

					s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(":debug response: %s \n", response))

				}

				// End conversation
				delete(userStates, m.Author.ID)
			}
			
		case "delete":
			switch state.Step {
			// Step 1: Confirm deletion
			case 1:
				if strings.ToLower(m.Content) == "yes" {
					// Look up the actual task ID using the friendly number
					paginationState, exists := userPagination[m.Author.ID]
					if !exists || paginationState.TaskIDMap == nil {
						s.ChannelMessageSend(m.ChannelID, "❌ Error: Task list not found. Please run `!todo-list` first.")
						delete(userStates, m.Author.ID)
						return
					}

					taskID, taskExists := paginationState.TaskIDMap[state.TaskNumber]
					if !taskExists {
						state.Attempts++
						if state.Attempts >= 3 {
							s.ChannelMessageSend(m.ChannelID, "❌ Too many invalid attempts. Please run `!todo-list` to see the current task numbers.")
							delete(userStates, m.Author.ID)
							return
						}
						s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Invalid task number. Please try again. (%d/3 attempts)", state.Attempts))
						s.ChannelMessageSend(m.ChannelID, "🗑️ Are you sure you want to delete this task? Type 'yes' to confirm or 'no' to cancel.")
						return
					}

					response, err := TodoApp.DeleteTask(taskID, m.Author.ID)
					if err != nil {
						s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ %v", err))
						s.ChannelMessageSend(m.ChannelID, "Try Again")

					} else {
						s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("✅ Task Deleted Successfully\n"))
						s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(":debug response: %s \n", response))
					}
				} else {
					s.ChannelMessageSend(m.ChannelID, "🗑️ Task deletion cancelled.")
				}

				// End conversation
				delete(userStates, m.Author.ID)
			}
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

	// Helper function to show task list
	showTaskList := func() {
		// Set default page to 1
		page := 1

		// Check if user has an existing pagination state
		if state, exists := userPagination[m.Author.ID]; exists {
			page = state.Page
		} else {
			// Initialize pagination state
			userPagination[m.Author.ID] = &PaginationState{Page: 1, TaskIDMap: make(map[int]string)}
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

		// Initialize or reset the task ID map for this page
		userPagination[m.Author.ID].TaskIDMap = make(map[int]string)
		
		// Build the task list message
		message := fmt.Sprintf("**📋 Your Todo List (Page %d/%d)**\n\n", taskResponse.Page, taskResponse.TotalPages)

		for i, task := range taskResponse.Tasks {
			// Calculate the friendly number for this task
			friendlyNumber := (i + 1) + ((page - 1) * 5)
			
			// Store the mapping between friendly number and actual task ID
			userPagination[m.Author.ID].TaskIDMap[friendlyNumber] = task.ID
			
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
				friendlyNumber,
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
		reply := fmt.Sprintf("Hey %s! 👋 I'm WinayaBot, here to help you manage your tasks and more!\n\n"+
			"**General Commands:**\n"+
			"• `!ping` - Check if I'm alive\n"+
			"• `!hello` - Get a friendly greeting\n"+
			"• `!help` - Show this help message\n"+
			"• `!summarize <text>` - Summarize a long piece of text\n"+
			"• `!summarize-link <url>` - Summarize the content of a webpage\n\n"+
			"**Task Management:**\n"+
			"• `!todo-create` - Create a new task\n"+
			"• `!todo-list` - View your tasks (with pagination)\n"+
			"• `!todo-update <number>` - Update a task (use the number from !todo-list)\n"+
			"• `!todo-delete <number>` - Delete a task (use the number from !todo-list)\n\n"+
			"Just type any command to get started!", m.Author.GlobalName)
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
		userStates[m.Author.ID] = &ConversationState{Step: 1, Action: "create"}
		s.ChannelMessageSend(m.ChannelID, "📝 Let's create a new task! What's the title?")
		return
	}

	if strings.HasPrefix(m.Content, "!todo-update") {
		// Check if the command is exactly "!todo-update" with no arguments
		if strings.TrimSpace(m.Content) == "!todo-update" {
			// No number provided, show the task list automatically
			s.ChannelMessageSend(m.ChannelID, "No task number provided. Here's your task list:")
			showTaskList()
			return
		}
		
		// Extract task number from command
		taskNumberStr := strings.TrimPrefix(m.Content, "!todo-update ")
		if taskNumberStr == "" {
			// No number provided, show the task list automatically
			s.ChannelMessageSend(m.ChannelID, "No task number provided. Here's your task list:")
			showTaskList()
			return
		}
		
		// Convert to integer
		taskNumber, err := strconv.Atoi(taskNumberStr)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "❌ Please provide a valid task number. Usage: `!todo-update <number>`")
			return
		}
		
		// Start the update conversation
		userStates[m.Author.ID] = &ConversationState{Step: 1, TaskNumber: taskNumber, Action: "update", Attempts: 0}
		s.ChannelMessageSend(m.ChannelID, "📝 Let's update your task! What's the new title? (Type 'skip' to keep the current title)")
		return
	}

	if strings.HasPrefix(m.Content, "!todo-delete") {
		// Check if the command is exactly "!todo-delete" with no arguments
		if strings.TrimSpace(m.Content) == "!todo-delete" {
			// No number provided, show the task list automatically
			s.ChannelMessageSend(m.ChannelID, "No task number provided. Here's your task list:")
			showTaskList()
			return
		}
		
		// Extract task number from command
		taskNumberStr := strings.TrimPrefix(m.Content, "!todo-delete ")
		if taskNumberStr == "" {
			// No number provided, show the task list automatically
			s.ChannelMessageSend(m.ChannelID, "No task number provided. Here's your task list:")
			showTaskList()
			return
		}
		
		// Convert to integer
		taskNumber, err := strconv.Atoi(taskNumberStr)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "❌ Please provide a valid task number. Usage: `!todo-delete <number>`")
			return
		}
		
		// Start the delete conversation
		userStates[m.Author.ID] = &ConversationState{Step: 1, TaskNumber: taskNumber, Action: "delete", Attempts: 0}
		s.ChannelMessageSend(m.ChannelID, "🗑️ Are you sure you want to delete this task? Type 'yes' to confirm or 'no' to cancel.")
		return
	}

	if strings.HasPrefix(m.Content, "!todo-list") {
		showTaskList()
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
			userID := i.Member.User.ID
			if _, exists := userPagination[userID]; !exists {
				userPagination[userID] = &PaginationState{TaskIDMap: make(map[int]string)}
			}
			userPagination[userID].Page = page

			// Fetch tasks from API
			taskResponse, err := TodoApp.GetTasks(userID, page, 5)
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

			// Reset the task ID map for this page
			userPagination[userID].TaskIDMap = make(map[int]string)
			
			// Build the task list message
			message := fmt.Sprintf("**📋 Your Todo List (Page %d/%d)**\n\n", taskResponse.Page, taskResponse.TotalPages)

			for i, task := range taskResponse.Tasks {
				// Calculate the friendly number for this task
				friendlyNumber := (i + 1) + ((page - 1) * 5)
				
				// Store the mapping between friendly number and actual task ID
				userPagination[userID].TaskIDMap[friendlyNumber] = task.ID
				
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
					friendlyNumber,
					statusEmoji,
					task.Title,
					task.Status)
			}

			message += fmt.Sprintf("\n📄 Page %d of %d | Total tasks: %d\n", taskResponse.Page, taskResponse.TotalPages, taskResponse.Total)
			message += "Use `!todo-update <number>` or `!todo-delete <number>` to modify tasks\n"

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
