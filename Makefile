APP := sysdash
BUILD_DIR := bin
GOCACHE := $(CURDIR)/.cache/go-build
GOMODCACHE := $(CURDIR)/.cache/go-mod
GOENV := GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE)

.PHONY: build tidy run clean

build:
	mkdir -p $(BUILD_DIR) $(GOCACHE) $(GOMODCACHE)
	env CGO_ENABLED=0 $(GOENV) go build -o $(BUILD_DIR)/$(APP) .

tidy:
	mkdir -p $(GOCACHE) $(GOMODCACHE)
	env $(GOENV) go mod tidy

run:
	mkdir -p $(GOCACHE) $(GOMODCACHE)
	env CGO_ENABLED=0 $(GOENV) go run .

clean:
	rm -rf $(BUILD_DIR)
