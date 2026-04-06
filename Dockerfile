FROM golang:1.26.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/src ./cmd/src && \
    CGO_ENABLED=0 go build -o /out/web ./cmd/web && \
    CGO_ENABLED=0 go build -o /out/alert ./cmd/alert

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/src /out/web /out/alert /usr/local/bin/
