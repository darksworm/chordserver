# ──────────────────────────────────────────────────────────────────────────────
# 1) BUILD STAGE
# ──────────────────────────────────────────────────────────────────────────────
FROM golang:1.21-alpine AS builder

# disable cgo so we get a fully static binary
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

COPY . .

# build a small, static binary
# -ldflags "-s -w" strips debug info to shrink size further
RUN go build -ldflags="-s -w" -o /app/chordserver ./main.go

# ──────────────────────────────────────────────────────────────────────────────
# 2) FINAL STAGE
# ──────────────────────────────────────────────────────────────────────────────
FROM scratch

# Copy only the binary and the json folder
COPY --from=builder /app/chordserver /chordserver
COPY --from=builder /src/json        /json

# Expose the port your server listens on
EXPOSE 8080

# no shell in scratch, so use exec form
ENTRYPOINT ["/chordserver"]
