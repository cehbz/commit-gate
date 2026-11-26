BUILD := build

.PHONY: build test accept clean

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
