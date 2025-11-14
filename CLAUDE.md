# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a WhatsApp API service built in Go that allows users to manage multiple WhatsApp sessions via a REST API. It uses the `whatsmeow` library to interface with WhatsApp Web and provides JWT-authenticated endpoints for session management, messaging, and real-time updates via WebSockets.

## Architecture

### Core Components

The application is structured into 4 main files:

- **main.go**: Application entry point, configuration management, server setup with graceful shutdown
- **api.go**: HTTP handlers, middleware (JWT auth, CORS, logging), and API endpoints
- **whatsapp.go**: WhatsApp client management, session lifecycle, event handling, and messaging logic
- **database.go**: Database models, GORM repositories, and dual-database architecture

### Database Architecture

**Dual Database Setup:**
1. **MySQL** (via GORM) - Stores application data:
   - WhatsAppSession: Session metadata, status, QR codes, connection info
   - WhatsAppContact: Synced contacts with phone parsing
   - WhatsAppGroup: Group information and participant counts
   - WhatsAppEvent: Event logs for auditing

2. **SQLite** (via whatsmeow/sqlstore) - Stores WhatsApp protocol data:
   - Device keys and authentication tokens
   - Message encryption keys
   - Located at `./data/whatsapp_store.db`

### Session Management Flow

1. User creates session → Status: `pending`
2. WhatsApp client initializes → QR code generated → Status: `qr_ready`
3. User scans QR → Pairing succeeds → Status: `connected`
4. Session auto-reconnects on disconnection (if enabled)
5. Health monitor runs every 60s to restore disconnected sessions

### Key Services

**WhatsAppService** (whatsapp.go):
- Manages in-memory map of active SessionClients
- Handles WhatsApp event callbacks (QR, Connected, Disconnected, Messages)
- Provides message sending (text, image, video, audio, document)
- Auto-syncs groups and contacts after connection
- Detects business vs personal accounts

**WebSocketManager** (whatsapp.go):
- Broadcasts real-time events to connected clients
- Events: qr_ready, connected, disconnected, message_sent, session_health

**DatabaseManager** (database.go):
- GORM-based repositories for all models
- Enforces device limits (5 per user) via MySQL triggers
- Bulk upsert operations for contacts and groups

## Development Commands

### Build and Run

```bash
# Build the application
go build -o whatsapp-api.exe .

# Run directly
go run .

# Run with hot reload (if using air or similar)
air
```

### Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific test
go test -run TestFunctionName
```

### Database

The application auto-migrates tables on startup. To reset:

```bash
# Drop and recreate MySQL database
mysql -u root -p -e "DROP DATABASE IF EXISTS whatsapp_api; CREATE DATABASE whatsapp_api;"

# Delete SQLite store (will force new QR pairing)
rm -rf ./data/whatsapp_store.db
```

## Environment Configuration

Key environment variables (see `.env.example`):

```bash
# Application
APP_PORT=8080
APP_ENV=development

# Database (MySQL for app data)
DB_HOST=localhost
DB_PORT=3306
DB_NAME=whatsapp_api
DB_USER=root
DB_PASSWORD=your_password

# JWT Authentication
JWT_SECRET=your-secret-key
JWT_ISSUER=your-app-name

