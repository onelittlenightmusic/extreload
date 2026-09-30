INSTALL_DIR ?= $(HOME)/.local/bin
SKILLS_DIR ?= $(HOME)/.claude/skills
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build install install-skills test clean

build:
	@mkdir -p bin
	go build -ldflags "-X main.version=$(VERSION)" -o bin/extreload .

install: build
	@mkdir -p $(INSTALL_DIR)
	cp bin/extreload $(INSTALL_DIR)/extreload
	@echo "Installed $(INSTALL_DIR)/extreload"
	@echo "Load the helper in Chrome: chrome://extensions → Load unpacked → $(CURDIR)/extension"

# The Claude skill, linked so it follows this repository.
install-skills:
	@mkdir -p $(SKILLS_DIR)
	ln -sfn $(CURDIR)/skills/extreload $(SKILLS_DIR)/extreload
	@echo "Linked $(SKILLS_DIR)/extreload"

test:
	go vet ./...
	go test ./...

clean:
	rm -rf bin
