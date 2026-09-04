package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/raphaieu/vorcarozap/migrations"
)

// Migrate aplica todas as migrations pendentes no banco de dados.
func Migrate(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("store: falha ao configurar dialeto goose: %w", err)
	}

	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("store: falha ao executar migrations: %w", err)
	}

	return nil
}

// CheckReady valida a conectividade com o banco e garante que todas as migrations
// embutidas estão aplicadas. Retorna erro caso o banco esteja inacessível ou haja
// migrations pendentes.
func CheckReady(ctx context.Context, db *sql.DB) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// 1. Verifica conexão efetiva
	if err := db.PingContext(timeoutCtx); err != nil {
		return fmt.Errorf("store: ping falhou: %w", err)
	}

	// 2. Verifica se todas as migrations estão aplicadas
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("store: dialeto goose inválido: %w", err)
	}

	currentVersion, err := goose.GetDBVersion(db)
	if err != nil {
		return fmt.Errorf("store: falha ao consultar versão do banco: %w", err)
	}

	migrationList, err := goose.CollectMigrations(".", 0, goose.MaxVersion)
	if err != nil {
		return fmt.Errorf("store: falha ao listar migrations embutidas: %w", err)
	}

	if len(migrationList) == 0 {
		return nil
	}

	lastMigration := migrationList[len(migrationList)-1]
	if currentVersion < lastMigration.Version {
		return fmt.Errorf("store: migrations pendentes (versão banco=%d, esperada=%d)", currentVersion, lastMigration.Version)
	}

	return nil
}
