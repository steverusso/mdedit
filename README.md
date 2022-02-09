# MdEdit

_MdEdit is a Vi-like markdown editor built using [Gio](https://gioui.org/). It
is extremely early stage software. The Vi editor lacks most functionality and
might be pretty buggy._

## Getting Started

If you have [goimports](https://pkg.go.dev/golang.org/x/tools/cmd/goimports)
and [gofumpt](https://github.com/mvdan/gofumpt) installed, everything can be
formatted and built (without wayland) simply by running `make`.

However, to just build the `mdedit` executable, run:

```sh
go build -o mdedit cmd/mdedit/main.go

# or, to build without wayland:

go build --tags nowayland -o mdedit cmd/mdedit/main.go
```
