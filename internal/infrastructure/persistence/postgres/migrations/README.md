# Порядок действий

## Создание миграции

```shell
goose create init_tag sql
```

## Применение всех миграций

```shell
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="user=growscada password=growscada host=localhost dbname=growscada"
goose up
```

## При отладке

Миграции применяются к базе при запуске через отладочный `docker-compose.yaml`.
