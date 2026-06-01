FROM golang:1.24-bookworm AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd/gateway

FROM debian:bookworm-slim

RUN useradd -r -u 10001 gateway && mkdir -p /data && chown gateway:gateway /data
USER gateway
WORKDIR /app
COPY --from=build /out/gateway /app/gateway
EXPOSE 8080
ENTRYPOINT ["/app/gateway"]
