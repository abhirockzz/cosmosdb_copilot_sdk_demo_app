package cosmosdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/google/uuid"
)

// Well-known emulator key (public, safe to hardcode)
// See: https://learn.microsoft.com/en-us/azure/cosmos-db/emulator-linux
const emulatorKey = "C2y6yDjf5/R+ob0N8A7Cgv30VRDJIWEHLM+4QDU5DE2nQ9nDuVTqobD4b8mGGyPMbIZnqyMsEcaGQy67XIw/Jw=="

// BoardingPass represents a flight extracted from a boarding pass image
type BoardingPass struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	FlightNumber  string `json:"flightNumber"`
	Airline       string `json:"airline"`
	FromAirport   string `json:"fromAirport"`
	ToAirport     string `json:"toAirport"`
	DepartureDate string `json:"departureDate"`
	DepartureTime string `json:"departureTime"`
	Seat          string `json:"seat"`
	Gate          string `json:"gate"`
	Passenger     string `json:"passenger"`
	CreatedAt     string `json:"createdAt"`
}

// Client wraps the Azure Cosmos DB client
type Client struct {
	client    *azcosmos.Client
	container *azcosmos.ContainerClient
}

// NewClient creates a new Cosmos DB client.
// When USE_EMULATOR=true, uses key-based auth with the well-known emulator key (HTTP only).
// Otherwise, uses DefaultAzureCredential for Azure service authentication.
// Expects the database and container to already exist.
func NewClient(endpoint, database, container string) (*Client, error) {
	var cosmosClient *azcosmos.Client
	var err error

	if os.Getenv("USE_EMULATOR") == "true" {
		// Emulator mode: use well-known key (HTTP only, no TLS)
		keyCred, keyErr := azcosmos.NewKeyCredential(emulatorKey)
		if keyErr != nil {
			return nil, fmt.Errorf("failed to create key credential: %w", keyErr)
		}
		cosmosClient, err = azcosmos.NewClientWithKey(endpoint, keyCred, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create Cosmos client (emulator): %w", err)
		}
		log.Println("Using Cosmos DB Emulator (HTTP mode)")
	} else {
		// Azure mode: use DefaultAzureCredential (supports Azure CLI, managed identity, etc.)
		cred, credErr := azidentity.NewDefaultAzureCredential(nil)
		if credErr != nil {
			return nil, fmt.Errorf("failed to create credential: %w", credErr)
		}
		cosmosClient, err = azcosmos.NewClient(endpoint, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create Cosmos client: %w", err)
		}
	}

	// Note: Database and container must be pre-created via Azure CLI, Portal, or Emulator Data Explorer
	containerClient, err := cosmosClient.NewContainer(database, container)
	if err != nil {
		return nil, fmt.Errorf("failed to get container client: %w", err)
	}

	return &Client{
		client:    cosmosClient,
		container: containerClient,
	}, nil
}