# WhatsApp Settings
WA_AUTO_RECONNECT=true
WA_QR_TIMEOUT=30s
MAX_DEVICES_PER_USER=5
```

## API Endpoints

All endpoints require JWT token in `Authorization: Bearer <token>` header (except `/health`).

**JWT Authentication Note**: Currently DISABLED for testing (see api.go:29-45). Auth always returns user_id=1. To enable production auth, uncomment the original validation code in `AuthMiddleware()` and `validateWebSocketToken()`.

### Session Management
- `POST /api/v1/sessions` - Create new session
- `GET /api/v1/sessions` - List user's sessions
- `GET /api/v1/sessions/:session_id/qr` - Get QR code (supports ?format=png)
- `GET /api/v1/sessions/:session_id/status` - Get session status
- `DELETE /api/v1/sessions/:session_id` - Delete session
- `POST /api/v1/sessions/:session_id/refresh` - Manually reconnect session

### Messaging
- `POST /api/v1/sessions/:session_id/send` - Send text message
- `POST /api/v1/sessions/:session_id/send-advanced` - Send media (image/video/audio/document)

### WebSocket
- `GET /api/v1/sessions/:session_id/events?token=<jwt>` - Real-time event stream

## Important Implementation Details

### Phone Number Handling

Always verify numbers with `IsOnWhatsApp()` before sending to get proper JID format. The API handles both:
- JID format: `201097154916@s.whatsapp.net`
- Phone numbers: `+201097154916` (auto-verified)

See whatsapp.go:722-797 for the message sending implementation.

### Media Upload

Media messages follow this flow:
1. Upload to WhatsApp servers via `Client.Upload()`
2. Receive encrypted media URL and keys
3. Send message with media metadata

Max sizes:
- Image: 16 MB
- Video: 100 MB
- Audio: 16 MB
- Document: 100 MB

### Session Recovery

The app automatically restores sessions on startup by:
1. Querying all devices from SQLite store
2. Matching devices to active sessions in MySQL
3. Reconnecting clients in memory

See whatsapp.go:881-965 for restoration logic.

### Branding Configuration

WhatsApp device appearance is configured via constants in whatsapp.go:30-52:
- `ClientName`: "WA Sender Pro" (shown in WhatsApp)
- `ClientPlatformType`: "Chrome" (determines icon)
- Device metadata is set on connection

### Health Monitoring

Background monitor runs every 60s (whatsapp.go:1614-1728):
- Checks all "connected" sessions in DB
- Restores sessions not in memory
- Reconnects disconnected clients
- Sends WebSocket notifications on status changes

## Common Development Scenarios

### Adding a New API Endpoint

1. Add handler function in api.go (e.g., `func (h *APIHandlers) NewHandler(c *gin.Context)`)
2. Register route in main.go's router setup
3. Extract user_id from context: `userID := c.GetInt("user_id")`
4. Return JSON with structure: `{"success": bool, "data": any, "error": string}`

### Adding a New WhatsApp Event Handler

1. Add event case in `registerEventHandlers()` (whatsapp.go:426-447)
2. Create handler function (e.g., `handleNewEvent()`)
3. Update database status if needed
4. Broadcast to WebSocket clients via `wsManager.SendToSession()`

### Adding a New Database Model

1. Define struct in database.go with GORM tags
2. Add to `AutoMigrate()` call in `Migrate()`
3. Create repository methods (Create, Get, Update, Delete)
4. Add foreign key constraints if needed

## Security Considerations

- JWT authentication is currently DISABLED (see warning in api.go:1-7)
- No rate limiting implemented
- WebSocket CORS is set to allow all origins (api.go:665-668)
- Media URLs from users are downloaded without size pre-check (header validation only)
- Phone number validation relies on WhatsApp's IsOnWhatsApp() API

## Known Issues & Limitations

- Device limit (5 per user) enforced by MySQL triggers, may cause race conditions under high concurrency
- QR codes expire after configured timeout but aren't automatically regenerated
- Group sync can hit WhatsApp rate limits (handled with retries and backoff)
- Session restoration assumes SQLite store integrity - corrupted DB requires re-pairing
- No message history persistence (messages are ephemeral events only)

## Dependencies

Key external libraries:
- `gin-gonic/gin` - HTTP framework
- `whatsmeow` - WhatsApp Web protocol implementation
- `gorm.io/gorm` - ORM for MySQL
- `gorilla/websocket` - WebSocket support
- `golang-jwt/jwt` - JWT authentication
- `google/uuid` - UUID generation
- `skip2/go-qrcode` - QR code generation

## Deployment Notes

- Application supports graceful shutdown (30s timeout)
- Auto-reconnect should be enabled in production
- Requires MySQL server for app data
- SQLite store should be backed up (contains session keys)
- Suggested to run behind reverse proxy (nginx/caddy) for TLS
