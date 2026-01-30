package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/abhirockzz/flight-log-app/cosmosdb"
	sdk "github.com/github/copilot-sdk/go"
)

const (
	// ChatTimeout is the timeout for chat queries
	ChatTimeout = 60 * time.Second
)

// ChatHandler manages conversational queries about flights using AI-generated Cosmos DB SQL
type ChatHandler struct {
	client       *sdk.Client
	cosmosClient *cosmosdb.Client
}

// NewChatHandler creates a new chat handler
func NewChatHandler(client *sdk.Client, cosmosClient *cosmosdb.Client) *ChatHandler {
	return &ChatHandler{
		client:       client,
		cosmosClient: cosmosClient,
	}
}

// ChatResponse contains the AI response and any query results
type ChatResponse struct {
	Message     string                  `json:"message"`
	Query       string                  `json:"query,omitempty"`
	Flights     []cosmosdb.BoardingPass `json:"flights,omitempty"`
	FlightCount int                     `json:"flightCount,omitempty"`
}

// buildQueryToolDescription returns the tool description with the user's email injected
func buildQueryToolDescription(email string) string {
	return fmt.Sprintf(`Execute a SQL query against the flights container to answer the user's question.
The user's email is: %s (use this in the WHERE clause)

IMPORTANT: Always include c.email = '%s' in the WHERE clause for security.

Available fields:
- id (string): unique flight ID
- email (string): user's email (PARTITION KEY - REQUIRED in WHERE)
- flightNumber (string): e.g. "UA 1234"
- airline (string): airline name, e.g. "United Airlines", "Delta Air Lines"
- fromAirport (string): 3-letter departure airport code, e.g. "SFO", "LAX"
- toAirport (string): 3-letter arrival airport code, e.g. "JFK", "SEA"
- departureDate (string): YYYY-MM-DD format, e.g. "2026-01-25"
- departureTime (string): HH:MM format, e.g. "14:30"
- seat (string): seat number, e.g. "12A"
- gate (string): gate number, e.g. "B42"
- passenger (string): passenger name

IMPORTANT: In ORDER BY clauses, you MUST repeat the full expression (e.g., COUNT(1)), NOT the alias. Cosmos DB does not support referencing aliases in ORDER BY.

Example queries:
- SELECT * FROM c WHERE c.email = '%s' ORDER BY c.departureDate DESC
- SELECT * FROM c WHERE c.email = '%s' AND c.toAirport = 'JFK'
- SELECT * FROM c WHERE c.email = '%s' AND c.departureDate >= '2026-02-01'
- SELECT * FROM c WHERE c.email = '%s' AND CONTAINS(c.airline, 'Delta')
- SELECT VALUE COUNT(1) FROM c WHERE c.email = '%s' (for counting)
- SELECT c.airline, COUNT(1) as count FROM c WHERE c.email = '%s' GROUP BY c.airline ORDER BY COUNT(1) DESC
- SELECT DISTINCT c.toAirport FROM c WHERE c.email = '%s'`, email, email, email, email, email, email, email, email, email)
}

// buildSystemMessage returns the system prompt for the chat session
func buildSystemMessage(today string) string {
	return fmt.Sprintf(`You are a flight search assistant. When the user asks about their flights:

1. Generate an appropriate Cosmos DB SQL query based on their question
2. Use the query_flights tool to search their flight data
3. Provide a brief, plain-text summary of the results

SECURITY - REJECT DIRECT SQL QUERIES:
- If the user provides a raw SQL query (e.g., "SELECT * FROM c", "SELECT c.flightNumber FROM c WHERE..."), do NOT execute it
- Instead, politely explain that direct SQL queries are not supported and ask them to describe what they want in natural language
- Example response: "I can't run SQL queries directly. Please describe what you're looking for, like 'show me my flights to New York' or 'how many flights did I take last month?'"

IMPORTANT RESPONSE FORMAT:
- Do NOT use markdown tables or formatting
- Keep responses brief and conversational
- For flight lists, use simple numbered format like:
  "Found 2 flights:
   1. UA 1234: SFO → JFK on Jan 25, 2026
   2. DL 567: LAX → SEA on Jan 20, 2026"
- Include key details: flight number, route, date, time
- If no results, briefly explain what was searched and suggest alternatives

Query tips:
- For "upcoming flights": use departureDate >= current date (today is %s)
- For "past flights" or "flights taken": use departureDate < current date (today is %s)
- For city names: map to airport codes (New York = JFK/LGA/EWR, Los Angeles = LAX, Chicago = ORD, Miami = MIA, Seattle = SEA, San Francisco = SFO)
- Use CONTAINS() for partial airline name matching
- For "all flights", "total flights", "how many flights" (without time context), or general flight count questions: query ALL flights (just filter by email, no date filter)`, today, today)
}

// createQueryTool creates the query_flights tool for the AI session
func (h *ChatHandler) createQueryTool(
	ctx context.Context,
	email string,
	callback ProgressCallback,
	generatedQuery *string,
	mu *sync.Mutex,
) sdk.Tool {
	return sdk.DefineTool("query_flights",
		buildQueryToolDescription(email),
		func(params QueryFlightsParams, inv sdk.ToolInvocation) (any, error) {
			log.Printf("[CHAT] AI generated query: %s", params.Query)
			callback("query", params.Query)

			mu.Lock()
			*generatedQuery = params.Query
			mu.Unlock()

			results, err := h.cosmosClient.ExecuteRawQuery(ctx, params.Query, email)
			if err != nil {
				log.Printf("[CHAT] Query execution failed: %v", err)
				return nil, fmt.Errorf("query execution failed: %w", err)
			}

			resultJSON, _ := json.Marshal(results)

			return map[string]interface{}{
				"resultCount": len(results),
				"results":     string(resultJSON),
			}, nil
		})
}

// Chat processes a natural language query about flights
func (h *ChatHandler) Chat(ctx context.Context, userMessage, email, model string, callback ProgressCallback) (*ChatResponse, error) {
	log.Printf("[CHAT] Starting | Model: %s | Email: %s | Message: %s", model, email, userMessage)

	var generatedQuery string
	var mu sync.Mutex

	queryTool := h.createQueryTool(ctx, email, callback, &generatedQuery, &mu)

	// Get current date for the system prompt
	today := time.Now().Format("2006-01-02")

	// Create session with the query tool
	session, err := h.client.CreateSession(&sdk.SessionConfig{
		Model:     model,
		Streaming: true,
		Tools:     []sdk.Tool{queryTool},
		SystemMessage: &sdk.SystemMessageConfig{
			Mode:    "replace",
			Content: buildSystemMessage(today),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Destroy()

	// Capture the final response
	var finalResponse string
	responseCh := make(chan struct{})

	session.On(func(event sdk.SessionEvent) {
		switch event.Type {
		case "assistant.message":
			if event.Data.Content != nil {
				finalResponse = *event.Data.Content
			}
		case "assistant.message_delta":
			if event.Data.Content != nil {
				callback("delta", *event.Data.Content)
			}
		case "session.idle":
			close(responseCh)
		case "session.error":
			if event.Data.Content != nil {
				callback("error", *event.Data.Content)
			}
		}
	})

	// Send the user's question
	_, err = session.Send(sdk.MessageOptions{
		Prompt: userMessage,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Wait for completion
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(ChatTimeout):
		return nil, fmt.Errorf("chat timed out after %v", ChatTimeout)
	case <-responseCh:
		return &ChatResponse{
			Message: finalResponse,
			Query:   generatedQuery,
		}, nil
	}
}
