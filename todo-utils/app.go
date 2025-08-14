package todo_utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type TodoApp struct {
	HttpClient *http.Client
	APIUrl     string
}

type CreateTaskRequest struct {
	Title     string
	Status    string
	Discordid string
}

type Task struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	DiscordID string `json:"discord_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type TaskListResponse struct {
	Tasks       []Task `json:"tasks"`
	Total       int    `json:"total"`
	Page        int    `json:"page"`
	Limit       int    `json:"limit"`
	TotalPages  int    `json:"total_pages"`
}

func InitTodoAPP(httpClient *http.Client, API_Url string) *TodoApp {
	return &TodoApp{
		HttpClient: httpClient,
		APIUrl:     API_Url,
	}
}

// TODO : implement these features
/*
	1. Create task
	2. Delete task
	3. Update task
	4. Read a task
	5. Read all task (paginated by a given number)
	6. Register for the app
*/

func (t *TodoApp) CreateTask(title string, status string, userid string) (string, error) {
	/*
		1. use the t.httpclient
		2. call the url with the correct postfix
		3. handle the response (formatting and such)
		4. return the formatted message
	*/

	// constructing request
	// for now, hard coded task
	// TODO: error handling for this block

	var requestObj = CreateTaskRequest{
		Title:     title,
		Status:    status,
		Discordid: userid,
	}

	// Marshal the struct to JSON
	jsonData, err := json.Marshal(requestObj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request object: %w", err)
	}

	// Send the POST request with application/json header
	resp, err := t.HttpClient.Post(
		t.APIUrl+"/task/create", // always include http://
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Optional: log the raw body for debugging
	// fmt.Println(string(respBody))

	// Check for "error" field in JSON
	var jsonResp map[string]interface{}
	if err := json.Unmarshal(respBody, &jsonResp); err == nil {
		if errVal, exists := jsonResp["error"]; exists {
			return "", fmt.Errorf("%v", errVal)
		}
	}

	return string(respBody), nil

}

func (t *TodoApp) GetTasks(discordID string, page int, limit int) (*TaskListResponse, error) {
	// Build URL with query parameters
	apiURL := fmt.Sprintf("%s/task/user", t.APIUrl)

	// Prepare query parameters
	params := url.Values{}
	params.Add("discord_id", discordID)
	params.Add("page", fmt.Sprintf("%d", page))
	params.Add("limit", fmt.Sprintf("%d", limit))

	// Construct full URL
	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	// Make GET request
	resp, err := t.HttpClient.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON response
	var taskResponse TaskListResponse
	if err := json.Unmarshal(body, &taskResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &taskResponse, nil
}

type UpdateTaskRequest struct {
	Title     string `json:"Title"`
	Status    string `json:"Status"`
	DiscordID string `json:"DiscordID"`
}

func (t *TodoApp) UpdateTask(taskID string, title string, status string, discordID string) (string, error) {
	// Construct the request URL
	apiURL := fmt.Sprintf("%s/task/edit/%s", t.APIUrl, taskID)

	// Create the request object
	requestObj := UpdateTaskRequest{
		Title:     title,
		Status:    status,
		DiscordID: discordID,
	}

	// Marshal the struct to JSON
	jsonData, err := json.Marshal(requestObj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request object: %w", err)
	}

	// Create a PUT request
	req, err := http.NewRequest("PUT", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set the content type header
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := t.HttpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check for "error" field in JSON
	var jsonResp map[string]interface{}
	if err := json.Unmarshal(respBody, &jsonResp); err == nil {
		if errVal, exists := jsonResp["error"]; exists {
			return "", fmt.Errorf("%v", errVal)
		}
	}

	return string(respBody), nil
}

func (t *TodoApp) DeleteTask(taskID string, discordID string) (string, error) {
	// Construct the request URL with query parameters
	apiURL := fmt.Sprintf("%s/task/delete/%s", t.APIUrl, taskID)
	
	// Add discord_id as a query parameter
	params := url.Values{}
	params.Add("discord_id", discordID)
	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	// Create a DELETE request
	req, err := http.NewRequest("DELETE", fullURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set the content type header
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := t.HttpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check for "error" field in JSON
	var jsonResp map[string]interface{}
	if err := json.Unmarshal(respBody, &jsonResp); err == nil {
		if errVal, exists := jsonResp["error"]; exists {
			return "", fmt.Errorf("%v", errVal)
		}
	}

	return string(respBody), nil
}
