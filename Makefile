
default: fmt build

build:
	@go build --tags nowayland -o mdedit cmd/mdedit/main.go

fmt:
	@goimports -w -l .
	@gofumpt -w -l .

lbedit: fmt
	@go build --tags nowayland -o lbedit cmd/lbedit/main.go

with-wayland: fmt
	@go build -o mdedit cmd/mdedit/main.go
