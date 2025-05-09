# ──────────────────────────────────────────────────────────────────────────────
# 1) BASE BUILD STAGE
# ──────────────────────────────────────────────────────────────────────────────
FROM golang:1.24.2-alpine AS base-builder

RUN apk add build-base

# Enable CGO for SQLite3
ENV CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

RUN mkdir /app

COPY go.mod go.sum ./
RUN go mod download

# ──────────────────────────────────────────────────────────────────────────────
# 2) DB BUILD STAGE
# ──────────────────────────────────────────────────────────────────────────────
FROM base-builder AS db-builder

# Copy only what's needed for database building
COPY build_db.go ./
COPY json/ ./json/

# Build the database from the JSON files
RUN go run build_db.go -source=./json -output=/app/chords.db

# ──────────────────────────────────────────────────────────────────────────────
# 3) APP BUILD STAGE
# ──────────────────────────────────────────────────────────────────────────────
FROM base-builder AS app-builder

# Copy only what's needed for app building
COPY server.go ./

# build a small, static binary
# -ldflags "-s -w" strips debug info to shrink size further
RUN go build -ldflags="-s -w" -o /app/chordserver ./server.go

# ──────────────────────────────────────────────────────────────────────────────
# 4) FINAL STAGE
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
