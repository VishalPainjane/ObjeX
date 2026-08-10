# Build stage
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /objex ./cmd/objex

# Runtime stage
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /objex /app/objex
ENV OBJEX_DATA_DIR=/data OBJEX_HTTP_ADDRESS=:9000
EXPOSE 9000
VOLUME ["/data"]
ENTRYPOINT ["/app/objex"]
