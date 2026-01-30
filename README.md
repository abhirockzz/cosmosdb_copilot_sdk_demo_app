# Flight Log App

The [GitHub Copilot SDK](https://github.com/github/copilot-sdk) lets you embed Copilot's agentic workflows directly into your apps. Available for Python, TypeScript, Go, and .NET — you define agent behavior, Copilot handles the rest.

This is a demo application showcasing [GitHub Copilot Go SDK](https://github.com/github/copilot-sdk/tree/main/go) integration with [Azure Cosmos DB](https://learn.microsoft.com/en-us/azure/cosmos-db/introduction).

You can:

- Bulk load sample flight data
- Use natural language queries to retrieve flight history - Copilot generates and runs Cosmos DB SQL queries automatically.
- Upload a boarding pass image - the app uses GitHub Copilot CLI built-in capabilities to extract flight details automatically.

![](images/app.png)

## Prerequisites

1. [Go](https://go.dev/)
2. **GitHub Copilot CLI** - [Install it](https://docs.github.com/en/copilot/how-tos/set-up/install-copilot-cli) and login.
3. **One of the following for Cosmos DB:**
   - **Run locally**: Use [Cosmos DB vNext emulator](https://learn.microsoft.com/en-us/azure/cosmos-db/emulator-linux)
   - **Azure**: Azure CLI + Azure Cosmos DB account

## Clone the Repository

```bash
git clone https://github.com/abhirockzz/cosmosdb_copilot_sdk_demo_app
cd cosmosdb_copilot_sdk_demo_app
```

## Option A: Using the vNext Emulator

The vNext emulator runs on Linux/macOS/Windows via Docker. No Azure account needed.

### 1. Start the Emulator

```bash
docker run -p 8081:8081 -p 1234:1234 mcr.microsoft.com/cosmosdb/linux/azure-cosmos-emulator:vnext-preview
```

### 2. Create Database and Container

Open the Data Explorer at http://localhost:1234 and create:

- Database: `flightlog`
- Container: `boardingPasses` with partition key `/email`

### 3. Run the app

```bash
export USE_EMULATOR=true
export COSMOS_ENDPOINT=http://localhost:8081

go run main.go
```

Open http://localhost:8080 in your browser.

Skip to [Usage](#usage) section below.

---

### Option B: Using Azure Cosmos DB

#### 1. Create Cosmos DB Resources

Login using Azure CLI:

```bash
az login
```

Then run the following commands to create the necessary resources:

```bash
# Set environment variables (customize these values)
export RG_NAME="flight-log-rg"
export LOCATION="westus2"
export COSMOS_ACCOUNT="flight-log-cosmos"
export COSMOS_DATABASE="flightlog"
export COSMOS_CONTAINER="boardingPasses"

# Create resource group
az group create --name $RG_NAME --location $LOCATION

# Create Cosmos DB account
az cosmosdb create \
  --name $COSMOS_ACCOUNT \
  --resource-group $RG_NAME \
  --kind GlobalDocumentDB

# Create database
az cosmosdb sql database create \
  --account-name $COSMOS_ACCOUNT \
  --resource-group $RG_NAME \
  --name $COSMOS_DATABASE

# Create container
az cosmosdb sql container create \
  --account-name $COSMOS_ACCOUNT \
  --resource-group $RG_NAME \
  --database-name $COSMOS_DATABASE \
  --name $COSMOS_CONTAINER \
  --partition-key-path /email
```

#### 2. Assign RBAC Role

```bash
# Get your user object ID
USER_ID=$(az ad signed-in-user show --query id -o tsv)

# Get Cosmos DB account ID
COSMOS_ID=$(az cosmosdb show --name $COSMOS_ACCOUNT --resource-group $RG_NAME --query id -o tsv)

# Assign Cosmos DB Built-in Data Contributor role
az cosmosdb sql role assignment create \
  --account-name $COSMOS_ACCOUNT \
  --resource-group $RG_NAME \
  --role-definition-name "Cosmos DB Built-in Data Contributor" \
  --principal-id $USER_ID \
  --scope $COSMOS_ID
```

#### 3. Run the App


```bash
export COSMOS_ENDPOINT="https://${COSMOS_ACCOUNT}.documents.azure.com:443/"
export COSMOS_DATABASE="flightlog"
export COSMOS_CONTAINER="boardingPasses"
export PORT="8080"

go run main.go
```

Open http://localhost:8080 in your browser.

---

## Usage

**Enter any email to login (just for demo purposes)** - Used as partition key for the flight data

**Load sample data** - Quick demo with pre-populated flights

![](images/home.png)

**Ask questions** - Use natural language to query flight history

![](images/ask.png)

**Add a flight** - Click "Add Flight" and upload/choose a boarding pass image

**Review extraction** - Copilot extracts flight details in real-time

![](images/analyse_boarding_pass.png)

**Save** - Confirm and save to Cosmos DB

**All Flights** - Check all the flights data in Cosmos DB

![](images/all_flights.png)