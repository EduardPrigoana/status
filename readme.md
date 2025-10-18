# API Monitor

A lightweight, real-time monitoring system for API and UI instances with Server-Sent Events (SSE) for live updates.

## Features

- ✅ Real-time monitoring with SSE
- 📊 Visual uptime history (24h/7d views)
- 🎯 Embeddable status badges
- 📱 Responsive design
- 🚀 High performance (handles 1000+ instances)
- 🔄 Automatic reconnection
- 📈 Uptime percentage tracking
- ⚡ Average response time monitoring
- 🎨 Clean, minimal UI

## Quick Start

### Prerequisites

- Go 1.21 or higher

### Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd api-monitor
```

2. Copy the example environment file:
```bash
cp .env.example .env
```

3. Edit `.env` with your configuration.

4. Run the application:
```bash
go run .
```

The server will start on `http://localhost:8080`

### Using Environment Variables
```bash
# Custom port
PORT=3000 go run .

# Check every 5 minutes
CHECK_INTERVAL_MINUTES=5 go run .

# Combined
PORT=3000 CHECK_INTERVAL_MINUTES=5 go run .
```

## Configuration

All configuration is done via environment variables:

| Variable | Default | Description |
|-----------|----------|-------------|
| `PORT` | 8080 | Server port |
| `CHECK_INTERVAL_MINUTES` | 60 | How often to check instances (minutes) |
| `REQUEST_TIMEOUT_SECONDS` | 30 | HTTP request timeout (seconds) |
| `MAX_CHECK_HISTORY` | 168 | Maximum checks to store per instance |
| `INSTANCES_URL` | GitHub URL | URL to fetch instances JSON |
| `SSE_KEEPALIVE_SECONDS` | 30 | SSE keepalive ping interval |
| `LOG_LEVEL` | info | Logging level (info/debug) |

