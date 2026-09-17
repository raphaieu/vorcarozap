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

	"github.com/raphaieu/vorcarozap/internal/backup"
	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/exporter"
	"github.com/raphaieu/vorcarozap/internal/importer"
	"github.com/raphaieu/vorcarozap/internal/monitoring"
	"github.com/raphaieu/vorcarozap/internal/observability"
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

	if cfg, err := config.Load(); err == nil {
		observability.InitLogger(cfg, os.Stderr)
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
	case "backup":
		if err := runBackup(os.Args[2:]); err != nil {
			slog.Error("erro ao executar comando backup", "error", err)
			os.Exit(1)
		}
	case "verify-backup":
		if err := runVerifyBackup(os.Args[2:]); err != nil {
			slog.Error("erro ao executar comando verify-backup", "error", err)
			os.Exit(1)
		}
	case "verify-remote":
		if err := runVerifyRemote(os.Args[2:]); err != nil {
			slog.Error("erro ao executar comando verify-remote", "error", err)
			os.Exit(1)
		}
	case "restore":
		if err := runRestore(os.Args[2:]); err != nil {
			slog.Error("erro ao executar comando restore", "error", err)
			os.Exit(1)
		}
	case "restore-sandbox":
		if err := runRestoreSandbox(os.Args[2:]); err != nil {
			slog.Error("erro ao executar comando restore-sandbox", "error", err)
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
  serve            Aplica migrations pendentes e inicia o servidor HTTP
  migrate          Aplica migrations pendentes no banco SQLite
  import           Importa dados da planilha XLSX curated_seed (--file obrigatório, opcional --dry-run)
  export           Exporta a base pública ativa em planilha XLSX (--out opcional)
  monitor          Executa ciclo seguro de monitoramento OpenRouter (--query obrigatório)
  backup           Gera backup atômico do SQLite via VACUUM INTO e opcionalmente envia ao Object Storage (--out, --retention, --remote)
  verify-backup    Verifica integridade (PRAGMA integrity_check) e migrations de um backup local (--file obrigatório)
  verify-remote    Verifica presença, integridade e metadados de backup no Object Storage (--key obrigatório)
  restore          Restaura backup verificado para o banco de destino (--backup, --target e --confirm)
  restore-sandbox  Restaura backup em sandbox isolado sem alterar produção (--file ou --remote-key)
  help             Exibe esta mensagem de ajuda

Configuração via variáveis de ambiente:
  APP_PORT                                Porta HTTP do servidor (padrão: 8080)
  APP_ENV                                 Ambiente de execução (padrão: development)
  DB_PATH                                 Caminho do banco SQLite (padrão: ./data/vorcarozap.db)
  MAPPING_PATH                            Caminho do YAML de mapping (padrão: config/import-mapping-v1.yaml)
  PUBLIC_DATA_CUTOFF                      Data de corte para dados públicos YYYY-MM-DD (padrão: 2026-09-03)
  LOG_FORMAT                              Formato de log: text|json (padrão: text)
  LOG_LEVEL                               Nível de log: debug|info|warn|error (padrão: info)
  APP_READ_TIMEOUT                        Timeout de leitura HTTP (padrão: 5s)
  APP_WRITE_TIMEOUT                       Timeout de escrita HTTP (padrão: 10s)
  APP_IDLE_TIMEOUT                        Timeout de conexões ociosas (padrão: 60s)
  BACKUP_REMOTE_ENABLED                   Habilita envio seguro para Object Storage S3 (padrão: false)
  BACKUP_REMOTE_PROVIDER                  Provedor de Object Storage: s3|r2|minio|wasabi|oracle|custom (padrão: s3)
  BACKUP_REMOTE_ENDPOINT                  Endpoint HTTPS do S3/R2 (obrigatório se remote habilitado)
  BACKUP_REMOTE_REGION                    Região do bucket S3 (padrão: us-east-1)
  BACKUP_REMOTE_BUCKET                    Nome do bucket S3 (obrigatório se remote habilitado)
  BACKUP_REMOTE_PREFIX                    Prefixo/diretório de destino no bucket (padrão: backups)
  BACKUP_REMOTE_ACCESS_KEY_ID             Access Key ID para autenticação AWS SigV4
  BACKUP_REMOTE_SECRET_ACCESS_KEY         Secret Access Key para autenticação AWS SigV4
  BACKUP_REMOTE_RETENTION_COUNT           Quantidade de backups remotos a reter no bucket (padrão: 14)
  BACKUP_REMOTE_TIMEOUT                   Timeout de operações HTTP com o Object Storage (padrão: 60s)
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

	// 2.1 Bootstrap de conta de administrador inicial se configurada e base estiver vazia
	if cfg.IsAdminEnabled() {
		bootstrapped, err := store.BootstrapAdminUser(ctx, db, cfg.AdminUser, cfg.AdminPasswordHash, "Administrador Principal")
		if err != nil {
			slog.Warn("aviso durante bootstrap de usuário admin inicial", "error", err)
		} else if bootstrapped {
			slog.Info("conta administradora inicial criada com sucesso", "username", cfg.AdminUser)
		}
	}

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

func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Uso do comando backup:
  vorcarozap backup [--out <caminho.db>] [--retention <N>] [--remote]

Opções:
  --out string     Caminho do arquivo de destino do backup (padrão: /backups/vorcarozap-YYYYMMDD_HHMMSSZ.db ou ./backups/...)
  --retention int  Quantidade de backups locais mais recentes a reter (0 = desabilitado)
  --remote         Força o envio do backup para o Object Storage configurado
  -h, --help       Exibe esta ajuda
`)
	}

	outPath := fs.String("out", "", "Caminho do arquivo de backup de destino")
	retention := fs.Int("retention", 0, "Quantidade de backups a reter (0 = desabilitado)")
	remote := fs.Bool("remote", false, "Força envio do backup para Object Storage")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("parâmetros inválidos: %w", err)
	}

	if *retention < 0 {
		return fmt.Errorf("a flag --retention deve ser um número inteiro não negativo (recebido %d)", *retention)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("carregamento de configuração: %w", err)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("abertura do banco SQLite %q: %w", cfg.DBPath, err)
	}
	defer db.Close()

	mgr, err := backup.NewManager(backup.ManagerConfig{
		DB:       db,
		Config:   cfg,
		Registry: observability.GetRegistry(),
	})
	if err != nil {
		return fmt.Errorf("inicialização do gerenciador de backup: %w", err)
	}

	slog.Info("iniciando rotina de backup consistente...", "origem", cfg.DBPath, "destino", *outPath, "force_remote", *remote)
	report, err := mgr.RunBackup(ctx, *outPath, *retention, *remote)
	if err != nil {
		return fmt.Errorf("execução do backup: %w", err)
	}

	printBackupExecutionReport(os.Stdout, report)
	if report.RemoteAttempted && !report.RemoteSuccess && report.RemoteError != nil {
		slog.Warn("atenção: backup local foi concluído com sucesso, mas o upload remoto falhou", "error", report.RemoteError)
	}

	return nil
}

func printBackupExecutionReport(w io.Writer, rep *backup.BackupExecutionReport) {
	localRetentionInfo := "Nenhuma remoção realizada (ou retenção desabilitada)"
	if len(rep.LocalRotated) > 0 {
		localRetentionInfo = fmt.Sprintf("%d arquivo(s) antigo(s) removido(s) por retenção:\n  - %s", len(rep.LocalRotated), strings.Join(rep.LocalRotated, "\n  - "))
	}

	remoteStatus := "Desabilitado (não executado)"
	if rep.RemoteAttempted {
		if rep.RemoteSuccess {
			remoteRetentionInfo := "Nenhum objeto remoto removido (ou retenção desabilitada)"
			if len(rep.RemoteRotated) > 0 {
				remoteRetentionInfo = fmt.Sprintf("%d objeto(s) remoto(s) rotacionado(s):\n  - %s", len(rep.RemoteRotated), strings.Join(rep.RemoteRotated, "\n  - "))
			}
			remoteStatus = fmt.Sprintf(`SUCESSO
  Provedor:              %s
  Bucket:                %s
  Chave Remota:          %s
  Checksum SHA-256:      %s
  Duração do Upload:     %s
  Retenção Remota:       %s`,
				rep.RemoteProvider, rep.RemoteBucket, rep.RemoteKey, rep.RemoteSHA256, rep.RemoteDuration.Round(time.Millisecond), remoteRetentionInfo)
		} else {
			remoteStatus = fmt.Sprintf("FALHA: %v (Isolamento ativo: backup local preservado)", rep.RemoteError)
		}
	}

	fmt.Fprintf(w, `
================================================================================
VorcaroZAP — Relatório de Backup do SQLite
================================================================================
Backup Local:
  Arquivo de Destino:   %s
  Tamanho:              %.2f KB (%d bytes)
  Integridade (PRAGMA): OK (sem corrupção de páginas ou índices)
  Versão de Migrations: %d (100%% compatível com o binário)
  Duração Local:        %s
  Retenção Local:       %s

Backup Remoto (Object Storage):
  Status: %s
================================================================================
Backup concluído com sucesso e verificado.
`,
		rep.LocalPath,
		float64(rep.LocalSizeBytes)/1024.0,
		rep.LocalSizeBytes,
		rep.LocalSchemaVersion,
		rep.LocalDuration.Round(time.Millisecond),
		localRetentionInfo,
		remoteStatus,
	)
}

func printBackupSummary(w io.Writer, res *store.BackupResult, rotated []string) {
	retentionInfo := "Nenhuma remoção realizada (ou retenção desabilitada)"
	if len(rotated) > 0 {
		retentionInfo = fmt.Sprintf("%d arquivo(s) antigo(s) removido(s) por retenção:\n  - %s", len(rotated), strings.Join(rotated, "\n  - "))
	}

	fmt.Fprintf(w, `
================================================================================
VorcaroZAP — Relatório de Backup do SQLite
================================================================================
Arquivo de Destino:   %s
Tamanho:              %.2f KB (%d bytes)
Integridade (PRAGMA): OK (sem corrupção de páginas ou índices)
Versão de Migrations: %d (100%% compatível com o binário)
Data e Hora (UTC):    %s
Política de Retenção: %s
================================================================================
Backup concluído com sucesso e verificado.
`,
		res.BackupPath,
		float64(res.SizeBytes)/1024.0,
		res.SizeBytes,
		res.SchemaVersion,
		res.CreatedAt.Format(time.RFC3339),
		retentionInfo,
	)
}

func runVerifyBackup(args []string) error {
	fs := flag.NewFlagSet("verify-backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Uso do comando verify-backup:
  vorcarozap verify-backup --file <caminho.db>

Opções:
  --file string   Caminho do arquivo de backup a verificar (obrigatório)
  -h, --help      Exibe esta ajuda
`)
	}

	filePath := fs.String("file", "", "Caminho do arquivo de backup a verificar")

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

	ctx := context.Background()
	slog.Info("verificando integridade e migrations do arquivo de backup...", "arquivo", *filePath)
	result, err := store.VerifyBackup(ctx, *filePath)
	if err != nil {
		return fmt.Errorf("verificação do backup reprovou: %w", err)
	}

	slog.Info("verificação concluída: backup íntegro", "arquivo", result.BackupPath, "tamanho_bytes", result.SizeBytes, "versao_schema", result.SchemaVersion)
	printBackupSummary(os.Stdout, result, nil)
	return nil
}

