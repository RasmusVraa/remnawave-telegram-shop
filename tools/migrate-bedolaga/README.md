# migrate-bedolaga

Go-движок миграции Bedolaga → этот shop. Для админов предпочтителен wizard:

```bash
./scripts/meows-bedolaga-migrate.sh
```

Документация: [`documentation/bedolaga-migration.md`](../../documentation/bedolaga-migration.md).

```bash
go run ./tools/migrate-bedolaga -write-example-config
go run ./tools/migrate-bedolaga -config migrate.yaml -dry-run -step all
go run ./tools/migrate-bedolaga -config migrate.yaml -apply -step tariffs
```

Бекап из админки Bedolaga (`backup_*.tar.gz` с `database.json`) разворачивается во временный Postgres:

```bash
go run ./tools/migrate-bedolaga -load-backup backup_2026.tar.gz -load-dsn "postgres://postgres:pass@host:5432/restore"
```

Шаги: `tariffs`, `customers`, `balance`, `referrals`, `history` (оплаты подписок → история покупок).

Тесты формулы срока:

```bash
go test ./tools/migrate-bedolaga/ -count=1
```
