SERVICE_DIR := $(HOME)/.config/systemd/user
SERVICE     := efferent.service

# Stamped into the binary (main.go's buildInfo) and logged once at startup,
# so "is the running binary current" is answered by a log line.
GIT_SHA     := $(shell git rev-parse --short HEAD 2>/dev/null || echo nogit)
GIT_DIRTY   := $(shell git diff --quiet 2>/dev/null || echo -dirty)
BUILD_INFO  := $(GIT_SHA)$(GIT_DIRTY)-$(shell date -u +%Y%m%dT%H%M%SZ)
LDFLAGS     := -X main.buildInfo=$(BUILD_INFO)

.PHONY: build install run service-install service-start service-stop service-restart service-status service-logs service-uninstall

build:
	go build -ldflags "$(LDFLAGS)" -o efferent .
	go build -o agenthook/agenthook ./agenthook

install:
	go build -ldflags "$(LDFLAGS)" -o $(HOME)/go/bin/efferent .
	go build -o $(HOME)/go/bin/agenthook ./agenthook

run:
	go run -ldflags "$(LDFLAGS)" .

# service-install renders the unit (substituting @GOBIN@ with this shell's
# actual `go`, since systemd --user's own PATH doesn't include a per-version
# Go SDK install, and @SRCDIR@ with this checkout) and enables it -- starts
# on next login/boot, not immediately; run `make service-start` too to start
# now. Needs `loginctl enable-linger $(USER)` once, separately, if efferent
# should keep running after you log out (see README.md).
#
# The unit's own ExecStartPre rebuilds the binary from this checkout on
# every start/restart, so no start path (systemctl directly, a reboot, this
# Makefile) can run stale code.
service-install: install
	mkdir -p $(SERVICE_DIR)
	sed -e "s|@GOBIN@|$$(command -v go)|" -e "s|@SRCDIR@|$(CURDIR)|" deploy/systemd/$(SERVICE) > $(SERVICE_DIR)/$(SERVICE)
	systemctl --user daemon-reload
	systemctl --user enable $(SERVICE)

service-start:
	systemctl --user start $(SERVICE)

service-stop:
	systemctl --user stop $(SERVICE)

service-restart: install
	systemctl --user restart $(SERVICE)

service-status:
	systemctl --user status $(SERVICE)

service-logs:
	journalctl --user -u $(SERVICE) -f

service-uninstall:
	-systemctl --user disable --now $(SERVICE)
	rm -f $(SERVICE_DIR)/$(SERVICE)
	systemctl --user daemon-reload
