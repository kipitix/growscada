install_tools:
	go install github.com/pressly/goose/v3/cmd/goose@latest

build: build_server build_growctl build_simulator

build_server:
	GOARCH=wasm GOOS=js go build -o bin/growscada_combined_server/web/app.wasm cmd/combined_server/main.go
	cp cmd/combined_server/web/icon-192.png cmd/combined_server/web/icon-512.png bin/growscada_combined_server/web/
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
	go test -race --cover ./...

# Runs the Bruno collections against the application built in containers
# (tests/api/docker-compose.yaml: fresh database with seed, server, simulator).
# The containers are removed whatever the outcome; the exit code is bru's.
BRUNO_CLI := @usebruno/cli@4.2.1
BRUNO_COMPOSE := docker compose -f tests/api/docker-compose.yaml
BRUNO_RUN := npx --yes $(BRUNO_CLI) run --env localhost --exclude-tags=manual

.PHONY: bruno_test
bruno_test:
	@$(BRUNO_COMPOSE) up --build --wait; status=$$?; \
	if [ $$status -ne 0 ]; then $(BRUNO_COMPOSE) logs; fi; \
	if [ $$status -eq 0 ]; then \
		(cd tests/api/bruno_collections/growscada_server && $(BRUNO_RUN)) || status=1; \
		(cd tests/api/bruno_collections/device_simulator && $(BRUNO_RUN)) || status=1; \
	fi; \
	$(BRUNO_COMPOSE) down -v; exit $$status

bench:
	go test -run=^$ -bench=. -benchmem ./...

# Regenerates the committed JSON schemas (schemas/<contract>/<MAJOR.MINOR>.json)
# from the contract types. Run it after raising a contract's SchemaVersion.
# Refuses to rewrite a version already on the branch `path`, freezes older
# versions in schemas/<contract>/frozen.sha256 and, while MAJOR is 0, notes
# what in a new MINOR would have needed a MAJOR after 1.0.
.PHONY: schemas
schemas:
	@out=$$(go test ./contract/... -run TestSchemaIsCommitted -update -v 2>&1); status=$$?; \
	printf '%s\n' "$$out" | grep -vE '^(=== RUN|--- PASS|PASS$$|ok |testing: warning: no tests to run)'; exit $$status

full_restart:
	cd tools/debug_db && docker compose down || true
	docker volume rm debug_db_growscada_data || true
	$(MAKE) db_up run
