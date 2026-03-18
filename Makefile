build:
	GOARCH=wasm GOOS=js go build -o bin/growscada_combined/web/app.wasm cmd/combined/main.go
	go build -o bin/growscada_combined/growscada_combined cmd/combined/main.go

run: build
	chromium http://localhost:8080 &
	cd bin/growscada_combined && ./growscada_combined

start_db:
	cd tools/debug_db && docker compose up -d
