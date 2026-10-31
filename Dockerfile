# syntax=docker/dockerfile:1

FROM golang:1.26.4-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/glucose-dashboard ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache wget ca-certificates \
    && adduser -D -u 65532 app \
    && mkdir -p /data \
    && chown app:app /data
USER app
COPY --from=build /out/glucose-dashboard /usr/local/bin/glucose-dashboard
ENV LISTEN_ADDR=:8080 \
    DB_PATH=/data/glucose.db
EXPOSE 8080
ENTRYPOINT ["glucose-dashboard"]
