.PHONY: check fmt vet test race cover fuzz bench sim

check: fmt vet test

fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "Ejecuta: gofmt -w ." && exit 1)

vet:
	go vet ./...

test:
	go test ./... -cover

race:
	go test -race ./...

cover:
	go test ./internal/game -coverprofile=coverage.out
	go tool cover -func=coverage.out

fuzz:
	go test ./internal/game -run '^$$' -fuzz FuzzValidateMeld -fuzztime 30s

bench:
	go test ./internal/game -run '^$$' -bench . -benchmem

sim:
	go run ./cmd/simulate -games 300 -disconnect 0.05
