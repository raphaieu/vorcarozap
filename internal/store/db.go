package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Open abre o banco SQLite, configura o pool e valida os pragmas obrigatórios.
//
// POLÍTICA DE CONEXÕES:
// Para a Fase 1, adota-se política estritamente conservadora de pool (MaxOpenConns = 1, MaxIdleConns = 1).
// Em um monólito SQLite com WAL e uma única instância, essa configuração elimina contenção
// e bloqueios concorrentes de escrita (SQLITE_BUSY), garantindo previsibilidade operacional.
//
// PRAGMAS POR CONEXÃO:
// Os pragmas 'foreign_keys', 'busy_timeout', 'journal_mode' e 'synchronous' são injetados
// diretamente na DSN do modernc.org/sqlite usando parâmetros '_pragma'. Isso garante que
// qualquer conexão criada pelo pool receba os pragmas necessários automaticamente.
func Open(ctx context.Context, dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("store: caminho do banco de dados não informado")
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("store: falha ao criar diretório do banco %q: %w", dir, err)
	}

	// Monta a DSN com pragmas por conexão via modernc.org/sqlite
	// Sintaxe: file:caminho?_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&...
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")

	dsn := fmt.Sprintf("file:%s?%s", filepath.ToSlash(dbPath), q.Encode())

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao abrir SQLite: %w", err)
	}

	// Política conservadora de pool
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Valida os pragmas em uma conexão efetiva
	if err := validatePragmas(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: validação de pragmas falhou: %w", err)
	}

	return db, nil
}

func validatePragmas(ctx context.Context, db *sql.DB) error {
	vCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	conn, err := db.Conn(vCtx)
	if err != nil {
		return fmt.Errorf("falha ao obter conexão para validação: %w", err)
	}
	defer conn.Close()

	// 1. journal_mode
	var journalMode string
	if err := conn.QueryRowContext(vCtx, "PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		return fmt.Errorf("falha ao consultar PRAGMA journal_mode: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf("journal_mode esperado 'wal', obtido %q", journalMode)
	}

	// 2. foreign_keys
	var foreignKeys int
	if err := conn.QueryRowContext(vCtx, "PRAGMA foreign_keys;").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("falha ao consultar PRAGMA foreign_keys: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("foreign_keys esperado 1 (ON), obtido %d", foreignKeys)
	}

	// 3. busy_timeout
	var busyTimeout int
	if err := conn.QueryRowContext(vCtx, "PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		return fmt.Errorf("falha ao consultar PRAGMA busy_timeout: %w", err)
	}
	if busyTimeout < 5000 {
		return fmt.Errorf("busy_timeout esperado >= 5000, obtido %d", busyTimeout)
	}

	// 4. synchronous (0=OFF, 1=NORMAL, 2=FULL, 3=EXTRA)
	var synchronous int
	if err := conn.QueryRowContext(vCtx, "PRAGMA synchronous;").Scan(&synchronous); err != nil {
		return fmt.Errorf("falha ao consultar PRAGMA synchronous: %w", err)
	}
	if synchronous != 1 {
		return fmt.Errorf("synchronous esperado 1 (NORMAL), obtido %d", synchronous)
	}

	return nil
}
