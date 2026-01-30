package ai

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/abhirockzz/flight-log-app/cosmosdb"
	sdk "github.com/github/copilot-sdk/go"
)

const (
	// DefaultExtractionTimeout is the default timeout for boarding pass extraction
	DefaultExtractionTimeout = 60 * time.Second
)

// BoardingPassExtractor handles the extraction of flight details from boarding pass images
// using the Copilot SDK's vision capabilities.
type BoardingPassExtractor struct {
	client *sdk.Client
}

// NewBoardingPassExtractor creates a new extractor using the provided Copilot client.
func NewBoardingPassExtractor(client *sdk.Client) *BoardingPassExtractor {
	return &BoardingPassExtractor{
		client: client,
	}
}

// Extract analyzes a boarding pass image and extracts flight details.
// It uses Copilot's vision capabilities with streaming feedback via the callback.
//
// Parameters:
//   - ctx: Context for cancellation
//   - imagePath: Path to the boarding pass image file
//   - email: User's email address (used as partition key)
//   - callback: Function called with progress updates (eventType, data)
//
// Returns the extracted BoardingPass or an error if extraction fails.
func (e *BoardingPassExtractor) Extract(ctx context.Context, imagePath, email, model string, callback ProgressCallback) (*cosmosdb.BoardingPass, error) {
	log.Printf("[EXTRACT] Starting | Model: %s | Email: %s | Image: %s", model, email, imagePath)

	// Variable to capture extracted flight
	var extractedFlight *cosmosdb.BoardingPass
	var extractMu sync.Mutex

	// Define the extraction tool - this captures flight data without saving
	extractTool := e.createExtractionTool(&extractedFlight, &extractMu, callback)

	// Create session with streaming enabled
	session, err := e.client.CreateSession(&sdk.SessionConfig{
		Model:         model,
		Streaming:     true,
		Tools:         []sdk.Tool{extractTool},
		SystemMessage: e.buildSystemMessage(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Destroy()

	// Set up error channel for goroutine communication
	errCh := make(chan error, 1)

	// Set up event handler for streaming
	session.On(func(event sdk.SessionEvent) {
		e.handleSessionEvent(event, callback)
	})

	// Send the image with extraction prompt in a goroutine
	go func() {
		// Step 2: Analyzing image (AI processing starts)
		callback("step", `{"step":2,"status":"active"}`)

		prompt := fmt.Sprintf("Please analyze this boarding pass image and extract the flight details. The user's email is: %s", email)

		_, sendErr := session.Send(sdk.MessageOptions{
			Prompt: prompt,
			Attachments: []sdk.Attachment{
				{
					Type: "file",
					Path: &imagePath,
				},
			},
		})
		if sendErr != nil {
			errCh <- fmt.Errorf("failed to send message: %w", sendErr)
			return
		}
	}()

	// Wait for session to become idle (using a polling approach since we need to handle context)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(DefaultExtractionTimeout)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-errCh:
			return nil, err
		case <-timeout:
			return nil, fmt.Errorf("extraction timed out after %v", DefaultExtractionTimeout)
		case <-ticker.C:
			extractMu.Lock()
			if extractedFlight != nil {
				flight := extractedFlight
				extractMu.Unlock()
				return flight, nil
			}
			extractMu.Unlock()
		}
	}
}

// createExtractionTool creates the tool that captures extracted flight data.
// Note: This tool captures data for user confirmation - it does NOT save to the database.
func (e *BoardingPassExtractor) createExtractionTool(result **cosmosdb.BoardingPass, mu *sync.Mutex, callback ProgressCallback) sdk.Tool {
	return sdk.DefineTool("capture_flight_details", "Capture extracted boarding pass data for user confirmation",
		func(params SaveFlightParams, inv sdk.ToolInvocation) (any, error) {
			// Step 4: Ready for confirmation
			callback("step", `{"step":4,"status":"active"}`)

			flight := &cosmosdb.BoardingPass{
				Email:         params.Email,
				FlightNumber:  params.FlightNumber,
				Airline:       params.Airline,
				FromAirport:   params.FromAirport,
				ToAirport:     params.ToAirport,
				DepartureDate: params.DepartureDate,
				DepartureTime: params.DepartureTime,
				Seat:          params.Seat,
				Gate:          params.Gate,
				Passenger:     params.Passenger,
			}

			mu.Lock()
			*result = flight
			mu.Unlock()

			return map[string]string{
				"status":  "captured",
				"message": "Flight details captured successfully. User will confirm before saving.",
			}, nil
		})
}

// buildSystemMessage returns the system message configuration for the extraction session
func (e *BoardingPassExtractor) buildSystemMessage() *sdk.SystemMessageConfig {
	return &sdk.SystemMessageConfig{
		Mode: "replace",
		Content: `You are a boarding pass analyzer. When given an image of a boarding pass:

1. Carefully examine the image and extract the following information if visible:
   - Flight number (e.g., "UA 1234")
   - Airline name
   - Departure airport code (e.g., "SFO")  
   - Arrival airport code (e.g., "JFK")
   - Departure date (format as YYYY-MM-DD)
   - Departure time (format as HH:MM in 24-hour)
   - Seat number
   - Gate number
   - Passenger name

2. Once you have extracted the information, call the capture_flight_details tool with ALL the extracted data.
   Use the provided email address for the email field.

3. If any field is not visible or unclear, use an empty string for that field.

Be thorough and extract only what is clearly visible on the boarding pass.`,
	}
}

// handleSessionEvent processes session events and forwards relevant ones to the callback
func (e *BoardingPassExtractor) handleSessionEvent(event sdk.SessionEvent, callback ProgressCallback) {
	switch event.Type {
	case "assistant.message_delta":
		// Skip delta events - don't flood UI with AI thinking text
	case "tool.execution_start":
		// Step 3: Extracting details - include tool name for educational display
		toolName := "tool"
		if event.Data.ToolName != nil {
			toolName = *event.Data.ToolName
		}
		callback("step", fmt.Sprintf(`{"step":3,"status":"active","detail":"Tool: %s"}`, toolName))
	case "session.error":
		if event.Data.Content != nil {
			callback("error", *event.Data.Content)
		}
	}
}
