# GoSwift: An Experimental Web Framework

Welcome to **GoSwift**, an experimental, lightweight, and opinionated web framework built in Go, crafted by Soem. This project serves as a personal exploration into building a foundational web framework, focusing on **simplicity**, **modularity**, and **ease of use** for rapid application development.

GoSwift aims to provide the essential building blocks for web services and applications, from robust routing and middleware to flexible context management and integrated utilities like logging and configuration.

---

## Showcase Application: QuikDocs

To demonstrate GoSwift's capabilities in a real-world scenario, this repository includes **QuikDocs**, a simple, collaborative document editing application. QuikDocs leverages GoSwift's core features to provide:

- **User Authentication**: Sign-up and Login.
- **Document Management (CRUD)**: Create, Read, Update, Delete documents.
- **Version History**: Simple tracking of document changes.
- **Real-time Collaboration**: Using Server-Sent Events (SSE) for instant updates.
- **Public Sharing**: Generate shareable links for documents.
- **Vanilla JavaScript Frontend**: Served directly by the Go backend using Tailwind CSS.

---

## Features

GoSwift provides a set of core features designed to simplify web development:

- **Fast & Lightweight**: Built on Go's `net/http` package for high performance.
- **Intuitive Routing**: Simple API for defining HTTP routes with path parameters.
- **Flexible Middleware System**: Easily chain and apply middleware.
- **Request Context**: Manage request-scoped data and response types.
- **Built-in Utilities**:
  - Logger
  - Config Manager
  - Custom HTTP Error Handling
  - Basic Metrics
  - Basic Dependency Injection
  - SSE for real-time communication
  - Response Helpers
  - Static File Serving

---

## Getting Started

### Prerequisites

- Go 1.20+
- Git

### Installation & Running QuikDocs

```bash
git clone https://github.com/your-repo/go-swift.git
cd go-swift/quikdocs/backend
go mod tidy
go run .
```

Open your browser at: [http://localhost:8080](http://localhost:8080)

---

## GoSwift Core Concepts

### Engine

```go
app := goswift.New()
app.Use(goswift.LoggerMiddleware())
app.GET("/", myHandler)
app.Run(":8080")
```

### Context (`*goswift.Context`)

Handles:
- Request data: `c.Param`, `c.Query`, `c.BindJSON`, etc.
- Response methods: `c.JSON`, `c.String`, `c.HTML`, etc.
- Request-scoped data: `c.Set`, `c.Get`
- Engine access: `c.engine.Logger`, `c.engine.Config`

### HandlerFunc

```go
type HandlerFunc func(c *Context) error
```

---

## Key Components & Usage

### Routing

```go
app.GET("/", ...)
app.GET("/users/:id", ...)
app.POST("/items", ...)
```

### Middleware

Global or group-based:

```go
app.Use(goswift.LoggerMiddleware())

api := app.Group("/api")
api.Use(goswift.JWTAuthMiddleware())
```

Built-ins include:
- LoggerMiddleware
- RecoveryMiddleware
- RequestIDMiddleware
- CORSMiddleware
- JWTAuthMiddleware
- TimeoutMiddleware
- BasicAuth
- MetricsMiddleware
- Proxy

---

## Error Handling

```go
return goswift.NewHTTPError(http.StatusNotFound, "User not found")
```

Custom global error handlers via `app.SetErrorHandler()`.

---

## Configuration (ConfigManager)

```go
app.Config.Set("DATABASE_URL", "...")
val := app.Config.Get("DATABASE_URL")
```

---

## Logging (Logger)

```go
app.Logger.Info("Started")
app.Logger.Warning("Deprecated")
app.Logger.Error("Failure: %v", err)
```

---

## Static File Serving

```go
//go:embed static/*
var embeddedFiles embed.FS

app.StaticFS("/", embeddedFiles)
```

---

## Server-Sent Events (SSE)

```go
api.GET("/docs/:id/subscribe", ...)
sseManager.Broadcast(docID, newContent)
```

---

## Project Structure

```
quikdocs/
├── backend/
│   ├── go.mod
│   ├── main.go
│   ├── static/
│   │   └── index.html
│   ├── goswift/
│   │   ├── auth.go
│   │   ├── config.go
│   │   ├── context.go
│   │   ├── debug.go
│   │   ├── errors.go
│   │   ├── goswift.go
│   │   ├── jwt.go
│   │   ├── logger.go
│   │   ├── metrics.go
│   │   ├── middleware.go
│   │   ├── plugin.go
│   │   ├── router.go
│   │   └── sse.go
│   └── go.sum
└── README.md
```

---

## Deployment to Render

1. Commit code (including `render.yaml` in `quikdocs/backend/`)
2. Push to GitHub
3. Connect to Render & create "New Web Service"
4. Deploy and access live URL

---

## Contribution & Future Work

This project is experimental and built by Soem. Feedback is welcome.

**Possible future additions**:
- Advanced error handling & custom pages
- DB integration (PostgreSQL/GORM)
- Templating engine
- WebSockets
- Config enhancement
- Testing suite

---

**Thank you for exploring GoSwift!**
