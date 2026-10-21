.DEFAULT_GOAL := install

.PHONY: install fmt test vet-linux vet-darwin

install:
	go install ./cmd/perusal

fmt:
	gofmt -w cmd internal

# follow mode is Windows only, so the gate vets the other two platforms' build tags too.
test: vet-linux vet-darwin
	go vet ./...
	go test ./... -count=1

vet-linux: export GOOS := linux
vet-linux:
	go vet ./...

vet-darwin: export GOOS := darwin
vet-darwin:
	go vet ./...
