# syntax=docker/dockerfile:1.7
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.22 AS runtime
ARG REVISION=unknown
LABEL org.opencontainers.image.revision=$REVISION
RUN apk add --no-cache ca-certificates wget && addgroup -S -g 10001 scheduler && adduser -S -u 10001 -G scheduler scheduler
COPY --from=builder /out/server /usr/local/bin/server
COPY --from=builder /out/migrate /usr/local/bin/migrate
USER scheduler
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server"]
