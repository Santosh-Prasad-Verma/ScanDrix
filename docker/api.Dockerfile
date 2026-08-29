FROM golang:alpine AS builder
ENV GOTOOLCHAIN=auto
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /build/bin/scandrix-api ./cmd/api

FROM alpine:3.21 AS runner
RUN apk add --no-cache ca-certificates tzdata git && \
    addgroup -g 10001 -S scandrix && \
    adduser -u 10001 -S scandrix -G scandrix -h /home/scandrix
WORKDIR /app
COPY --from=builder /build/bin/scandrix-api /app/bin/scandrix-api
RUN chown -R scandrix:scandrix /app
USER scandrix:scandrix
EXPOSE 8080
ENTRYPOINT ["/app/bin/scandrix-api"]
