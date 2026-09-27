# Instructions

## Creating a Migration

```shell
goose create init_tag sql
```

## Applying All Migrations

```shell
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="user=growscada password=growscada host=localhost dbname=growscada"
goose up
```

## During Debugging

Migrations are applied to the database when starting via the debug `docker-compose.yaml`.
