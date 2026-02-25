package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/abhirockzz/flight-log-app/ai"
	"github.com/abhirockzz/flight-log-app/cosmosdb"
	sdk "github.com/github/copilot-sdk/go"
	"github.com/google/uuid"
)

//go:embed sample_flights.json
var sampleFlightsJSON []byte

// SampleFlightTemplate represents a flight template from the JSON file
type SampleFlightTemplate struct {
	FlightNumber       string `json:"flightNumber"`
	Airline            string `json:"airline"`
	FromAirport        string `json:"fromAirport"`
	ToAirport          string `json:"toAirport"`
	DepartureDayOffset int    `json:"departureDayOffset"`
	DepartureTime      string `json:"departureTime"`
	Seat               string `json:"seat"`
	Gate               string `json:"gate"`
}

// Server handles HTTP requests for the Flight Log app
type Server struct {
	cosmos        *cosmosdb.Client
	extractor     *ai.BoardingPassExtractor
	chatHandler   *ai.ChatHandler
	copilotClient *sdk.Client
	mux           *http.ServeMux
	models        []ModelResponse // Cached models from Copilot SDK
	defaultModel  string          // Default model ID (first free+vision model)
}

// New creates a new Server instance
func New(cosmosClient *cosmosdb.Client, copilotClient *sdk.Client) *Server {
	s := &Server{
		cosmos:        cosmosClient,
		extractor:     ai.NewBoardingPassExtractor(copilotClient),
		chatHandler:   ai.NewChatHandler(copilotClient, cosmosClient),
		copilotClient: copilotClient,
		mux:           http.NewServeMux(),
	}
	s.loadModels()
	s.routes()
	return s
}

// ServeHTTP implements the http.Handler interface
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// routes sets up all HTTP routes
func (s *Server) routes() {
	// API routes
	s.mux.HandleFunc("POST /api/extract", s.handleExtract)
	s.mux.HandleFunc("POST /api/flights", s.handleCreateFlight)
	s.mux.HandleFunc("GET /api/flights", s.handleListFlights)
	s.mux.HandleFunc("GET /api/flights/all", s.handleListAllFlights)
	s.mux.HandleFunc("DELETE /api/flights/{id}", s.handleDeleteFlight)
	s.mux.HandleFunc("POST /api/sample", s.handleLoadSampleData)
	s.mux.HandleFunc("POST /api/chat", s.handleChat)
	s.mux.HandleFunc("GET /api/samples", s.handleListSamples)
	s.mux.HandleFunc("GET /api/models", s.handleModels)

	// Sample images
	s.mux.HandleFunc("GET /samples/", s.handleSampleImage)

	// Static files
	s.mux.HandleFunc("GET /", s.handleStatic)
	s.mux.HandleFunc("GET /static/", s.handleStatic)
}

// handleStatic serves static files
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}

	// Remove leading slash and "static/" prefix if present
	filePath := strings.TrimPrefix(path, "/")
	if strings.HasPrefix(filePath, "static/") {
		filePath = strings.TrimPrefix(filePath, "static/")
	}

	// Construct full path
	fullPath := filepath.Join("static", filePath)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	// Set content type based on extension
	ext := filepath.Ext(fullPath)
	switch ext {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	}

	http.ServeFile(w, r, fullPath)
}

