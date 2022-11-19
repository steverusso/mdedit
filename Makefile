
default: build

build: fmt lint
	@go build --tags nowayland -o mdedit cmd/mdedit/main.go

fmt:
	@goimports -w -l .
	@gofumpt -w -l .

lint:
	@go vet ./...

release: fmt lint
	@go build -ldflags="-s -w" --tags nowayland -o mdedit cmd/mdedit/main.go

wayland: fmt lint
	@go build -o mdedit cmd/mdedit/main.go
