package ai

// SaveFlightParams defines the parameters for the save_flight tool
type SaveFlightParams struct {
	Email         string `json:"email" jsonschema:"User email (partition key)"`
	FlightNumber  string `json:"flightNumber" jsonschema:"Flight number, e.g. UA 1234"`
	Airline       string `json:"airline" jsonschema:"Airline name"`
	FromAirport   string `json:"fromAirport" jsonschema:"Departure airport code"`
	ToAirport     string `json:"toAirport" jsonschema:"Arrival airport code"`
	DepartureDate string `json:"departureDate" jsonschema:"Date in YYYY-MM-DD format"`
	DepartureTime string `json:"departureTime" jsonschema:"Time in HH:MM format"`
	Seat          string `json:"seat" jsonschema:"Seat number"`
	Gate          string `json:"gate" jsonschema:"Gate number"`
	Passenger     string `json:"passenger" jsonschema:"Passenger name"`
}

// QueryFlightsParams defines the parameters for the AI-generated SQL query tool
type QueryFlightsParams struct {
	Query string `json:"query" jsonschema:"The complete Cosmos DB SQL query to execute. Must include c.email filter."`
}

// ProgressCallback is called with extraction progress updates
type ProgressCallback func(eventType, data string)
