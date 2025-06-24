#!/bin/bash

# Build and run the Webhook Router server using Docker

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Building Webhook Router Server Docker Image...${NC}"

# Check if ENCRYPTION_KEY is set
if [ -z "$ENCRYPTION_KEY" ]; then
    echo -e "${YELLOW}Warning: ENCRYPTION_KEY not set. Using default (not secure for production)${NC}"
    echo -e "${YELLOW}Set it with: export ENCRYPTION_KEY=\"your-32-character-secret-key-here\"${NC}"
fi

# Build the Docker image
docker build -f Dockerfile.server -t webhook-router-server:latest .

echo -e "${GREEN}Build complete!${NC}"
echo ""
echo -e "${GREEN}To run the server:${NC}"
echo ""
echo "  # With Docker run:"
echo "  docker run -d \\"
echo "    --name webhook-router \\"
echo "    -p 8080:8080 \\"
echo "    -v webhook_data:/data \\"
echo "    -e ENCRYPTION_KEY=\"\${ENCRYPTION_KEY:-your-32-char-key-change-this-now!}\" \\"
echo "    webhook-router-server:latest"
echo ""
echo "  # Or with Docker Compose:"
echo "  docker-compose -f docker-compose.server.yml up -d"
echo ""
echo -e "${GREEN}To view logs:${NC}"
echo "  docker logs -f webhook-router"
echo ""
echo -e "${GREEN}To stop:${NC}"
echo "  docker stop webhook-router && docker rm webhook-router"