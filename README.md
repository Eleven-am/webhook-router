# 🔗 Webhook Router

A powerful, configurable webhook routing system built with Go that receives webhooks and intelligently routes them to different RabbitMQ queues based on configurable rules.

## ✨ Features

- **Dynamic Routing**: Route webhooks to different queues based on endpoint, method, and custom filters
- **Web UI**: Beautiful, responsive admin interface for managing routes
- **SQLite Database**: Persistent storage for route configurations and webhook logs
- **RabbitMQ Integration**: Reliable message queuing with connection pooling
- **Real-time Stats**: Monitor webhook traffic and route performance
- **Filtering System**: Advanced filtering based on headers and body content
- **Health Monitoring**: Built-in health checks and system status
- **Multi-Architecture Support**: Available for AMD64 and ARM64 architectures
- **Docker Ready**: Complete containerized setup with Docker Compose

## 🚀 Quick Start

### Using Docker Compose (Recommended)

1. **Create a `docker-compose.yml` file**:
```yaml
version: '3.8'

services:
  webhook-router:
    image: elevenam/webhook-router:v0.1.0
    ports:
      - "8080:8080"
    environment:
      - RABBITMQ_URL=amqp://admin:password@rabbitmq:5672/
      - DATABASE_PATH=/data/webhook_router.db
      - DEFAULT_QUEUE=webhooks
      - LOG_LEVEL=info
    volumes:
      - webhook_data:/data
    depends_on:
      - rabbitmq
    restart: unless-stopped

  rabbitmq:
    image: rabbitmq:3-management-alpine
    ports:
      - "5672:5672"
      - "15672:15672"
    environment:
      - RABBITMQ_DEFAULT_USER=admin
      - RABBITMQ_DEFAULT_PASS=password
    volumes:
      - rabbitmq_data:/var/lib/rabbitmq
    restart: unless-stopped

volumes:
  webhook_data:
  rabbitmq_data:
```

2. **Start the services**:
```bash
docker-compose up -d
```

3. **Access the applications**:
   - **Webhook Router**: http://localhost:8080
   - **RabbitMQ Management**: http://localhost:15672 (admin/password)

### Using with External RabbitMQ

If you have an existing RabbitMQ instance (like in Kubernetes):

```yaml
version: '3.8'

services:
  webhook-router:
    image: elevenam/webhook-router:v0.1.0
    ports:
      - "8080:8080"
    environment:
      - RABBITMQ_URL=amqp://your-user:your-password@your-rabbitmq-host:5672/
      - DATABASE_PATH=/data/webhook_router.db
      - DEFAULT_QUEUE=webhooks
      - LOG_LEVEL=info
    volumes:
      - webhook_data:/data
    restart: unless-stopped

volumes:
  webhook_data:
```

### Development Setup with Make

This project includes a comprehensive Makefile for development and deployment:

#### Prerequisites
```bash
# Install Go 1.24+
# Install Docker with BuildX support
# Install make
```

#### Development Commands

```bash
# Setup development environment
make setup

# Run in development mode with hot reload
make dev

# Build the application
make build

# Run tests
make test

# Format and lint code
make fmt
make vet
make lint
```

#### Docker Commands

```bash
# Setup Docker BuildX for multi-architecture builds
make docker-setup

# Build multi-architecture images and push to registry
make docker-push DOCKER_IMAGE_NAME=your-username/webhook-router

# Build with custom tags
make docker-push-tags DOCKER_IMAGE_NAME=your-username/webhook-router TAGS="latest dev v1.0"

# Build production-optimized images
make docker-production DOCKER_IMAGE_NAME=your-username/webhook-router

# Clean up Docker resources
make docker-clean
```

#### Interactive Mode
The Makefile will prompt for image names if not provided:
```bash
# This will ask for your Docker image name
make docker-push

# This will ask for image name and tags
make docker-push-tags
```

#### Environment Variables
You can also set environment variables:
```bash
export DOCKER_IMAGE_NAME=your-username/webhook-router
make docker-push
```

#### Available Make Commands
Run `make help` to see all available commands:

