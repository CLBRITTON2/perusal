.DEFAULT_GOAL := install

.PHONY: install fmt test

install:
	go install ./cmd/perusal

fmt:
	gofmt -w cmd internal

# follow mode is Windows only, so the gate vets the other two platforms' build tags too.
test: vet-linux vet-darwin
	go vet ./...
	go test ./... -count=1

vet-%: export GOOS = $*
vet-%:
	go vet ./...
