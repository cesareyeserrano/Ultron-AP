# syntax=docker/dockerfile:1

FROM golang:1.25.7-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /out/ultron-ap ./cmd/ultron-ap

FROM alpine:3.21
RUN addgroup -S ultron && adduser -S -G ultron ultron
WORKDIR /app
COPY --from=build /out/ultron-ap /app/ultron-ap
USER ultron:ultron
EXPOSE 8080
ENV ULTRON_PORT=8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD wget -qO- "http://127.0.0.1:${ULTRON_PORT}/health" | grep -q '"status":"ok"'
ENTRYPOINT ["/app/ultron-ap"]
