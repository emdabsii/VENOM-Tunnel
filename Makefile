.PHONY: all build-c build-rust build-web build-go clean

DIST := dist

all: build-c build-rust build-web build-go

build-c:
	mkdir -p $(DIST)
	$(MAKE) -C sysopt
	cp sysopt/sysopt $(DIST)/venom-sysopt

build-rust:
	mkdir -p $(DIST)
	cd netprobe && cargo build --release
	cp netprobe/target/release/venom-netprobe $(DIST)/venom-netprobe

build-web:
	cd web && npm ci && npm run build
	cp -r web/dist/* backend/webdist/

build-go:
	mkdir -p $(DIST)
	cd backend && CGO_ENABLED=0 go build -ldflags="-s -w" -o ../$(DIST)/venomd .

clean:
	rm -rf $(DIST) sysopt/sysopt netprobe/target web/dist
