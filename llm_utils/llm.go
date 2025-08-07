package llm_utils

/*
All LLM related functions will be implemented here
- summarize and its utils
- (CS) summarize from a webpage
-

*/
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// LLMService holds the necessary information to interact with the Gemini API.
type LLMService struct {
	APIKey string
}

// --- Gemini API Request/Response Structs ---

// GeminiRequestPayload is the structure for the request body sent to Gemini.
type GeminiRequestPayload struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

// GeminiResponsePayload is the structure for parsing the response from Gemini.
type GeminiResponsePayload struct {
	Candidates []Candidate `json:"candidates"`
}

type Candidate struct {
	Content Content `json:"content"`
}

// --- Service Implementation ---

// NewLLMService creates a new instance of the LLMService for Gemini.
func NewLLMService(apiKey string) *LLMService {
	return &LLMService{APIKey: apiKey}
}

// SummarizeFromText takes text, sends it to the Gemini API for summarization, and returns the result.
func (l *LLMService) SummarizeFromText(text string) (string, error) {
	// 1. Construct the Gemini API endpoint URL.
	// We use gemini-pro as a good general-purpose model.
	apiURL := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=" + l.APIKey

	// 2. Construct the prompt.
	prompt := fmt.Sprintf("Anda adalah seorang summarizer handal. Buatlah ringkasan singkat dan substansial dari teks berikut. respon"+
		"dengan bahasa yang sama dengan bahasa dari text tersebut: \"%s\"", text)

	// 3. Create the request payload using the structs we defined.
	payload := GeminiRequestPayload{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: prompt},
				},
			},
		},
	}

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("error marshalling request body: %w", err)
	}

	// 4. Create and send the HTTP request.
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("error sending request to Gemini API: %w", err)
	}
	defer resp.Body.Close()

	// 5. Read and handle the response.
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Gemini API returned non-200 status: %s - %s", resp.Status, string(bodyBytes))
	}

	// 6. Parse the JSON response to extract the summary.
	var responseData GeminiResponsePayload
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return "", fmt.Errorf("error decoding Gemini API response: %w", err)
	}

	// 7. Extract the text from the response structure.
	if len(responseData.Candidates) > 0 && len(responseData.Candidates[0].Content.Parts) > 0 {
		summary := responseData.Candidates[0].Content.Parts[0].Text
		log.Println("Successfully received summary from Gemini.")
		return summary, nil
	}

	return "", fmt.Errorf("no summary found in Gemini response")
}

func (l *LLMService) ReadWebPages(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error making GET request: %v\n", err)
		return "", err
	}
	// 2. IMPORTANT: Defer closing the response body.
	// This ensures the connection is closed after the function finishes.
	defer resp.Body.Close()

	// 3. Read all the data from the response body stream.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %v\n", err)
		return "", err
	}

	// 4. Convert the byte slice to a string.
	bodyString := string(bodyBytes)

	// 5. Print the response body!
	fmt.Println(bodyString)
	return bodyString, nil
}
