# ──────────────────────────────────────────────────────────────────────────────
# 1) DB BUILD STAGE
# ──────────────────────────────────────────────────────────────────────────────
FROM golang:1.24.2-alpine AS db-builder

RUN apk add build-base

# Enable CGO for SQLite3
ENV CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

# Copy only what's needed for database building
COPY go.mod go.sum ./
COPY build_db.go ./
COPY json/ ./json/

RUN mkdir /app

# Build the database from the JSON files
RUN go run build_db.go -source=./json -output=/app/chords.db

# ──────────────────────────────────────────────────────────────────────────────
# 2) APP BUILD STAGE
# ──────────────────────────────────────────────────────────────────────────────
FROM golang:1.24.2-alpine AS app-builder

RUN apk add build-base

# Enable CGO for SQLite3
ENV CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

# Copy only what's needed for app building
COPY go.mod go.sum ./
COPY server.go ./

RUN mkdir /app

# build a small, static binary
# -ldflags "-s -w" strips debug info to shrink size further
RUN go build -ldflags="-s -w" -o /app/chordserver ./server.go

# ──────────────────────────────────────────────────────────────────────────────
# 3) FINAL STAGE
# ──────────────────────────────────────────────────────────────────────────────
FROM alpine:3.21

# Install required libraries for SQLite3
RUN apk add --no-cache libc6-compat sqlite

# Copy the binary from app-builder and the database from db-builder
COPY --from=app-builder /app/chordserver /chordserver
COPY --from=db-builder /app/chords.db /chords.db

# Expose the port your server listens on
EXPOSE 8080

ENTRYPOINT ["/chordserver"]
