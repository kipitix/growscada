install_tools:
	go install github.com/pressly/goose/v3/cmd/goose@latest

build:
	GOARCH=wasm GOOS=js go build -o bin/growscada_combined_server/web/app.wasm cmd/combined_server/main.go
	go build -o bin/growscada_combined_server/growscada_combined_server cmd/combined_server/main.go

run: build
	chromium --incognito http://localhost:8080 &
	cd bin/growscada_combined_server && ./growscada_combined_server

db_up:
	cd tools/debug_db && docker compose up -d

db_down:
	cd tools/debug_db && docker compose down
	docker volume rm debug_db_growscada_data

test:
	go test ./... -v
