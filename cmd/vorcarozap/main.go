package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/exporter"
	"github.com/raphaieu/vorcarozap/internal/importer"
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
	case "import":
		if err := runImport(os.Args[2:]); err != nil {
			slog.Error("erro ao executar comando import", "error", err)
			os.Exit(1)
		}
	case "export":
		if err := runExport(os.Args[2:]); err != nil {
			slog.Error("erro ao executar comando export", "error", err)
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
  import     Importa dados da planilha XLSX curated_seed (--file obrigatório, opcional --dry-run)
  export     Exporta a base pública ativa em planilha XLSX (--out opcional)
  help       Exibe esta mensagem de ajuda

Configuração via variáveis de ambiente:
  APP_PORT          Porta HTTP do servidor (padrão: 8080)
  APP_ENV           Ambiente de execução (padrão: development)
  DB_PATH           Caminho do banco SQLite (padrão: ./data/vorcarozap.db)
  MAPPING_PATH      Caminho do YAML de mapping (padrão: config/import-mapping-v1.yaml)
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

func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Uso do comando import:
  vorcarozap import --file <caminho.xlsx> [--dry-run]

Opções:
  --file string   Caminho do arquivo XLSX da planilha curated_seed (obrigatório)
  --dry-run       Analisa a planilha sem persistir alterações no banco de dados
  -h, --help      Exibe esta ajuda
`)
	}

	filePath := fs.String("file", "", "Caminho do arquivo XLSX a importar")
	dryRun := fs.Bool("dry-run", false, "Executa simulação sem persistir dados")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("parâmetros inválidos: %w", err)
	}

	if *filePath == "" {
		fs.Usage()
		return fmt.Errorf("a flag --file é obrigatória")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("carregamento de configuração: %w", err)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("inicialização do banco SQLite: %w", err)
	}
	defer db.Close()

	// Garante que migrations estão aplicadas
	if err := store.Migrate(ctx, db); err != nil {
		return fmt.Errorf("aplicação de migrations: %w", err)
	}

	// Resolução do caminho do arquivo de mapping
	mappingPath := os.Getenv("MAPPING_PATH")
	if mappingPath == "" {
		if _, err := os.Stat("config/import-mapping-v1.yaml"); err == nil {
			mappingPath = "config/import-mapping-v1.yaml"
		} else if _, err := os.Stat("/config/import-mapping-v1.yaml"); err == nil {
			mappingPath = "/config/import-mapping-v1.yaml"
		} else {
			mappingPath = "config/import-mapping-v1.yaml"
		}
	}

	imp := importer.NewImporter(db, mappingPath)

	result, err := imp.ImportFromFile(ctx, importer.ImportOptions{
		FilePath: *filePath,
		DryRun:   *dryRun,
	})
	if err != nil {
		return err
	}

	fmt.Print(result.SummaryReport())
	return nil
}

func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Uso do comando export:
  vorcarozap export [--out <caminho.xlsx>]

Opções:
  --out string   Caminho do arquivo de destino (padrão: vorcarozap-dados-publicos.xlsx)
  -h, --help     Exibe esta ajuda
`)
	}

	outPath := fs.String("out", "vorcarozap-dados-publicos.xlsx", "Caminho do arquivo XLSX de saída")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("parâmetros inválidos: %w", err)
	}

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

	slog.Info("iniciando exportação da base pública...", "out", *outPath)
	exp := exporter.New(db, cfg.PublicDataCutoff)

	outFile, err := os.Create(*outPath)
	if err != nil {
		return fmt.Errorf("criação do arquivo de saída %q: %w", *outPath, err)
	}
	defer outFile.Close()

	if err := exp.WriteTo(ctx, outFile); err != nil {
		return fmt.Errorf("geração da planilha XLSX: %w", err)
	}

	slog.Info("exportação concluída com sucesso", "arquivo", *outPath)
	fmt.Printf("Base pública exportada com sucesso para %s\n", *outPath)
	return nil
}