```bash
make help
```

### Manual Setup

1. **Install Dependencies**:
```bash
make setup
# or manually:
go mod download
```

2. **Set Environment Variables**:
```bash
export RABBITMQ_URL="amqp://guest:guest@localhost:5672/"
export DATABASE_PATH="./webhook_router.db"
export PORT="8080"
export DEFAULT_QUEUE="webhooks"
export LOG_LEVEL="info"
```

3. **Run the Application**:
```bash
make run
# or manually:
go run main.go
```

## 📡 API Endpoints

### Webhook Endpoints
- `POST /webhook/{endpoint}` - Receive webhooks for specific endpoint
- `POST /webhook` - Receive webhooks for default endpoint

### Management API
- `GET /api/routes` - List all routes
- `POST /api/routes` - Create new route
- `GET /api/routes/{id}` - Get specific route
- `PUT /api/routes/{id}` - Update route
- `DELETE /api/routes/{id}` - Delete route
- `POST /api/routes/{id}/test` - Test route

### Monitoring
- `GET /api/stats` - Get system statistics
- `GET /api/stats/route/{id}` - Get route-specific stats
- `GET /health` - Health check endpoint

## 🎯 Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | RabbitMQ connection URL |
| `DATABASE_PATH` | `./webhook_router.db` | SQLite database file path |
| `DEFAULT_QUEUE` | `webhooks` | Default queue for unmatched webhooks |
| `LOG_LEVEL` | `info` | Logging level |

### Route Configuration

Routes can be configured through the web UI or API with the following options:

```json
{
  "name": "GitHub Webhooks",
  "endpoint": "github",
  "method": "POST",
  "queue": "github-events",
  "exchange": "webhooks",
  "routing_key": "github.events",
  "filters": {
    "headers": {
      "X-GitHub-Event": "push"
    },
    "body_contains": ["repository"]
  },
  "active": true
}
```

#### Filter Options

- **Headers**: Match specific header values
- **Body Contains**: Check if body contains specific strings
- **Method**: HTTP method matching (POST, GET, PUT, DELETE, PATCH, *)
- **Endpoint**: Endpoint matching (specific name or * for wildcard)

## 📊 Webhook Payload Format

Webhooks are forwarded to RabbitMQ with the following JSON structure:

```json
{
  "method": "POST",
  "url": {
    "path": "/webhook/github",
    "query": "param=value"
  },
  "headers": {
    "Content-Type": ["application/json"],
    "X-GitHub-Event": ["push"]
  },
  "body": "{\"repository\": {...}}",
  "timestamp": "2024-01-15T10:30:00Z",
  "route_id": 1,
  "route_name": "GitHub Webhooks"
}
```

## 🔧 Advanced Configuration

### Custom Filters

Create sophisticated routing rules using JSON filters:

```json
{
  "headers": {
    "Content-Type": "application/json",
    "X-Event-Type": "payment"
  },
  "body_contains": ["succeeded", "payment_intent"]
}
```

### Queue and Exchange Setup

- **Queue Only**: Messages go directly to the specified queue
- **Exchange + Queue**: Messages are published to exchange and routed to queue via routing key
- **Exchange Only**: Messages published to exchange (useful for fanout exchanges)

### Connection Pooling

The application maintains a pool of RabbitMQ connections for optimal performance:
- Default pool size: 5 connections
- Automatic connection recovery
- Health monitoring and replacement of stale connections

## 📈 Monitoring and Observability

### Web Dashboard

The built-in web interface provides:
- Real-time statistics and metrics
- Route management and testing
- System health monitoring
- Export/import functionality

### Health Checks

Health endpoint (`/health`) monitors:
- Database connectivity
- RabbitMQ connection status
- System resources

### Logging

The application logs:
- All webhook requests and responses
- Route matching and filtering decisions
- RabbitMQ publishing status
- System health and errors

## 🐳 Docker Deployment

### Multi-Architecture Support

