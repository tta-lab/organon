FROM golang:1.26.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/src ./cmd/src && \
    CGO_ENABLED=0 go build -o /out/url ./cmd/url && \
    CGO_ENABLED=0 go build -o /out/web ./cmd/web

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/src /out/url /out/web /usr/local/bin/
