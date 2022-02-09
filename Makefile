
default: fmt build

build:
	@go build --tags nowayland -o mdedit cmd/mdedit/main.go

fmt:
	@goimports -w -l .
	@gofumpt -w -l .

with-wayland: fmt
	@go build -o mdedit cmd/mdedit/main.go
