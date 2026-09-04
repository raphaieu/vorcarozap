package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "serve":
		if err := runServe(); err != nil {
			slog.Error("erro ao executar comando serve", "error", err)
			os.Exit(1)
		}
	case "migrate":
		if err := runMigrate(); err != nil {
			slog.Error("erro ao executar comando migrate", "error", err)
			os.Exit(1)
		}
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Erro: comando desconhecido %q.\n\n", command)
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, `VorcaroZAP — Plataforma documental e investigativa

Uso:
  vorcarozap <comando> [opções]

Comandos disponíveis nesta fase:
  serve      Aplica migrations pendentes e inicia o servidor HTTP
  migrate    Aplica migrations pendentes no banco SQLite
  help       Exibe esta mensagem de ajuda

Configuração via variáveis de ambiente:
  APP_PORT          Porta HTTP do servidor (padrão: 8080)
  APP_ENV           Ambiente de execução (padrão: development)
  DB_PATH           Caminho do banco SQLite (padrão: ./data/vorcarozap.db)
  APP_READ_TIMEOUT  Timeout de leitura HTTP (padrão: 5s)
  APP_WRITE_TIMEOUT Timeout de escrita HTTP (padrão: 10s)
  APP_IDLE_TIMEOUT  Timeout de conexões ociosas (padrão: 60s)
`)
}

func runServe() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("carregamento de configuração: %w", err)
	}

	ctx := context.Background()

	// 1. Abre o banco de dados e valida pragmas
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("inicialização do banco SQLite: %w", err)
	}
	defer db.Close()

	// 2. Aplica migrations pendentes antes de iniciar o servidor HTTP
	slog.Info("verificando e aplicando migrations pendentes...")
	if err := store.Migrate(ctx, db); err != nil {
		return fmt.Errorf("aplicação de migrations antes de iniciar o servidor: %w", err)
	}
	slog.Info("migrations aplicadas com sucesso")

	// 3. Inicializa o servidor HTTP
	server, err := web.NewServer(cfg, db)
	if err != nil {
		return fmt.Errorf("criação do servidor HTTP: %w", err)
	}

	// 4. Inicia escuta em goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("servidor HTTP ativo", "addr", server.Addr, "env", cfg.Env)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// 5. Encerramento gracioso sob sinais do sistema operacional
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("servidor encerrou inesperadamente: %w", err)
	case sig := <-stopChan:
		slog.Info("sinal de término recebido, iniciando encerramento gracioso...", "signal", sig.String())
	}

	// Timeout de shutdown gracioso
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("falha durante encerramento gracioso do servidor: %w", err)
	}

	slog.Info("servidor encerrado graciosamente")
	return nil
}

func runMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("carregamento de configuração: %w", err)
	}

	ctx := context.Background()

	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("abertura do banco SQLite: %w", err)
	}
	defer db.Close()

	slog.Info("executando migrations...", "db_path", cfg.DBPath)
	if err := store.Migrate(ctx, db); err != nil {
		return fmt.Errorf("execução de migrations: %w", err)
	}

	slog.Info("migrations concluídas com sucesso")
	return nil
}