// handleExtract handles boarding pass image upload and extraction via SSE
func (s *Server) handleExtract(w http.ResponseWriter, r *http.Request) {
	// Get email from header
	email := r.Header.Get("X-User-Email")
	if email == "" {
		http.Error(w, "X-User-Email header is required", http.StatusBadRequest)
		return
	}

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get model from form (optional, defaults to server default)
	model := r.FormValue("model")
	if model == "" {
		model = s.defaultModel
	}
	// log.Printf("[EXTRACT] Request | User: %s | Model: %s", email, model)

	// Get uploaded file
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Failed to get image: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save to temp file
	// Use UPLOAD_DIR if set (Docker Compose: shared volume with CLI container), else system temp
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = os.TempDir()
	}
	tempFile := filepath.Join(uploadDir, "boarding-pass-"+uuid.New().String()+filepath.Ext(header.Filename))
	out, err := os.Create(tempFile)
	if err != nil {
		http.Error(w, "Failed to save image: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempFile)

	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		http.Error(w, "Failed to save image: "+err.Error(), http.StatusInternalServerError)
		return
	}
	out.Close()

	// Set up SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial step (Step 1: Image uploaded)
	sendSSE(w, flusher, "step", `{"step":1,"status":"completed"}`)

	// Create callback for extraction progress
	callback := func(eventType, data string) {
		sendSSE(w, flusher, eventType, data)
	}

	// Extract flight data using Copilot
	flight, err := s.extractor.Extract(r.Context(), tempFile, email, model, callback)
	if err != nil {
		sendSSE(w, flusher, "error", err.Error())
		return
	}

	// Send extracted data
	flightJSON, _ := json.Marshal(flight)
	sendSSE(w, flusher, "extracted", string(flightJSON))
	sendSSE(w, flusher, "done", "")
}

