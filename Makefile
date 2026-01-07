.PHONY: build run clean docker-build docker-run

BINARY=echobridge
BACKEND_DIR=backend

build:
	cd $(BACKEND_DIR) && go build -o $(BINARY) ./cmd/server

run: build
	cd $(BACKEND_DIR) && ./$(BINARY)

clean:
	rm -f $(BACKEND_DIR)/$(BINARY)
	rm -rf $(BACKEND_DIR)/data

docker-build:
	docker compose build

docker-run:
	docker compose up

docker-clean:
	docker compose down -v
