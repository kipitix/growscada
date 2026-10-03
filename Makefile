install_tools:
	go install github.com/pressly/goose/v3/cmd/goose@latest

build: build_server build_growctl build_simulator

build_server:
	GOARCH=wasm GOOS=js go build -o bin/growscada_combined_server/web/app.wasm cmd/combined_server/main.go
	go build -o bin/growscada_combined_server/growscada_combined_server cmd/combined_server/main.go

build_growctl:
	go build -o bin/growctl/growctl cmd/growctl/main.go

build_simulator:
	go build -o bin/growscada_device_simulator/growscada_device_simulator cmd/device_simulator/main.go

run_simulator: build_simulator
	bin/growscada_device_simulator/growscada_device_simulator --config tests/device_simulator/example.yaml

run_simulator_dashboard: build_simulator
	bin/growscada_device_simulator/growscada_device_simulator --config tests/device_simulator/seed_dashboard.yaml

run: build_server
	chromium --incognito --start-maximized --disable-background-networking http://localhost:8080 &
	cd bin/growscada_combined_server && ./growscada_combined_server

db_up:
	cd tools/debug_db && docker compose up -d

db_down:
	cd tools/debug_db && docker compose down
	docker volume rm debug_db_growscada_data

test:
	go test --cover ./...

bench:
	go test -run=^$ -bench=. -benchmem ./...

# Regenerates the committed JSON schemas (schemas/<contract>/<MAJOR.MINOR>.json)
# from the contract types. Run it after raising a contract's SchemaVersion.
.PHONY: schemas
schemas:
	go test ./contract/... -run TestSchemaIsCommitted -update

full_restart:
	cd tools/debug_db && docker compose down || true
	docker volume rm debug_db_growscada_data || true
	$(MAKE) db_up run