// sendSSE sends a Server-Sent Event
func sendSSE(w http.ResponseWriter, flusher http.Flusher, event, data string) {
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// handleCreateFlight saves a confirmed flight to Cosmos DB
func (s *Server) handleCreateFlight(w http.ResponseWriter, r *http.Request) {
	var flight cosmosdb.BoardingPass
	if err := json.NewDecoder(r.Body).Decode(&flight); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if flight.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	// Save to Cosmos DB
	saved, err := s.cosmos.SaveFlight(r.Context(), &flight)
	if err != nil {
		log.Printf("Failed to save flight: %v", err)
		http.Error(w, "Failed to save flight: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(saved)
}

// handleListFlights returns recent flights for a user
func (s *Server) handleListFlights(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "email query parameter is required", http.StatusBadRequest)
		return
	}

	// Show recent flights in the main UI (sorted by most recent first)
	flights, err := s.cosmos.ListFlights(r.Context(), email)
	if err != nil {
		log.Printf("Failed to list flights: %v", err)
		http.Error(w, "Failed to list flights: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(flights)
}

// handleListAllFlights returns all flights for a user (for the expandable section)
func (s *Server) handleListAllFlights(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "email query parameter is required", http.StatusBadRequest)
		return
	}

	flights, err := s.cosmos.ListFlights(r.Context(), email)
	if err != nil {
		log.Printf("Failed to list all flights: %v", err)
		http.Error(w, "Failed to list flights: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(flights)
}

// handleDeleteFlight removes a flight from Cosmos DB
func (s *Server) handleDeleteFlight(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	email := r.URL.Query().Get("email")

	if id == "" || email == "" {
		http.Error(w, "id path parameter and email query parameter are required", http.StatusBadRequest)
		return
	}

	if err := s.cosmos.DeleteFlight(r.Context(), id, email); err != nil {
		log.Printf("Failed to delete flight: %v", err)
		http.Error(w, "Failed to delete flight: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleLoadSampleData inserts sample flights for demo purposes
func (s *Server) handleLoadSampleData(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "email query parameter is required", http.StatusBadRequest)
		return
	}

	// Parse sample flight templates from embedded JSON
	var templates []SampleFlightTemplate
	if err := json.Unmarshal(sampleFlightsJSON, &templates); err != nil {
		log.Printf("Failed to parse sample flights JSON: %v", err)
		http.Error(w, "Failed to load sample data", http.StatusInternalServerError)
		return
	}

	// Determine how many flights to select (default: 30, configurable via ?count=N)
	count := 30
	if countParam := r.URL.Query().Get("count"); countParam != "" {
		if n, err := strconv.Atoi(countParam); err == nil && n > 0 {
			count = n
		}
	}

	// Shuffle templates randomly and select up to 'count' flights
	rand.Shuffle(len(templates), func(i, j int) {
		templates[i], templates[j] = templates[j], templates[i]
	})
	if count > len(templates) {
		count = len(templates)
	}
	selected := templates[:count]

	// Derive passenger name from email prefix
	passengerName := formatNameFromEmail(email)

	// Convert templates to BoardingPass with dynamic dates
	now := time.Now()
	saved := make([]cosmosdb.BoardingPass, 0, len(selected))
	for _, tmpl := range selected {
		departureDate := now.AddDate(0, 0, tmpl.DepartureDayOffset).Format("2006-01-02")
		flight := cosmosdb.BoardingPass{
			FlightNumber:  tmpl.FlightNumber,
			Airline:       tmpl.Airline,
			FromAirport:   tmpl.FromAirport,
			ToAirport:     tmpl.ToAirport,
			DepartureDate: departureDate,
			DepartureTime: tmpl.DepartureTime,
			Seat:          tmpl.Seat,
			Gate:          tmpl.Gate,
			Passenger:     passengerName,
			Email:         email,
		}
		f, err := s.cosmos.SaveFlight(r.Context(), &flight)
		if err != nil {
			log.Printf("Failed to save sample flight: %v", err)
			continue
		}
		saved = append(saved, *f)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(saved)
}

// formatNameFromEmail extracts and formats a name from an email prefix
// e.g., "john.doe@example.com" -> "John Doe"
// e.g., "jane_smith@example.com" -> "Jane Smith"
// e.g., "bob@example.com" -> "Bob"
func formatNameFromEmail(email string) string {
	// Extract prefix before @
	atIdx := strings.Index(email, "@")
	if atIdx == -1 {
		return email
	}
	prefix := email[:atIdx]

	// Replace common separators with spaces
	prefix = strings.ReplaceAll(prefix, ".", " ")
	prefix = strings.ReplaceAll(prefix, "_", " ")
	prefix = strings.ReplaceAll(prefix, "-", " ")

	// Capitalize each word
	words := strings.Fields(prefix)
	for i, word := range words {
		if len(word) > 0 {
			runes := []rune(word)
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}

	return strings.Join(words, " ")
}

// ChatRequest represents a chat message from the user
type ChatRequest struct {
	Message string `json:"message"`
	Model   string `json:"model"`
}

// handleChat processes natural language queries about flights via SSE
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	// Get email from header
	email := r.Header.Get("X-User-Email")
	if email == "" {
		http.Error(w, "X-User-Email header is required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	// Get model (default to server default if not provided)
	model := req.Model
	if model == "" {
		model = s.defaultModel
	}
	// log.Printf("[CHAT] Request | User: %s | Model: %s | Message: %s", email, model, req.Message)

	// Set up SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Create callback for streaming updates
	callback := func(eventType, data string) {
		sendSSE(w, flusher, eventType, data)
	}

	// Process the chat query
	response, err := s.chatHandler.Chat(r.Context(), req.Message, email, model, callback)
	if err != nil {
		sendSSE(w, flusher, "error", err.Error())
		return
	}

	// Send final response
	responseJSON, _ := json.Marshal(response)
	sendSSE(w, flusher, "response", string(responseJSON))
	sendSSE(w, flusher, "done", "")
}

// handleListSamples returns a list of available sample boarding pass images
func (s *Server) handleListSamples(w http.ResponseWriter, r *http.Request) {
	samplesDir := "static/samples"

	// Read directory
	entries, err := os.ReadDir(samplesDir)
	if err != nil {
		// If directory doesn't exist, return empty array
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}

	// Filter for image files
	var samples []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" {
			samples = append(samples, "/samples/"+name)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(samples)
}

// handleSampleImage serves sample boarding pass images
func (s *Server) handleSampleImage(w http.ResponseWriter, r *http.Request) {
	// Get filename from path
	filename := strings.TrimPrefix(r.URL.Path, "/samples/")
	if filename == "" {
		http.NotFound(w, r)
		return
	}

	// Prevent directory traversal
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		http.NotFound(w, r)
		return
	}

	fullPath := filepath.Join("static", "samples", filename)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	// Set content type based on extension
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	}

	http.ServeFile(w, r, fullPath)
}

// ============================================================================
// Model Selection Support
// ============================================================================

// ModelResponse represents a model for the frontend
type ModelResponse struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Vision     bool    `json:"vision"`
	Multiplier float64 `json:"multiplier"`
	CostLabel  string  `json:"costLabel"`
}

// ModelsListResponse is the response from /api/models
type ModelsListResponse struct {
	Models       []ModelResponse `json:"models"`
	DefaultModel string          `json:"defaultModel"`
}

// loadModels fetches available models from Copilot SDK and caches them
func (s *Server) loadModels() {
	models, err := s.copilotClient.ListModels()
	if err != nil {
		log.Printf("[MODELS] Failed to fetch models: %v", err)
		// Set a fallback default
		s.defaultModel = "gpt-4.1"
		return
	}

	var visionCount, freeCount int
	s.models = make([]ModelResponse, 0, len(models))

	for _, m := range models {
		multiplier := 0.0
		if m.Billing != nil {
			multiplier = m.Billing.Multiplier
		}

		vision := m.Capabilities.Supports.Vision

		// Compute cost label
		costLabel := fmt.Sprintf("%.0f×", multiplier)
		if multiplier == 0 {
			costLabel = "Free"
			freeCount++
		} else if multiplier < 1 {
			costLabel = fmt.Sprintf("%.2g×", multiplier)
		}

		if vision {
			visionCount++
		}

		s.models = append(s.models, ModelResponse{
			ID:         m.ID,
			Name:       m.Name,
			Vision:     vision,
			Multiplier: multiplier,
			CostLabel:  costLabel,
		})
	}

	// Sort: free models first, then by multiplier ascending
	// Within same multiplier, prefer vision-capable
	sortModels(s.models)

	// Select default: prefer gpt-4.1 if free+vision, else first free+vision
	s.defaultModel = selectDefaultModel(s.models)

	log.Printf("[MODELS] Loaded %d models, %d vision-capable, %d free. Default: %s",
		len(s.models), visionCount, freeCount, s.defaultModel)
}

// sortModels sorts models: free first, then by multiplier, vision-capable preferred
func sortModels(models []ModelResponse) {
	// Simple bubble sort for small list
	for i := 0; i < len(models); i++ {
		for j := i + 1; j < len(models); j++ {
			// Compare: free < premium, lower multiplier < higher, vision > non-vision
			shouldSwap := false
			if models[j].Multiplier < models[i].Multiplier {
				shouldSwap = true
			} else if models[j].Multiplier == models[i].Multiplier && models[j].Vision && !models[i].Vision {
				shouldSwap = true
			}
			if shouldSwap {
				models[i], models[j] = models[j], models[i]
			}
		}
	}
}

// selectDefaultModel picks the best default: prefer gpt-4.1 if free+vision
func selectDefaultModel(models []ModelResponse) string {
	// First, look for gpt-4.1 if it's free and has vision
	for _, m := range models {
		if m.ID == "gpt-4.1" && m.Multiplier == 0 && m.Vision {
			return m.ID
		}
	}
	// Otherwise, first free+vision model
	for _, m := range models {
		if m.Multiplier == 0 && m.Vision {
			return m.ID
		}
	}
	// Otherwise, first free model
	for _, m := range models {
		if m.Multiplier == 0 {
			return m.ID
		}
	}
	// Fallback to first model
	if len(models) > 0 {
		return models[0].ID
	}
	return "gpt-4.1"
}

// handleModels returns the list of available models
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ModelsListResponse{
		Models:       s.models,
		DefaultModel: s.defaultModel,
	})
}
