package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/exporter"
	"github.com/raphaieu/vorcarozap/internal/importer"
	"github.com/raphaieu/vorcarozap/internal/monitoring"
	"github.com/raphaieu/vorcarozap/internal/research/openrouter"
	"github.com/raphaieu/vorcarozap/internal/sourcecheck"
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
	case "monitor":
		if err := runMonitor(os.Args[2:]); err != nil {
			slog.Error("erro ao executar comando monitor", "error", err)
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
  monitor    Executa ciclo seguro de monitoramento OpenRouter (--query obrigatório)
  help       Exibe esta mensagem de ajuda

Configuração via variáveis de ambiente:
  APP_PORT                                Porta HTTP do servidor (padrão: 8080)
  APP_ENV                                 Ambiente de execução (padrão: development)
  DB_PATH                                 Caminho do banco SQLite (padrão: ./data/vorcarozap.db)
  MAPPING_PATH                            Caminho do YAML de mapping (padrão: config/import-mapping-v1.yaml)
  PUBLIC_DATA_CUTOFF                      Data de corte para dados públicos YYYY-MM-DD (padrão: 2026-09-03)
  APP_READ_TIMEOUT                        Timeout de leitura HTTP (padrão: 5s)
  APP_WRITE_TIMEOUT                       Timeout de escrita HTTP (padrão: 10s)
  APP_IDLE_TIMEOUT                        Timeout de conexões ociosas (padrão: 60s)
  SOURCE_NOT_CHECKED_POLICY_OPENROUTER    Política para openrouter not_checked: quarantine|allow (padrão: quarantine)
  SOURCE_NOT_CHECKED_POLICY_CURATED_SEED  Política para curated_seed not_checked: quarantine|allow (padrão: allow)
  SOURCE_CHECK_TIMEOUT                    Timeout de verificação de fonte (padrão: 5s)
  OPENROUTER_API_KEY                      Chave de API do OpenRouter (sem default; obrigatória para descoberta)
  OPENROUTER_BASE_URL                     Endpoint de completions (padrão: https://openrouter.ai/api/v1/chat/completions)
  OPENROUTER_DISCOVERY_MODEL              Modelo LLM para descoberta (padrão: openai/gpt-4.1-mini)
  OPENROUTER_VERIFICATION_MODEL           Modelo LLM para gate semântico (padrão: openai/gpt-4.1-mini)
  OPENROUTER_TIMEOUT                      Timeout HTTP para chamadas OpenRouter (padrão: 30s)
  OPENROUTER_WEB_SEARCH_ENGINE            Mecanismo de busca: auto|native|exa|firecrawl|parallel|perplexity (padrão: auto)
  OPENROUTER_WEB_SEARCH_MAX_RESULTS       Resultados por busca 1..25 (padrão: 5)
  OPENROUTER_WEB_SEARCH_MAX_TOTAL_RESULTS Limite cumulativo de resultados por requisição (padrão: 15)
  OPENROUTER_WEB_SEARCH_MAX_USES          Limite de buscas por requisição (padrão: 3)
  OPENROUTER_MAX_TOOL_CALLS               Passos máximos de server tools 1..30 (padrão: 5)
  MONITOR_WINDOW                          Janela temporal padrão para busca incremental (padrão: 24h)
  MONITOR_MAX_CANDIDATES_PER_RUN          Limite de candidatos ingeridos por execução 1..100 (padrão: 10)
  MONITOR_MAX_VERIFICATIONS_PER_RUN       Limite de verificações semânticas por execução 1..100 (padrão: 10)
  MONITOR_MAX_COST_PER_RUN_USD            Teto orçamentário máximo por execução em USD (padrão: 0.25)
  MONITOR_MAX_COST_PER_DAY_USD            Teto orçamentário máximo acumulado por dia em USD (padrão: 1.00)
  MONITOR_LOCK_TTL                        Tempo de vida do lease exclusivo no SQLite (padrão: 10m)
  ADMIN_USER                              Nome do usuário administrador para HTTP Basic Auth (sem default)
  ADMIN_PASSWORD_HASH                     Hash bcrypt da senha do administrador, custo 12..14 (sem default)
  ADMIN_ALLOWED_ORIGIN                    Origem permitida para proteção CSRF no admin (ex: http://localhost:8090)
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

// MonitorRunnerInterface define o contrato abstrato para execução do monitoramento a partir do CLI.
type MonitorRunnerInterface interface {
	Run(ctx context.Context, query string) (*monitoring.RunSummary, error)
}

func runMonitor(args []string) error {
	return runMonitorCommand(context.Background(), args, os.Stdout, os.Stderr, nil, nil)
}

func runMonitorCommand(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	cfgOverride *config.Config,
	runnerFactory func(cfg *config.Config, db *sql.DB) (MonitorRunnerInterface, error),
) error {
	fs := flag.NewFlagSet("monitor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, `Uso do comando monitor:
  vorcarozap monitor --query "<termo de busca>"

Opções:
  --query string   Consulta temática para pesquisa e monitoramento factual (obrigatório)
  -h, --help       Exibe esta ajuda
`)
	}

	query := fs.String("query", "", "Consulta de busca para o monitoramento")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("parâmetros inválidos: %w", err)
	}

	trimmedQuery := strings.TrimSpace(*query)
	if trimmedQuery == "" {
		fs.Usage()
		return fmt.Errorf("a flag --query é obrigatória e não pode ser vazia")
	}

	var cfg *config.Config
	var err error
	if cfgOverride != nil {
		cfg = cfgOverride
	} else {
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("carregamento de configuração: %w", err)
		}
	}

	if runnerFactory == nil && strings.TrimSpace(cfg.OpenRouterAPIKey) == "" {
		return fmt.Errorf("a variável de ambiente OPENROUTER_API_KEY é obrigatória para executar o monitoramento")
	}

	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("inicialização do banco SQLite: %w", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		return fmt.Errorf("aplicação de migrations: %w", err)
	}

	var runner MonitorRunnerInterface
	if runnerFactory != nil {
		runner, err = runnerFactory(cfg, db)
		if err != nil {
			return fmt.Errorf("inicialização do runner injetado: %w", err)
		}
	} else {
		openrouterClient, err := openrouter.NewClient(openrouter.ClientConfig{
			APIKey:                   cfg.OpenRouterAPIKey,
			BaseURL:                  cfg.OpenRouterBaseURL,
			DiscoveryModel:           cfg.OpenRouterDiscoveryModel,
			VerificationModel:        cfg.OpenRouterVerificationModel,
			Timeout:                  cfg.OpenRouterTimeout,
			WebSearchEngine:          cfg.OpenRouterWebSearchEngine,
			WebSearchMaxResults:      cfg.OpenRouterWebSearchMaxResults,
			WebSearchMaxTotalResults: cfg.OpenRouterWebSearchMaxTotalResults,
			WebSearchMaxUses:         cfg.OpenRouterWebSearchMaxUses,
			MaxToolCalls:             cfg.OpenRouterMaxToolCalls,
		})
		if err != nil {
			return fmt.Errorf("inicialização do cliente OpenRouter: %w", err)
		}

		sourceVerifier := sourcecheck.NewVerifier(sourcecheck.VerifierConfig{
			TotalTimeout: cfg.SourceCheckTimeout,
		})

		runner, err = monitoring.NewRunner(monitoring.RunnerConfig{
			DB:                     db,
			Provider:               openrouterClient,
			SourceVerifier:         sourceVerifier,
			DiscoveryModel:         cfg.OpenRouterDiscoveryModel,
			VerificationModel:      cfg.OpenRouterVerificationModel,
			MonitorWindow:          cfg.MonitorWindow,
			MaxCandidatesPerRun:    cfg.MonitorMaxCandidatesPerRun,
			MaxVerificationsPerRun: cfg.MonitorMaxVerificationsPerRun,
			MaxCostPerRunUSD:       cfg.MonitorMaxCostPerRunUSD,
			MaxCostPerRunMicroUSD:  cfg.MonitorMaxCostPerRunMicroUSD,
			MaxCostPerDayUSD:       cfg.MonitorMaxCostPerDayUSD,
			MaxCostPerDayMicroUSD:  cfg.MonitorMaxCostPerDayMicroUSD,
			LockTTL:                cfg.MonitorLockTTL,
		})
		if err != nil {
			return fmt.Errorf("inicialização do runner de monitoramento: %w", err)
		}
	}

	summary, err := runner.Run(ctx, trimmedQuery)
	if err != nil {
		return fmt.Errorf("execução do monitoramento: %w", err)
	}

	printMonitorSummary(stdout, summary, cfg.MonitorMaxCostPerDayUSD)
	return nil
}

func printMonitorSummary(w io.Writer, s *monitoring.RunSummary, maxCostPerDayUSD float64) {
	fmt.Fprintf(w, `
================================================================================
VorcaroZAP — Relatório de Monitoramento Operacional
================================================================================
Run ID:                  %s
Status Operacional:      %s
Consulta:                %s
Janela Temporal (UTC):   %s até %s
--------------------------------------------------------------------------------
Candidatos Descobertos:  %d (%d únicos, %d duplicados)
Verificações Executadas: %d
Publicações Realizadas:  %d
Quarentena Editorial:    %d
Rejeições:               %d
--------------------------------------------------------------------------------
Tokens Totais:           %d (Descoberta: %d, Verificações: %d)
Custo Real (USD):        %s (Descoberta: %s, Verificações: %s)
Custo Diário Acumulado:  %s (Teto: %s)
--------------------------------------------------------------------------------
Resumo Técnico:          %s
================================================================================
`,
		s.RunID,
		strings.ToUpper(s.Status),
		s.Query,
		s.WindowStart,
		s.WindowEnd,
		s.TotalCandidates,
		s.UniqueCandidates,
		s.DuplicateCandidates,
		s.VerificationsExecuted,
		s.PublishedCount,
		s.QuarantinedCount,
		s.RejectedCount,
		s.TotalTokens,
		s.DiscoveryTokens,
		s.VerificationTokens,
		monitoring.FormatUSD(s.TotalCostUSD),
		monitoring.FormatUSD(s.DiscoveryCostUSD),
		monitoring.FormatUSD(s.VerificationCostUSD),
		monitoring.FormatUSD(s.DailyTotalCostUSD),
		monitoring.FormatUSD(maxCostPerDayUSD),
		s.TechnicalSummary,
	)
}
