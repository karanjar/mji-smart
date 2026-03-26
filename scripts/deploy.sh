#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}Starting Mji-Smart Deployment...${NC}"

# Load environment variables
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Check prerequisites
command -v docker >/dev/null 2>&1 || { echo -e "${RED}Docker is required but not installed. Aborting.${NC}" >&2; exit 1; }
command -v docker-compose >/dev/null 2>&1 || { echo -e "${RED}Docker Compose is required but not installed. Aborting.${NC}" >&2; exit 1; }

# Pull latest images
echo -e "${YELLOW}Pulling latest Docker images...${NC}"
docker-compose pull

# Build images
echo -e "${YELLOW}Building services...${NC}"
docker-compose build --parallel

# Run database migrations
echo -e "${YELLOW}Running database migrations...${NC}"
docker-compose run --rm go-engine ./migrate up

# Start services
echo -e "${YELLOW}Starting services...${NC}"
docker-compose up -d

# Wait for services to be healthy
echo -e "${YELLOW}Waiting for services to be healthy...${NC}"
sleep 30

# Check health
echo -e "${YELLOW}Checking health status...${NC}"
curl -f http://localhost:3000/health || { echo -e "${RED}Health check failed${NC}"; exit 1; }
curl -f http://localhost:8000/health || { echo -e "${RED}AI service health check failed${NC}"; exit 1; }

echo -e "${GREEN}Deployment completed successfully!${NC}"
echo -e "${GREEN}Services running:${NC}"
echo "  - API Gateway: http://localhost:3000"
echo "  - AI Service: http://localhost:8000"
echo "  - Grafana: http://localhost:3001"
echo "  - Kibana: http://localhost:5601"
echo "  - MinIO Console: http://localhost:9001"