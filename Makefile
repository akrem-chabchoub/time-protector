.PHONY: binary test install

BIN := bin/time-protector
CONFIG_DIR := /usr/local/etc/time-protector
VAR_DIR := /usr/local/var/time-protector
PLIST := /Library/LaunchDaemons/com.akrem.time-protector.plist
LABEL := com.akrem.time-protector

binary:
	mkdir -p bin
	go build -o $(BIN) ./cmd/time-protector

test:
	go test ./...

install: binary
	@if [ "$$(id -u)" -ne 0 ]; then echo "run: sudo make install"; exit 1; fi
	install -d -m 755 $(CONFIG_DIR) $(VAR_DIR)
	chown root:wheel $(CONFIG_DIR) $(VAR_DIR)
	chmod 755 $(CONFIG_DIR) $(VAR_DIR)
	install -m 755 $(BIN) /usr/local/bin/time-protector
	@if [ ! -f $(CONFIG_DIR)/config.yaml ]; then \
		$(BIN) write-default-config $(CONFIG_DIR)/config.yaml; \
	fi
	@if [ -f $(CONFIG_DIR)/config.yaml ]; then chown root:wheel $(CONFIG_DIR)/config.yaml; fi
	@if [ ! -f $(VAR_DIR)/hosts.backup ]; then \
		cp /etc/hosts $(VAR_DIR)/hosts.backup; \
		chmod 600 $(VAR_DIR)/hosts.backup; \
	fi
	@if [ -f $(VAR_DIR)/hosts.backup ]; then chown root:wheel $(VAR_DIR)/hosts.backup; fi
	install -m 644 deploy/com.akrem.time-protector.plist $(PLIST)
	launchctl bootout system/$(LABEL) 2>/dev/null || true
	launchctl bootstrap system $(PLIST)
	launchctl enable system/$(LABEL) || true
	@echo "Installed. Chrome secure DNS is a hole: turn off Use secure DNS in Chrome settings, under Privacy and security, then Security."
