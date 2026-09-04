package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/raphaieu/vorcarozap/migrations"
)

// newGooseProvider encapsula uma instância isolada do provedor de migrations Goose
// com as migrations embutidas (migrations.FS) e dialeto SQLite3, evitando alterar
// o estado global do pacote goose.
func newGooseProvider(db *sql.DB) (*goose.Provider, error) {
	p, err := goose.NewProvider(goose.DialectSQLite3, db, migrations.FS)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao inicializar provedor goose: %w", err)
	}
	return p, nil
}

// Migrate aplica todas as migrations pendentes no banco de dados usando o provedor encapsulado.
func Migrate(ctx context.Context, db *sql.DB) error {
	p, err := newGooseProvider(db)
	if err != nil {
		return err
	}

	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("store: falha ao executar migrations: %w", err)
	}

	return nil
}

// CheckReady valida a conectividade com o banco e garante que a versão do banco
// é exatamente compatível com a versão esperada pelo binário.
// Deve falhar quando:
// - existem migrations pendentes (banco atrás do binário);
// - o banco está à frente do binário;
// - não é possível determinar a versão;
// - o contexto expira;
// - o banco não responde.
func CheckReady(ctx context.Context, db *sql.DB) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// 1. Verifica conexão efetiva e expiração de contexto
	if err := db.PingContext(timeoutCtx); err != nil {
		return fmt.Errorf("store: ping falhou: %w", err)
	}

	// 2. Provedor goose encapsulado
	p, err := newGooseProvider(db)
	if err != nil {
		return err
	}

	// 3. Consulta versões atual do banco e esperada pelo binário
	current, target, err := p.GetVersions(timeoutCtx)
	if err != nil {
		return fmt.Errorf("store: falha ao consultar versão do banco: %w", err)
	}

	if current < target {
		return fmt.Errorf("store: migrations pendentes (versão banco=%d, esperada=%d)", current, target)
	}
	if current > target {
		return fmt.Errorf("store: banco à frente do binário (versão banco=%d, esperada=%d)", current, target)
	}

	return nil
}
