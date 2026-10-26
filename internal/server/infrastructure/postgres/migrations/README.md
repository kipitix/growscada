# Instructions

One database per server, three schemas, each with its own goose chain
(ADR 0011), applied in this order:

| Directory      | Schema        | goose table                    |
|----------------|---------------|--------------------------------|
| `public/`      | `public`      | `goose_db_version`             |
| `engineering/` | `engineering` | `engineering.goose_db_version` |
| `runtime/`     | `runtime`     | `runtime.goose_db_version`     |

`public` is the server's (organizations, projects, the outbox, policy jobs);
`engineering` and `runtime` reference it, never each other. The public
migration `20261026000000` made the two role schemas and marks the baseline of
their chains. Go code applies the chains with `migrations.Up`.

## Creating a Migration

```shell
goose -dir engineering create add_something sql
```

## Applying All Migrations

```shell
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="user=growscada password=growscada host=localhost dbname=growscada"
goose -dir public -allow-missing up
goose -dir engineering -table engineering.goose_db_version up
goose -dir runtime -table runtime.goose_db_version up
```

## During Debugging

Migrations are applied to the database when starting via the debug `docker-compose.yaml`.
