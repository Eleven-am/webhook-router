#!/bin/bash

# Run the webhook router in API-only mode

echo "Starting Webhook Router in API-only mode..."

# Build with the no-frontend main file
go build -o webhook-router-api main_no_frontend.go

# Run the server
./webhook-router-api

# Alternative: If you want to keep the frontend structure but empty:
# mkdir -p frontend/dist
# echo '<!DOCTYPE html><html><body>API Only Mode</body></html>' > frontend/dist/index.html
# go run main.go