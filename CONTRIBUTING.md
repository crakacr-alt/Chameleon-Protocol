# Contributing

Thanks for helping improve Chameleon Protocol.

## Development workflow

1. fork the repository
2. create a feature branch
3. make your changes
4. run `gofmt -w ./...`
5. run `go test ./...`
6. open a pull request with a concise description

## Code style

- prefer small, idiomatic Go code
- keep the transport logic readable and explicit
- use `if err != nil` error handling consistently
- add regression tests for protocol behavior changes