The Docker image `elevenam/webhook-router:v0.1.0` is built for multiple architectures:
- `linux/amd64` (Intel/AMD 64-bit)
- `linux/arm64` (ARM 64-bit, including Apple Silicon and ARM servers)

Docker will automatically pull the correct architecture for your platform.

### Production Setup

For production, customize the docker-compose.yml:

```yaml
services:
  webhook-router:
    image: elevenam/webhook-router:v0.1.0
    environment:
      - RABBITMQ_URL=amqp://user:pass@your-rabbitmq:5672/
      - DATABASE_PATH=/data/webhook_router.db
    volumes:
      - /host/data:/data
    restart: always
    deploy:
      replicas: 3
      resources:
        limits:
          cpus: '0.5'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 256M
```

### Kubernetes Deployment

Example Kubernetes deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: webhook-router
spec:
  replicas: 2
  selector:
    matchLabels:
      app: webhook-router
  template:
    metadata:
      labels:
        app: webhook-router
    spec:
      containers:
      - name: webhook-router
        image: elevenam/webhook-router:v0.1.0
        ports:
        - containerPort: 8080
        env:
        - name: RABBITMQ_URL
          value: "amqp://user:pass@rabbitmq-service:5672/"
        - name: DATABASE_PATH
          value: "/data/webhook_router.db"
        volumeMounts:
        - name: data
          mountPath: /data
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: webhook-router-pvc
```

### Scaling

Scale the webhook router horizontally:

```bash
docker-compose up -d --scale webhook-router=3
```

Add a load balancer (nginx, traefik) in front for distribution.

## 🔒 Security Considerations

1. **Authentication**: Add authentication middleware for production use
2. **HTTPS**: Use TLS/SSL certificates for secure communication
3. **Rate Limiting**: Implement rate limiting to prevent abuse
4. **Input Validation**: Validate webhook payloads and headers
5. **Network Security**: Use proper firewall rules and network segmentation

## 🧪 Testing

### Make Commands for Testing

```bash
# Run all tests
make test

# Create an example route
make example-route

# Send a test webhook
make test-webhook

# Check application health
make health

# Run load tests (requires wrk)
make load-test
```

### Manual Testing

Test a route using curl:

```bash
curl -X POST http://localhost:8080/webhook/github \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: push" \
  -d '{"repository": {"name": "test"}}'
```

### Route Testing

Use the built-in test functionality:

```bash
curl -X POST http://localhost:8080/api/routes/1/test
```

## 🔄 Migration and Backup

### Database Operations with Make

```bash
# Backup database
make db-backup

# Reset database
make db-reset
```

### Manual Database Backup

```bash
# Backup
cp webhook_router.db webhook_router_backup.db

# Restore
cp webhook_router_backup.db webhook_router.db
```

### Configuration Export/Import

Export routes via the web interface or API:

```bash
curl http://localhost:8080/api/routes > routes_backup.json
```

## 🛠️ Development

### Building Your Own Images

1. **Clone the repository**:
```bash
git clone <your-repo>
cd webhook-router
```

2. **Build multi-architecture images**:
```bash
# Setup Docker BuildX
make docker-setup

# Build and push your own images
make docker-push DOCKER_IMAGE_NAME=your-username/webhook-router

# Or with custom tags
make docker-push-tags DOCKER_IMAGE_NAME=your-username/webhook-router TAGS="latest dev v1.0"
```

3. **Development workflow**:
```bash
# Setup development environment
make setup

# Run with hot reload
make dev

# Format and test
make fmt vet test

# Build for production
make build-prod
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Format code: `make fmt`
6. Submit a pull request

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🆘 Support

For support and questions:
- Check the [Issues](https://github.com/your-repo/issues) page
- Review the documentation
- Check RabbitMQ and SQLite logs for troubleshooting

## 🗺️ Roadmap

- [ ] Authentication and authorization
- [ ] Prometheus metrics integration
- [ ] Webhook signature validation
- [ ] Rate limiting and throttling
- [ ] Multi-tenant support
- [ ] Webhook replay functionality
- [ ] Advanced filtering with regex support
- [ ] Real-time WebSocket dashboard updates