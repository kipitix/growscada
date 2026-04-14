install_tools:
	go install github.com/pressly/goose/v3/cmd/goose@latest
	go install github.com/dmarkham/enumer@latest

build:
	GOARCH=wasm GOOS=js go build -o bin/growscada_combined/web/app.wasm cmd/combined/main.go
	go build -o bin/growscada_combined/growscada_combined cmd/combined/main.go

run: build
	chromium --incognito http://localhost:8080 &
	cd bin/growscada_combined && ./growscada_combined

db_up:
	cd tools/debug_db && docker compose up -d

db_down:
	cd tools/debug_db && docker compose down
	docker volume rm debug_db_growscada_data