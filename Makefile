# Determine this makefile's path.
# Be sure to place this BEFORE `include` directives, if any.

GO_VERSION_MIN=1.17.11
GO_CMD?=go

default: build

build:
	$(GO_CMD) build -o bin/clickhouse-database-plugin .