# vault-plugin-database-clickhouse
[![Stand With Ukraine](https://raw.githubusercontent.com/vshymanskyy/StandWithUkraine/main/banner2-direct.svg)](https://vshymanskyy.github.io/StandWithUkraine/)

![Diagram](diagram.svg)

Plugin for generating clickhouse user credentials from Vault.

Supports single-node clickhouse and cluster versions.

## Example of statement for user creation in vault configuration ( terraform ):
``` hcl
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

## How to install ( Kubernetes ):

1) Add plugin directory to the vault config
    ``` YAML
    server:
      # standalone or ha.raft
      standalone:
        config: |
          plugin_directory = "/usr/local/libexec/vault/"
    ```

2) Download and extract plugin
    ``` YAML
    server: 
      volumes:
        - name: plugins
          emptyDir: {}
  
      volumeMounts:
        - mountPath: /usr/local/libexec/vault
          name: plugins
          readOnly: true
  
      extraInitContainers:
          - name: clickhouse
            image: "alpine"
            command: [sh, -c]
            args:
             - PLUGIN_PLATFORM='linux-amd64' &&
               PLUGIN_VERSION=`wget -qO- "https://api.github.com/repos/everythings-gonna-be-alright/vault-plugin-database-clickhouse/releases/latest" | grep '"tag_name"' | cut -d '"' -f 4 ` &&
               wget https://github.com/everythings-gonna-be-alright/vault-plugin-database-clickhouse/releases/download/${PLUGIN_VERSION}/clickhouse-database-plugin-${PLUGIN_PLATFORM}.zip -O clickhouse.zip &&
               unzip clickhouse.zip &&
               mv ${PLUGIN_PLATFORM}/clickhouse-database-plugin /usr/local/libexec/vault/clickhouse-database-plugin &&
               rm clickhouse.zip &&
               chmod +x /usr/local/libexec/vault/clickhouse-database-plugin
            volumeMounts:
              - name: plugins
                mountPath: /usr/local/libexec/vault
    ```

3) Allow plugin ( Execute into vault pod shell )
    ``` bash
    cd /usr/local/libexec/vault/
    
    #calculate sha256 sum of plugin file
    sha256sum clickhouse-database-plugin
    
    #login to vault as admin
    vault login
    # --enter your token---
    
    #register the plugin
    vault plugin register -sha256=//sha256sum calculated earlier// database clickhouse-database-plugin
    ```

## How to build it:
``` bash
make build-linux-amd64
make build-linux-arm64
make build-darwin-arm64
```


## Advertising

You can aso check my another project for Clickhouse. High-performance HTTPS Clickhouse connector:

https://github.com/everythings-gonna-be-alright/amazing-clickhouse-connector