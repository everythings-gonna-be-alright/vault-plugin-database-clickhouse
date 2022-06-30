# Determine this makefile's path.
# Be sure to place this BEFORE `include` directives, if any.

GO_VERSION_MIN=1.17.11

default: build-linux-arm64

build-linux-amd64:
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/linux-amd64/clickhouse-database-plugin ./clickhouse-database-plugin

build-linux-arm64:
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/linux-arm64/clickhouse-database-plugin ./clickhouse-database-plugin

build-darwin-arm64:
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/darwin-arm64/clickhouse-database-plugin ./clickhouse-database-plugin