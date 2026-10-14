.PHONY: build test vet coverage clean install setup

BINARY    := tempest-mqtt
CMD       := ./cmd/tempest-mqtt
INSTALL   := /usr/local/bin/$(BINARY)
SERVICE   := /etc/systemd/system/$(BINARY).service
ENVFILE   := /etc/$(BINARY)/$(BINARY).env
LOGIC_PKGS := ./internal/daemon/... ./internal/event/... ./internal/parser/...

## setup: download dependencies and generate go.sum
setup:
	go mod tidy
	go mod download

## build: compile a linux/amd64 binary
build:
	go build -ldflags="-s -w" -o $(BINARY) $(CMD)

## test: run all tests with race detector and coverage
test:
	go test -v -race -coverprofile=coverage.out ./...

## vet: run go vet across all packages
vet:
	go vet ./...

## coverage: show per-function coverage after running tests
coverage: test
	go tool cover -func=coverage.out

## coverage-html: open an HTML coverage report in the browser
coverage-html: test
	go tool cover -html=coverage.out -o coverage.html

## clean: remove build artefacts
clean:
	rm -f $(BINARY) coverage.out coverage.html tempest-mqtt-*

## install: build and install the binary + system service, enabled at boot (requires root)
##          The config is installed from deploy/tempest-mqtt.env.example only if there is none.
install: build
	install -m 755 $(BINARY) $(INSTALL)
	install -m 644 deploy/tempest-mqtt.service $(SERVICE)
	@if [ -f $(ENVFILE) ]; then echo "Keeping existing $(ENVFILE)"; \
	else install -d $(dir $(ENVFILE)) && install -m 640 deploy/tempest-mqtt.env.example $(ENVFILE) \
	  && echo "Installed $(ENVFILE) from the example"; fi
	systemctl daemon-reload
	systemctl enable $(BINARY)
	@echo "Service installed and enabled at boot. Edit $(ENVFILE) then: systemctl restart $(BINARY)"

## uninstall: stop and remove the binary + systemd service (the config is kept)
uninstall:
	-systemctl stop $(BINARY)
	-systemctl disable $(BINARY)
	rm -f $(INSTALL) $(SERVICE)
	systemctl daemon-reload

## cross-build: compile binaries for linux amd64, arm64, and armv6 (Raspberry Pi)
cross-build: clean
	GOOS=linux GOARCH=amd64 \
		go build -ldflags="-s -w" -o $(BINARY)-linux-amd64 $(CMD)
	GOOS=linux GOARCH=arm64 \
		go build -ldflags="-s -w" -o $(BINARY)-linux-arm64 $(CMD)
	GOOS=linux GOARCH=arm GOARM=6 \
		go build -ldflags="-s -w" -o $(BINARY)-linux-armv6 $(CMD)
	@ls -lh $(BINARY)-linux-*
