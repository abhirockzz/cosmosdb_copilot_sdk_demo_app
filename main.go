package main

import (
	"log"
	"net/http"
	"os"

	"github.com/abhirockzz/flight-log-app/cosmosdb"
	"github.com/abhirockzz/flight-log-app/server"
	sdk "github.com/github/copilot-sdk/go"
)

const (
	defaultDatabase  = "flightlog"
	defaultContainer = "boardingPasses"
)

func main() {
	// Get Cosmos DB endpoint from environment
	endpoint := os.Getenv("COSMOS_ENDPOINT")
	if endpoint == "" {
		log.Fatal("COSMOS_ENDPOINT environment variable is required")
	}

	// Database name with default
	database := os.Getenv("COSMOS_DATABASE")
	if database == "" {
		database = defaultDatabase
	}

	// Container name with default
	container := os.Getenv("COSMOS_CONTAINER")
	if container == "" {
		container = defaultContainer
	}

	// Initialize Cosmos DB client
	cosmosClient, err := cosmosdb.NewClient(endpoint, database, container)
	if err != nil {
		log.Fatalf("Failed to initialize Cosmos DB client: %v", err)
	}

	// Initialize Copilot SDK client
	copilotClient := sdk.NewClient(&sdk.ClientOptions{
		LogLevel: "error",
	})
	if err := copilotClient.Start(); err != nil {
		log.Fatalf("Failed to start Copilot client: %v", err)
	}
	defer copilotClient.Stop()

	// Create server
	srv := server.New(cosmosClient, copilotClient)

	// Get port from environment or default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Flight Log app starting on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, srv); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
