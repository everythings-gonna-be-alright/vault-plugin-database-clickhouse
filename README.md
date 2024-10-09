# vault-plugin-database-clickhouse
[![Stand With Ukraine](https://raw.githubusercontent.com/vshymanskyy/StandWithUkraine/main/banner2-direct.svg)](https://vshymanskyy.github.io/StandWithUkraine/)

![Diagram](diagram.svg)

Plugin for generating clickhouse user credentials from Vault.

Supports single-node clickhouse and cluster versions.

## Example of statement for user creation in vault configuration:
``` yaml
resource "vault_database_secret_backend_role" "clickhouse_analytics" {
  backend = vault_mount.clickhouse.path
  name    = "clickhouse_analytics"
  db_name = "my_clickhouse"
  creation_statements = ["CREATE USER '{{name}}' IDENTIFIED WITH sha256_password BY '{{password}}' ON CLUSTER '{{cluster}}';",
  "GRANT ON CLUSTER '{{cluster}}' analytics TO '{{name}}';"]
  default_ttl = 2593000
  max_ttl     = 2593000
}
```


## How to build it:
``` bash
make build-linux-amd64
make build-linux-arm64
make build-darwin-arm64
```