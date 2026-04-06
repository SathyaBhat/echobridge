FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=1 GOOS=linux go build -o echobridge ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/echobridge .
COPY frontend/ ./frontend/

RUN mkdir -p /app/data

ENV ECHOBRIDGE_DB_PATH=/app/data/echobridge.db
ENV ECHOBRIDGE_UPLOAD_DIR=/app/data/uploads
ENV ECHOBRIDGE_FRONTEND_DIR=/app/frontend
ENV ECHOBRIDGE_PORT=8080

EXPOSE 8080

CMD ["./echobridge"]
