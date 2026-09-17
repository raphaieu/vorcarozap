package observability

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/research"
)

// HTTPMetrics resume os dados de tráfego HTTP.
type HTTPMetrics struct {
	TotalRequests int64   `json:"total_requests"`
	Status2xx     int64   `json:"status_2xx"`
	Status3xx     int64   `json:"status_3xx"`
	Status4xx     int64   `json:"status_4xx"`
	Status5xx     int64   `json:"status_5xx"`
	Status401     int64   `json:"status_401"`
	Status403     int64   `json:"status_403"`
	Status404     int64   `json:"status_404"`
	Status429     int64   `json:"status_429"`
	Status500     int64   `json:"status_500"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	P95LatencyMs  float64 `json:"p95_latency_ms"`
	MaxLatencyMs  float64 `json:"max_latency_ms"`
}

// RuntimeMetrics resume o estado do runtime Go.
type RuntimeMetrics struct {
	UptimeSeconds int64  `json:"uptime_seconds"`
	UptimeHuman   string `json:"uptime_human"`
	GoVersion     string `json:"go_version"`
	NumGoroutines int    `json:"num_goroutines"`
	NumCPU        int    `json:"num_cpu"`
	AllocMB       string `json:"alloc_mb"`
	TotalAllocMB  string `json:"total_alloc_mb"`
	SysMB         string `json:"sys_mb"`
	NumGC         uint32 `json:"num_gc"`
}

// SQLiteMetrics resume o estado e tamanho do banco de dados SQLite WAL.
type SQLiteMetrics struct {
	DBSizeBytes     int64  `json:"db_size_bytes"`
	DBSizeHuman     string `json:"db_size_human"`
	WALSizeBytes    int64  `json:"wal_size_bytes"`
	WALSizeHuman    string `json:"wal_size_human"`
	PageCount       int64  `json:"page_count"`
	PageSizeBytes   int64  `json:"page_size_bytes"`
	FreelistCount   int64  `json:"freelist_count"`
	CurrentSchemaV  int64  `json:"current_schema_version"`
	JournalMode     string `json:"journal_mode"`
	SynchronousMode string `json:"synchronous_mode"`
}

// MonitoringAccountingMetrics resume o uso e custos de LLM e execuções de monitoramento.
type MonitoringAccountingMetrics struct {
	TotalRuns             int64   `json:"total_runs"`
	CompletedRuns         int64   `json:"completed_runs"`
	PartialRuns           int64   `json:"partial_runs"`
	FailedRuns            int64   `json:"failed_runs"`
	TotalCandidates       int64   `json:"total_candidates"`
	TotalEvaluations      int64   `json:"total_evaluations"`
	PublishedCount        int64   `json:"published_count"`
	QuarantinedCount      int64   `json:"quarantined_count"`
	RejectedCount         int64   `json:"rejected_count"`
	TotalCostUSD          float64 `json:"total_cost_usd"`
	TotalCostUSDFormatted string  `json:"total_cost_usd_formatted"`
	DailyCostUSD          float64 `json:"daily_cost_usd"`
	DailyCostUSDFormatted string  `json:"daily_cost_usd_formatted"`
	DailyBudgetUSD        float64 `json:"daily_budget_usd"`
	DailyBudgetFormatted  string  `json:"daily_budget_formatted"`
	BudgetConsumptionPct  float64 `json:"budget_consumption_pct"`
}

// QueueMetrics resume o tamanho das filas editoriais e operacionais.
type QueueMetrics struct {
	TotalClaims           int64 `json:"total_claims"`
	ActiveClaims          int64 `json:"active_claims"`
	QuarantinedClaims     int64 `json:"quarantined_claims"`
	RejectedClaims        int64 `json:"rejected_claims"`
	TotalManifestations   int64 `json:"total_manifestations"`
	PendingManifestations int64 `json:"pending_manifestations"`
	TotalUsers            int64 `json:"total_users"`
	ActiveSessions        int64 `json:"active_sessions"`
	LockedUsers           int64 `json:"locked_users"`
}

// BackupTelemetry resume o status das operações de backup local e remoto.
type BackupTelemetry struct {
	LocalLastRunAt       *string `json:"local_last_run_at,omitempty"`
	LocalDurationMs      int64   `json:"local_duration_ms"`
	LocalSizeBytes       int64   `json:"local_size_bytes"`
	LocalSizeHuman       string  `json:"local_size_human"`
	LocalSchemaVersion   int64   `json:"local_schema_version"`
	LocalStatus          string  `json:"local_status"` // "none", "ok", "error"
	LocalLastError       string  `json:"local_last_error,omitempty"`
	RemoteEnabled        bool    `json:"remote_enabled"`
	RemoteProvider       string  `json:"remote_provider"`
	RemoteBucket         string  `json:"remote_bucket"`
	RemotePrefix         string  `json:"remote_prefix"`
	RemoteLastRunAt      *string `json:"remote_last_run_at,omitempty"`
	RemoteDurationMs     int64   `json:"remote_duration_ms"`
	RemoteSizeBytes      int64   `json:"remote_size_bytes"`
	RemoteSizeHuman      string  `json:"remote_size_human"`
	RemoteKey            string  `json:"remote_key,omitempty"`
	RemoteSHA256         string  `json:"remote_sha256,omitempty"`
	RemoteStatus         string  `json:"remote_status"` // "disabled", "none", "ok", "error"
	RemoteLastError      string  `json:"remote_last_error,omitempty"`
	RemoteRetentionCount int     `json:"remote_retention_count"`
}

// MetricsSnapshot consolida todas as métricas operacionais para a tela administrativa e API.
type MetricsSnapshot struct {
	Timestamp   string                      `json:"timestamp"`
	Environment string                      `json:"environment"`
	Runtime     RuntimeMetrics              `json:"runtime"`
	HTTP        HTTPMetrics                 `json:"http"`
	SQLite      SQLiteMetrics               `json:"sqlite"`
	Monitoring  MonitoringAccountingMetrics `json:"monitoring"`
	Queues      QueueMetrics                `json:"queues"`
	Backup      BackupTelemetry             `json:"backup"`
}

// MetricsRegistry gerencia a coleta em memória de telemetria operacional.
type MetricsRegistry struct {
	startTime time.Time

	// Contadores HTTP atômicos
	reqTotal  atomic.Int64
	status2xx atomic.Int64
	status3xx atomic.Int64
	status4xx atomic.Int64
	status5xx atomic.Int64
	status401 atomic.Int64
	status403 atomic.Int64
	status404 atomic.Int64
	status429 atomic.Int64
	status500 atomic.Int64

	// Latências recentes em milissegundos
	latencyMu   sync.RWMutex
	latencyList []float64
	maxLatency  float64

	// Telemetria de Backups
	backupMu           sync.RWMutex
	localLastRunAt     *time.Time
	localDurationMs    int64
	localSizeBytes     int64
	localSchemaVersion int64
	localStatus        string
	localLastError     string

	remoteLastRunAt  *time.Time
	remoteDurationMs int64
	remoteSizeBytes  int64
	remoteProvider   string
	remoteBucket     string
	remoteKey        string
	remoteSHA256     string
	remoteStatus     string
	remoteLastError  string
}

// GlobalRegistry é a instância singleton para a aplicação.
var (
	globalRegistry     *MetricsRegistry
	globalRegistryOnce sync.Once
)

// GetRegistry retorna o registro global de métricas.
func GetRegistry() *MetricsRegistry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewMetricsRegistry()
	})
	return globalRegistry
}

// NewMetricsRegistry cria um novo coletor isolado.
func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		startTime:    time.Now().UTC(),
		latencyList:  make([]float64, 0, 1000),
		localStatus:  "none",
		remoteStatus: "none",
	}
}

// RecordHTTPRequest registra a ocorrência de uma requisição HTTP.
func (r *MetricsRegistry) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration) {
	r.reqTotal.Add(1)

	switch {
	case statusCode >= 200 && statusCode < 300:
		r.status2xx.Add(1)
	case statusCode >= 300 && statusCode < 400:
		r.status3xx.Add(1)
	case statusCode >= 400 && statusCode < 500:
		r.status4xx.Add(1)
		switch statusCode {
		case 401:
			r.status401.Add(1)
		case 403:
			r.status403.Add(1)
		case 404:
			r.status404.Add(1)
		case 429:
			r.status429.Add(1)
		}
	case statusCode >= 500:
		r.status5xx.Add(1)
		if statusCode == 500 {
			r.status500.Add(1)
		}
	}

	latMs := float64(duration.Microseconds()) / 1000.0

	r.latencyMu.Lock()
	defer r.latencyMu.Unlock()

	if latMs > r.maxLatency {
		r.maxLatency = latMs
	}

	if len(r.latencyList) >= 1000 {
		// Desloca janela deslizante mantendo 500 mais recentes
		copy(r.latencyList, r.latencyList[500:])
		r.latencyList = r.latencyList[:500]
	}
	r.latencyList = append(r.latencyList, latMs)
}

// RecordLocalBackup registra o resultado da execução do backup local.
func (r *MetricsRegistry) RecordLocalBackup(sizeBytes int64, duration time.Duration, schemaVersion int64, err error) {
	r.backupMu.Lock()
	defer r.backupMu.Unlock()

	now := time.Now().UTC()
	r.localLastRunAt = &now
	r.localDurationMs = duration.Milliseconds()
	r.localSizeBytes = sizeBytes
	r.localSchemaVersion = schemaVersion

	if err != nil {
		r.localStatus = "error"
		r.localLastError = err.Error()
	} else {
		r.localStatus = "ok"
		r.localLastError = ""
	}
}

// RecordRemoteBackup registra o resultado da execução do backup remoto.
func (r *MetricsRegistry) RecordRemoteBackup(sizeBytes int64, duration time.Duration, provider, bucket, key, sha256Hex string, err error) {
	r.backupMu.Lock()
	defer r.backupMu.Unlock()

	now := time.Now().UTC()
	r.remoteLastRunAt = &now
	r.remoteDurationMs = duration.Milliseconds()
	r.remoteSizeBytes = sizeBytes
	r.remoteProvider = provider
	r.remoteBucket = bucket
	r.remoteKey = key
	r.remoteSHA256 = sha256Hex

	if err != nil {
		r.remoteStatus = "error"
		r.remoteLastError = err.Error()
	} else {
		r.remoteStatus = "ok"
		r.remoteLastError = ""
	}
}

// CollectSnapshot agrega métricas de runtime, banco, filas e telemetria em um snapshot imutável.
func (r *MetricsRegistry) CollectSnapshot(ctx context.Context, db *sql.DB, cfg *config.Config) (*MetricsSnapshot, error) {
	now := time.Now().UTC()

	// 1. Runtime Go
	uptime := time.Since(r.startTime)
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	runtimeMetrics := RuntimeMetrics{
		UptimeSeconds: int64(uptime.Seconds()),
		UptimeHuman:   formatDuration(uptime),
		GoVersion:     runtime.Version(),
		NumGoroutines: runtime.NumGoroutine(),
		NumCPU:        runtime.NumCPU(),
		AllocMB:       fmt.Sprintf("%.2f MB", float64(memStats.Alloc)/(1024*1024)),
		TotalAllocMB:  fmt.Sprintf("%.2f MB", float64(memStats.TotalAlloc)/(1024*1024)),
		SysMB:         fmt.Sprintf("%.2f MB", float64(memStats.Sys)/(1024*1024)),
		NumGC:         memStats.NumGC,
	}

	// 2. HTTP Performance
	r.latencyMu.RLock()
	var avgLat, p95Lat, maxLat float64
	maxLat = r.maxLatency
	n := len(r.latencyList)
	if n > 0 {
		var sum float64
		sortedLat := make([]float64, n)
		copy(sortedLat, r.latencyList)
		for _, v := range sortedLat {
			sum += v
		}
		avgLat = sum / float64(n)

		sort.Float64s(sortedLat)
		p95Idx := int(float64(n) * 0.95)
		if p95Idx >= n {
			p95Idx = n - 1
		}
		p95Lat = sortedLat[p95Idx]
	}
	r.latencyMu.RUnlock()

	httpMetrics := HTTPMetrics{
		TotalRequests: r.reqTotal.Load(),
		Status2xx:     r.status2xx.Load(),
		Status3xx:     r.status3xx.Load(),
		Status4xx:     r.status4xx.Load(),
		Status5xx:     r.status5xx.Load(),
		Status401:     r.status401.Load(),
		Status403:     r.status403.Load(),
		Status404:     r.status404.Load(),
		Status429:     r.status429.Load(),
		Status500:     r.status500.Load(),
		AvgLatencyMs:  roundToTwoDecimals(avgLat),
		P95LatencyMs:  roundToTwoDecimals(p95Lat),
		MaxLatencyMs:  roundToTwoDecimals(maxLat),
	}

	// 3. SQLite WAL Health
	var sqliteMetrics SQLiteMetrics
	if db != nil {
		if cfg != nil && cfg.DBPath != "" {
			if fi, err := os.Stat(cfg.DBPath); err == nil {
				sqliteMetrics.DBSizeBytes = fi.Size()
				sqliteMetrics.DBSizeHuman = formatBytes(fi.Size())
			}
			walPath := cfg.DBPath + "-wal"
			if fiWal, err := os.Stat(walPath); err == nil {
				sqliteMetrics.WALSizeBytes = fiWal.Size()
				sqliteMetrics.WALSizeHuman = formatBytes(fiWal.Size())
			} else {
				sqliteMetrics.WALSizeHuman = "0 B (sem dados no journal)"
			}
		}

		_ = db.QueryRowContext(ctx, "PRAGMA page_count;").Scan(&sqliteMetrics.PageCount)
		_ = db.QueryRowContext(ctx, "PRAGMA page_size;").Scan(&sqliteMetrics.PageSizeBytes)
		_ = db.QueryRowContext(ctx, "PRAGMA freelist_count;").Scan(&sqliteMetrics.FreelistCount)
		_ = db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&sqliteMetrics.JournalMode)
		_ = db.QueryRowContext(ctx, "PRAGMA synchronous;").Scan(&sqliteMetrics.SynchronousMode)

		// Goose version
		var maxVer sql.NullInt64
		_ = db.QueryRowContext(ctx, "SELECT max(version_id) FROM goose_db_version;").Scan(&maxVer)
		if maxVer.Valid {
			sqliteMetrics.CurrentSchemaV = maxVer.Int64
		}
	}

	// 4. Monitoring & LLM Accounting
	var monMetrics MonitoringAccountingMetrics
	if db != nil {
		var totalCostMicros, dailyCostMicros int64
		_ = db.QueryRowContext(ctx, `
			SELECT 
				count(*),
				coalesce(sum(case when status='completed' then 1 else 0 end), 0),
				coalesce(sum(case when status='partial' then 1 else 0 end), 0),
				coalesce(sum(case when status='failed' then 1 else 0 end), 0),
				coalesce(sum(total_cost_micros), 0)
			FROM monitoring_runs;
		`).Scan(
			&monMetrics.TotalRuns,
			&monMetrics.CompletedRuns,
			&monMetrics.PartialRuns,
			&monMetrics.FailedRuns,
			&totalCostMicros,
		)

		_ = db.QueryRowContext(ctx, `
			SELECT coalesce(sum(total_cost_micros), 0)
			FROM monitoring_runs
			WHERE date(created_at) = date('now');
		`).Scan(&dailyCostMicros)

		_ = db.QueryRowContext(ctx, "SELECT count(*) FROM monitoring_candidates;").Scan(&monMetrics.TotalCandidates)

		_ = db.QueryRowContext(ctx, `
			SELECT 
				count(*),
				coalesce(sum(case when recommendation='publish' then 1 else 0 end), 0),
				coalesce(sum(case when recommendation='quarantine' then 1 else 0 end), 0),
				coalesce(sum(case when recommendation='reject' then 1 else 0 end), 0)
			FROM semantic_evaluations;
		`).Scan(
			&monMetrics.TotalEvaluations,
			&monMetrics.PublishedCount,
			&monMetrics.QuarantinedCount,
			&monMetrics.RejectedCount,
		)

		monMetrics.TotalCostUSD = research.MicroUSDToFloat(totalCostMicros)
		monMetrics.TotalCostUSDFormatted = fmt.Sprintf("$%.4f", monMetrics.TotalCostUSD)
		monMetrics.DailyCostUSD = research.MicroUSDToFloat(dailyCostMicros)
		monMetrics.DailyCostUSDFormatted = fmt.Sprintf("$%.4f", monMetrics.DailyCostUSD)

		if cfg != nil {
			monMetrics.DailyBudgetUSD = cfg.MonitorMaxCostPerDayUSD
			monMetrics.DailyBudgetFormatted = fmt.Sprintf("$%.2f", cfg.MonitorMaxCostPerDayUSD)
			if cfg.MonitorMaxCostPerDayUSD > 0 {
				monMetrics.BudgetConsumptionPct = roundToTwoDecimals((monMetrics.DailyCostUSD / cfg.MonitorMaxCostPerDayUSD) * 100.0)
			}
		}
	}

	// 5. Queues and Moderation
	var queueMetrics QueueMetrics
	if db != nil {
		_ = db.QueryRowContext(ctx, `
			SELECT 
				count(*),
				coalesce(sum(case when editorial_status='active' then 1 else 0 end), 0),
				coalesce(sum(case when editorial_status='quarantined' then 1 else 0 end), 0),
				coalesce(sum(case when editorial_status='rejected' then 1 else 0 end), 0)
			FROM claims;
		`).Scan(
			&queueMetrics.TotalClaims,
			&queueMetrics.ActiveClaims,
			&queueMetrics.QuarantinedClaims,
			&queueMetrics.RejectedClaims,
		)

		_ = db.QueryRowContext(ctx, `
			SELECT 
				count(*),
				coalesce(sum(case when status='pending' then 1 else 0 end), 0)
			FROM defense_statements;
		`).Scan(
			&queueMetrics.TotalManifestations,
			&queueMetrics.PendingManifestations,
		)

		_ = db.QueryRowContext(ctx, `
			SELECT 
				count(*),
				coalesce(sum(case when status='locked' then 1 else 0 end), 0)
			FROM admin_users;
		`).Scan(
			&queueMetrics.TotalUsers,
			&queueMetrics.LockedUsers,
		)

		_ = db.QueryRowContext(ctx, `
			SELECT count(*) FROM admin_sessions WHERE expires_at > datetime('now');
		`).Scan(&queueMetrics.ActiveSessions)
	}

	// 6. Backup Telemetry
	r.backupMu.RLock()
	var localRunStr, remoteRunStr *string
	if r.localLastRunAt != nil {
		s := r.localLastRunAt.Format(time.RFC3339)
		localRunStr = &s
	}
	if r.remoteLastRunAt != nil {
		s := r.remoteLastRunAt.Format(time.RFC3339)
		remoteRunStr = &s
	}

	remoteEnabled := false
	remoteProvider := "s3"
	remoteBucket := ""
	remotePrefix := "backups"
	remoteRetention := 14
	if cfg != nil {
		remoteEnabled = cfg.BackupRemoteEnabled
		remoteProvider = cfg.BackupRemoteProvider
		remoteBucket = cfg.BackupRemoteBucket
		remotePrefix = cfg.BackupRemotePrefix
		remoteRetention = cfg.BackupRemoteRetentionCount
	}

	remoteStatus := r.remoteStatus
	if !remoteEnabled && remoteStatus == "none" {
		remoteStatus = "disabled"
	}

	backupMetrics := BackupTelemetry{
		LocalLastRunAt:       localRunStr,
		LocalDurationMs:      r.localDurationMs,
		LocalSizeBytes:       r.localSizeBytes,
		LocalSizeHuman:       formatBytes(r.localSizeBytes),
		LocalSchemaVersion:   r.localSchemaVersion,
		LocalStatus:          r.localStatus,
		LocalLastError:       r.localLastError,
		RemoteEnabled:        remoteEnabled,
		RemoteProvider:       remoteProvider,
		RemoteBucket:         remoteBucket,
		RemotePrefix:         remotePrefix,
		RemoteLastRunAt:      remoteRunStr,
		RemoteDurationMs:     r.remoteDurationMs,
		RemoteSizeBytes:      r.remoteSizeBytes,
		RemoteSizeHuman:      formatBytes(r.remoteSizeBytes),
		RemoteKey:            r.remoteKey,
		RemoteSHA256:         r.remoteSHA256,
		RemoteStatus:         remoteStatus,
		RemoteLastError:      r.remoteLastError,
		RemoteRetentionCount: remoteRetention,
	}
	r.backupMu.RUnlock()

	envName := "development"
	if cfg != nil && cfg.Env != "" {
		envName = cfg.Env
	}

	return &MetricsSnapshot{
		Timestamp:   now.Format(time.RFC3339),
		Environment: envName,
		Runtime:     runtimeMetrics,
		HTTP:        httpMetrics,
		SQLite:      sqliteMetrics,
		Monitoring:  monMetrics,
		Queues:      queueMetrics,
		Backup:      backupMetrics,
	}, nil
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func formatBytes(b int64) string {
	if b <= 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func roundToTwoDecimals(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100.0
}
