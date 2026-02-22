---
name: db-migrate
description: "Create, apply, and manage SQLite database migrations for the dejli project. Use when adding/altering tables, creating new domains, running db alter/db migrate/db reset commands, or writing migration SQL files."
---

# Skill: db-migrate

## Context

- Database package: `database/`
- Migrations live in: `database/migrations/<dbname>/`
- Naming: `<YYYYMMDDHHmmss>__<description>.sql`
- Go migration engine: `database/lib/migrate.go`
- CLI tool: `db` (available in all devenv shells)
- Migrations are applied via CLI, not at Lambda startup

## Creating a New Migration

From any devenv shell with `db` available:

```bash
db alter <dbname> <description>
# e.g. db alter waitlist add_referral_source
# Creates: database/migrations/waitlist/20260208151500__add_referral_source.sql
```

Then write the SQL in the created file. Use standard SQLite DDL:

```sql
ALTER TABLE waitlist ADD COLUMN referral_source TEXT;
```

For new tables, use `CREATE TABLE IF NOT EXISTS` for safety.

If this is a new domain (new database), also create the domain package in the consuming service (e.g., `backend/api/internal/<dbname>/`).

## Applying Migrations

```bash
db migrate <dbname>     # Apply pending migrations
db reset <dbname>       # Drop DB and reapply all migrations
```

For production (S3-backed):

```bash
DB_S3_BUCKET=my-bucket DB_LOCK_TABLE=dejli-db-locks db migrate <dbname>
```

## Verifying

```bash
db reset <dbname>         # Full chain applies cleanly
db migrations <dbname>    # Check applied
db query <dbname> "SELECT * FROM <table>"  # Inspect data
```

## Rules

- **Forward-only** — never edit an already-applied migration, create a new one
- **One concern per file** — each migration does one thing
- **Idempotent initial migrations** — use `IF NOT EXISTS` for the first CREATE TABLE
- **Test after creating** — run `db reset <dbname>` to verify the full chain applies cleanly
