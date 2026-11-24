BINARY := valkyrie-driver
PREFIX ?= $(HOME)/.local
BINDIR := $(PREFIX)/bin
APPDIR := $(PREFIX)/share/applications
ICONDIR := $(PREFIX)/share/icons/hicolor/256x256/apps

.PHONY: all build test test-live install install-udev uninstall clean

all: build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) .

test:
	go test -v -count=1 ./...

test-live:
	VK_LIVE_HID=1 go test -v -count=1 ./hid -run TestLiveIdempotentBTWrite

install: build
	install -Dm755 $(BINARY) "$(BINDIR)/$(BINARY)"
	install -Dm644 assets/icon.png "$(ICONDIR)/$(BINARY).png"
	sed "s|@EXEC@|$(BINDIR)/$(BINARY)|g; s|@ICON@|$(ICONDIR)/$(BINARY).png|g" \
		packaging/valkyrie-driver.desktop > "$(APPDIR)/valkyrie-driver.desktop"
	@echo "Installed $(BINDIR)/$(BINARY)"

install-udev:
	sudo install -Dm644 packaging/99-valkyrie-hid.rules /etc/udev/rules.d/99-valkyrie-hid.rules
	sudo udevadm control --reload-rules
	sudo udevadm trigger --subsystem-match=hidraw --subsystem-match=usb
	@echo "Installed udev rules; reconnect or wake your mouse if needed."

uninstall:
	rm -f "$(BINDIR)/$(BINARY)" "$(ICONDIR)/$(BINARY).png" "$(APPDIR)/valkyrie-driver.desktop"

clean:
	rm -f $(BINARY) $(BINARY).new
