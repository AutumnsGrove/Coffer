# Coffer. Everything runs in Docker (DEVELOPMENT.md section 7); `make` only wraps the commands.
# Quick iteration on one package with a local `go test ./internal/x` is fine, but it never counts
# as a gate: the container is the canonical environment.

COMPOSE := docker compose -f compose.test.yml
RUN     := $(COMPOSE) run --rm --build
VERSION ?= dev

.PHONY: help test bad sad fuzz fuzz-smoke vuln image smoke clean

help:
	@echo "make test                         vet, staticcheck, race detector, coverage"
	@echo "make bad | make sad               only the bad or sad paths"
	@echo "make fuzz P=./pkg T=FuzzName      one fuzz target (FUZZTIME=30s)"
	@echo "make fuzz-smoke                   every fuzz target, briefly"
	@echo "make vuln                         known-vulnerability scan (needs network)"
	@echo "make image [VERSION=r1.abc1234]   build the production image"
	@echo "make smoke                        run the built image with --version"

test:
	$(RUN) test

bad:
	$(RUN) bad

sad:
	$(RUN) sad

fuzz:
	@test -n "$(P)" -a -n "$(T)" || { echo "usage: make fuzz P=./internal/x T=FuzzName"; exit 2; }
	$(RUN) fuzz

fuzz-smoke:
	$(RUN) fuzz-smoke

vuln:
	$(RUN) vuln

image:
	docker build --target final --build-arg VERSION=$(VERSION) -t coffer:$(VERSION) -t coffer:latest .

smoke: image
	docker run --rm coffer:latest --version

clean:
	$(COMPOSE) down --volumes --remove-orphans
