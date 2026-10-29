
FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o mini-shop-backend .

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/mini-shop-backend .
COPY migrations/ ./migrations/

EXPOSE 8080

CMD ["./mini-shop-backend"]