func runVerifyRemote(args []string) error {
	fs := flag.NewFlagSet("verify-remote", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Uso do comando verify-remote:
  vorcarozap verify-remote --key <nome_do_objeto>

Opções:
  --key string   Chave/caminho do arquivo de backup no Object Storage (obrigatório)
  -h, --help     Exibe esta ajuda
`)
	}

	key := fs.String("key", "", "Chave do arquivo de backup no Object Storage")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("parâmetros inválidos: %w", err)
	}

	if *key == "" {
		fs.Usage()
		return fmt.Errorf("a flag --key é obrigatória")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("carregamento de configuração: %w", err)
	}

	if !cfg.BackupRemoteEnabled {
		return fmt.Errorf("backup remoto está desabilitado na configuração (defina BACKUP_REMOTE_ENABLED=true e credenciais)")
	}

	ctx := context.Background()
	mgr, err := backup.NewManager(backup.ManagerConfig{
		Config:   cfg,
		Registry: observability.GetRegistry(),
	})
	if err != nil {
		return fmt.Errorf("inicialização do gerenciador de backup: %w", err)
	}

	slog.Info("verificando objeto de backup no Object Storage...", "key", *key, "bucket", cfg.BackupRemoteBucket)
	meta, err := mgr.VerifyRemote(ctx, *key)
	if err != nil {
		return fmt.Errorf("verificação remota falhou: %w", err)
	}

	slog.Info("backup remoto verificado com sucesso", "key", meta.Key, "size_bytes", meta.SizeBytes, "sha256", meta.SHA256Hex)
	fmt.Printf(`
================================================================================
VorcaroZAP — Verificação de Backup Remoto (Object Storage)
================================================================================
Provedor:              %s
Bucket:                %s
Chave Remota:          %s
Tamanho:               %.2f KB (%d bytes)
Checksum SHA-256:      %s
Última Modificação:    %s
Status:                Íntegro e presente no Object Storage
================================================================================
`,
		cfg.BackupRemoteProvider,
		cfg.BackupRemoteBucket,
		meta.Key,
		float64(meta.SizeBytes)/1024.0,
		meta.SizeBytes,
		meta.SHA256Hex,
		meta.LastModified.Format(time.RFC3339),
	)
	return nil
}

func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Uso do comando restore:
  vorcarozap restore --backup <origem.db> --target <destino.db> [--confirm]

Opções:
  --backup string   Caminho do arquivo de backup verificado (obrigatório)
  --target string   Caminho do banco de destino (obrigatório)
  --confirm         Confirmação explícita para restauração (obrigatório)
  -h, --help        Exibe esta ajuda
`)
	}

	backupPath := fs.String("backup", "", "Caminho do arquivo de backup de origem")
	targetPath := fs.String("target", "", "Caminho do banco de dados de destino")
	confirm := fs.Bool("confirm", false, "Confirmação explícita de restauração")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("parâmetros inválidos: %w", err)
	}

	if *backupPath == "" || *targetPath == "" {
		fs.Usage()
		return fmt.Errorf("as flags --backup e --target são obrigatórias")
	}

	if !*confirm {
		return fmt.Errorf("a restauração requer confirmação explícita com a flag --confirm")
	}

	ctx := context.Background()
	slog.Info("iniciando restauração de backup...", "backup", *backupPath, "target", *targetPath)
	if err := store.Restore(ctx, *backupPath, *targetPath); err != nil {
		return fmt.Errorf("restauração falhou: %w", err)
	}

	slog.Info("restauração concluída com sucesso e verificada", "destino", *targetPath)
	fmt.Printf("Backup %s restaurado com sucesso em %s (integridade e migrations verificadas)\n", *backupPath, *targetPath)
	return nil
}

