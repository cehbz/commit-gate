BUILD := build

.PHONY: build test accept clean

build:
	@mkdir -p $(BUILD)/bin $(BUILD)/hooks $(BUILD)/lib $(BUILD)/tests
	go build -o $(BUILD)/commit-gate ./cmd/cg
	@ln -sf ../commit-gate $(BUILD)/bin/approve
	@ln -sf ../commit-gate $(BUILD)/bin/approve-push
	@ln -sf ../commit-gate $(BUILD)/bin/clear-approvals
	@ln -sf ../commit-gate $(BUILD)/bin/gate-disable
	@ln -sf ../commit-gate $(BUILD)/bin/gate-enable
	@ln -sf ../commit-gate $(BUILD)/bin/gate-status
	@ln -sf ../commit-gate $(BUILD)/hooks/commit-msg
	@ln -sf ../commit-gate $(BUILD)/hooks/pre-push
	@ln -sf ../commit-gate $(BUILD)/lib/canonical
	@ln -sf commit-gate $(BUILD)/precheck
	@ln -sf commit-gate $(BUILD)/sessioncheck
	@for f in tests/*; do ln -sf "../../$$f" "$(BUILD)/$$f"; done

test:
	go test ./...

accept: build
	bash $(BUILD)/tests/run-all.sh

clean:
	rm -rf $(BUILD)