// SaveFlight saves a boarding pass to Cosmos DB
func (c *Client) SaveFlight(ctx context.Context, flight *BoardingPass) (*BoardingPass, error) {
	if flight.Email == "" {
		return nil, errors.New("email is required")
	}

	// Generate ID if not provided
	if flight.ID == "" {
		flight.ID = uuid.New().String()
	}

	// Set creation timestamp
	if flight.CreatedAt == "" {
		flight.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	// Marshal to JSON
	data, err := json.Marshal(flight)
	if err != nil {
		return nil, err
	}

	// Create partition key from email
	pk := azcosmos.NewPartitionKeyString(flight.Email)

	// Create item in Cosmos DB
	_, err = c.container.CreateItem(ctx, pk, data, nil)
	if err != nil {
		return nil, err
	}

	return flight, nil
}

// ListFlights retrieves all flights for a user
func (c *Client) ListFlights(ctx context.Context, email string) ([]BoardingPass, error) {
	if email == "" {
		return nil, errors.New("email is required")
	}

	pk := azcosmos.NewPartitionKeyString(email)

	// Query all items in the partition
	query := "SELECT * FROM c WHERE c.email = @email"
	queryOptions := &azcosmos.QueryOptions{
		QueryParameters: []azcosmos.QueryParameter{
			{Name: "@email", Value: email},
		},
	}

	pager := c.container.NewQueryItemsPager(query, pk, queryOptions)

	var flights []BoardingPass
	for pager.More() {
		response, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, item := range response.Items {
			var flight BoardingPass
			if err := json.Unmarshal(item, &flight); err != nil {
				continue
			}
			flights = append(flights, flight)
		}
	}

	// Sort by departure date descending
	sort.Slice(flights, func(i, j int) bool {
		return flights[i].DepartureDate > flights[j].DepartureDate
	})

	return flights, nil
}

// DeleteFlight removes a flight from Cosmos DB
func (c *Client) DeleteFlight(ctx context.Context, id, email string) error {
	if id == "" || email == "" {
		return errors.New("id and email are required")
	}

	pk := azcosmos.NewPartitionKeyString(email)

	_, err := c.container.DeleteItem(ctx, pk, id, nil)
	return err
}

// GetFlight retrieves a single flight by ID
func (c *Client) GetFlight(ctx context.Context, id, email string) (*BoardingPass, error) {
	if id == "" || email == "" {
		return nil, errors.New("id and email are required")
	}

	pk := azcosmos.NewPartitionKeyString(email)

	response, err := c.container.ReadItem(ctx, pk, id, nil)
	if err != nil {
		return nil, err
	}

	var flight BoardingPass
	if err := json.Unmarshal(response.Value, &flight); err != nil {
		return nil, err
	}

	return &flight, nil
}

// ExecuteQuery runs an AI-generated SQL query against the container.
// The email parameter is used as the partition key for efficient queries.
// The query should include c.email = '<email>' in the WHERE clause.
func (c *Client) ExecuteQuery(ctx context.Context, query, email string) ([]BoardingPass, error) {
	if email == "" {
		return nil, errors.New("email is required for partition-scoped queries")
	}

	// Use partition key for efficient single-partition query
	pk := azcosmos.NewPartitionKeyString(email)

	pager := c.container.NewQueryItemsPager(query, pk, nil)

	var flights []BoardingPass
	for pager.More() {
		response, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("query failed: %w", err)
		}

		for _, item := range response.Items {
			var flight BoardingPass
			if err := json.Unmarshal(item, &flight); err != nil {
				continue
			}
			flights = append(flights, flight)
		}
	}

	return flights, nil
}

// ExecuteRawQuery runs an AI-generated SQL query and returns raw JSON results.
// This handles any query type including aggregates (COUNT, SUM), GROUP BY, DISTINCT, etc.
// The email parameter is used as the partition key for efficient queries.
func (c *Client) ExecuteRawQuery(ctx context.Context, query, email string) ([]json.RawMessage, error) {
	// log.Printf("[COSMOS] ExecuteRawQuery called")
	// log.Printf("[COSMOS] Query: %s", query)
	// log.Printf("[COSMOS] Email (partition key): %s", email)

	if email == "" {
		return nil, errors.New("email is required for partition-scoped queries")
	}

	// Use partition key for efficient single-partition query
	pk := azcosmos.NewPartitionKeyString(email)

	pager := c.container.NewQueryItemsPager(query, pk, nil)

	var results []json.RawMessage
	pageCount := 0
	for pager.More() {
		pageCount++
		response, err := pager.NextPage(ctx)
		if err != nil {
			log.Printf("[COSMOS] Query failed on page %d: %v", pageCount, err)
			return nil, fmt.Errorf("query failed: %w", err)
		}
		// log.Printf("[COSMOS] Page %d returned %d items", pageCount, len(response.Items))

		for _, item := range response.Items {
			// log.Printf("[COSMOS] Item %d: %s", i, string(item))
			results = append(results, json.RawMessage(item))
		}
	}

	// log.Printf("[COSMOS] Total results: %d", len(results))
	return results, nil
}
