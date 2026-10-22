# GrowSCADA images, built from the repository root:
#   docker build --target server .      the combined server: API :9090, PWA :8080
#   docker build --target simulator .   the device simulator: control API :9191
# tests/api/docker-compose.yaml runs both for `make bruno_test`.

FROM golang:1.26.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# As `make build_server`: app.wasm for the browser, then the server binary.
RUN GOARCH=wasm GOOS=js go build -o /out/server/web/app.wasm ./cmd/combined_server \
 && CGO_ENABLED=0 go build -o /out/server/growscada_combined_server ./cmd/combined_server \
 && CGO_ENABLED=0 go build -o /out/simulator/growscada_device_simulator ./cmd/device_simulator

# Alpine rather than distroless: its wget serves the compose healthchecks.
FROM alpine:3.22 AS simulator
COPY --from=build /out/simulator/ /app/
WORKDIR /app
EXPOSE 9191
ENTRYPOINT ["/app/growscada_device_simulator"]

FROM alpine:3.22 AS server
COPY --from=build /out/server/ /app/
# The PWA is served from web/ next to the working directory.
WORKDIR /app
EXPOSE 8080 9090
ENTRYPOINT ["/app/growscada_combined_server"]
