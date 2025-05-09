# ──────────────────────────────────────────────────────────────────────────────
# 1) BUILD STAGE
# ──────────────────────────────────────────────────────────────────────────────
FROM golang:1.24.2-alpine AS builder

RUN apk add build-base

# disable cgo so we get a fully static binary
ENV CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

COPY . .

RUN mkdir /app

# Build the database from the JSON files
RUN go run build_db.go -source=./json -output=/app/chords.db

# build a small, static binary
# -ldflags "-s -w" strips debug info to shrink size further
RUN go build -ldflags="-s -w" -o /app/chordserver ./server.go

# ──────────────────────────────────────────────────────────────────────────────
# 2) FINAL STAGE
# ──────────────────────────────────────────────────────────────────────────────
FROM alpine:3.19

# Install required libraries for SQLite3
RUN apk add --no-cache libc6-compat sqlite

# Copy only the binary and the database
COPY --from=builder /app/chordserver /chordserver
COPY --from=builder /app/chords.db /chords.db

# Expose the port your server listens on
EXPOSE 8080

ENTRYPOINT ["/chordserver"]
