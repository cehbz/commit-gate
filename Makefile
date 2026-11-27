BUILD := build

.PHONY: build test accept clean release deploy

build:
	@mkdir -p $(BUILD)/hooks $(BUILD)/tests
	go build -o $(BUILD)/commit-gate ./cmd/cg
	@for h in $$($(BUILD)/commit-gate hook-names); do ln -sf ../commit-gate $(BUILD)/hooks/$$h; done
	@ln -sf commit-gate $(BUILD)/precheck
	@ln -sf commit-gate $(BUILD)/sessioncheck
	@for f in tests/*; do ln -sf "../../$$f" "$(BUILD)/$$f"; done

test:
	go test ./...

accept: build
	bash $(BUILD)/tests/run-all.sh

clean:
	rm -rf $(BUILD)

# make release VERSION=<tag> (DESIGN.md, Release).
release:
	@go run ./cmd/release "$(VERSION)"

# make deploy [HOST=<host>] (DESIGN.md, Deploy): without HOST, every host in
# deploy.hosts. The deploy authorization is checked before Go runs, since the
# go command writes under the home directory and a refused deploy changes
# nothing; cmd/deploy consumes it.
deploy:
	@test -e "$$(git rev-parse --git-common-dir)/commit-gate/deploy-token" || \
	  { echo "make deploy: no deploy authorization in this clone; the user runs commit-gate approve-deploy here first" >&2; exit 1; }
	@go run ./cmd/deploy $(HOST)