func runRestoreSandbox(args []string) error {
	fs := flag.NewFlagSet("restore-sandbox", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Uso do comando restore-sandbox:
  vorcarozap restore-sandbox [--file <caminho.db>] [--remote-key <nome_do_objeto>]

Opções:
  --file string         Caminho do arquivo de backup local a validar
  --remote-key string   Chave do backup no Object Storage a baixar e validar
  -h, --help            Exibe esta ajuda
`)
	}

	filePath := fs.String("file", "", "Caminho do arquivo de backup local")
	remoteKey := fs.String("remote-key", "", "Chave do arquivo de backup no Object Storage")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("parâmetros inválidos: %w", err)
	}

	if *filePath == "" && *remoteKey == "" {
		fs.Usage()
		return fmt.Errorf("informe ao menos uma origem para teste de restauração (--file ou --remote-key)")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("carregamento de configuração: %w", err)
	}

	ctx := context.Background()
	mgr, err := backup.NewManager(backup.ManagerConfig{
		Config:   cfg,
		Registry: observability.GetRegistry(),
	})
	if err != nil {
		return fmt.Errorf("inicialização do gerenciador de backup: %w", err)
	}

	slog.Info("iniciando restauração não-destrutiva em sandbox isolado...", "file", *filePath, "remote_key", *remoteKey)
	report, err := mgr.RestoreSandbox(ctx, *filePath, *remoteKey)
	if err != nil {
		return fmt.Errorf("teste de restauração em sandbox falhou: %w", err)
	}

	fmt.Printf(`
================================================================================
VorcaroZAP — Relatório de Restauração em Sandbox (Não-Destrutivo)
================================================================================
Origem:                %s (%s)
Tamanho:               %.2f KB (%d bytes)
Versão de Migrations:  %d
Integridade (PRAGMA):  OK (100%% íntegro)
Entidades no Banco:    %d
Alegações no Banco:    %d
Duração do Teste:      %s
Status:                Restauração verificada com sucesso (banco ativo preservado)
================================================================================
`,
		report.SourceIdentifier,
		report.SourceType,
		float64(report.SizeBytes)/1024.0,
		report.SizeBytes,
		report.SchemaVersion,
		report.TotalEntities,
		report.TotalClaims,
		report.Duration.Round(time.Millisecond),
	)

	return nil
}
