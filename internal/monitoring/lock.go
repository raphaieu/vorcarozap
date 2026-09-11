package monitoring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// Erros sentinela para controle de concorrência e lease.
var (
	ErrLockHeld       = errors.New("monitoring: lock já está adquirido por outra execução ativa")
	ErrLockLost       = errors.New("monitoring: lease do lock expirou ou foi perdido durante a execução")
	ErrLockNotHeld    = errors.New("monitoring: lock não pertence ao titular para liberação")
	ErrInvalidLockTTL = errors.New("monitoring: TTL do lock deve ser maior que zero")
)

// LockConfig define os parâmetros para inicialização do gerenciador de lease.
type LockConfig struct {
	DB       *sql.DB
	LockName string
	TTL      time.Duration
	NowFunc  func() time.Time
}

// LockManager gerencia a aquisição atômica e o ciclo de vida do lease no SQLite.
type LockManager struct {
	db       *sql.DB
	lockName string
	ttl      time.Duration
	nowFunc  func() time.Time
}

// NewLockManager cria uma nova instância de LockManager.
func NewLockManager(cfg LockConfig) (*LockManager, error) {
	if cfg.DB == nil {
		return nil, ErrNilDB
	}
	if cfg.TTL <= 0 {
		return nil, ErrInvalidLockTTL
	}
	name := cfg.LockName
	if name == "" {
		name = "monitoring"
	}
	nowFunc := cfg.NowFunc
	if nowFunc == nil {
		nowFunc = func() time.Time { return time.Now().UTC() }
	}

	return &LockManager{
		db:       cfg.DB,
		lockName: name,
		ttl:      cfg.TTL,
		nowFunc:  nowFunc,
	}, nil
}

// Lease representa uma concessão de execução exclusiva com renovação periódica em background.
type Lease struct {
	db             *sql.DB
	lockName       string
	holder         string
	ttl            time.Duration
	nowFunc        func() time.Time
	stopRenew      chan struct{}
	done           chan struct{}
	cancelPipeline context.CancelFunc
	mu             sync.Mutex
	isReleased     bool
}

// Holder retorna o identificador único do titular do lease.
func (l *Lease) Holder() string {
	return l.holder
}

// LockName retorna o nome do recurso protegido pelo lease.
func (l *Lease) LockName() string {
	return l.lockName
}

// Acquire tenta obter atomicamente o lease no SQLite.
// Se o lock estiver ocupado por outra execução válida, retorna ErrLockHeld.
// Se o lease anterior estiver expirado, realiza takeover atômico.
// Inicia renovação periódica em background e retorna o Lease e um Context cancelado caso o lease seja perdido.
func (m *LockManager) Acquire(ctx context.Context) (*Lease, context.Context, error) {
	holder := uuid.NewString()
	now := m.nowFunc()
	expiresAt := now.Add(m.ttl)

	nowStr := now.Format(time.RFC3339Nano)
	expiresStr := expiresAt.Format(time.RFC3339Nano)

	queries := sqlc.New(m.db)
	_, err := queries.AcquireMonitoringLock(ctx, sqlc.AcquireMonitoringLockParams{
		Name:       m.lockName,
		Holder:     holder,
		AcquiredAt: nowStr,
		ExpiresAt:  expiresStr,
		UpdatedAt:  nowStr,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrLockHeld
		}
		return nil, nil, fmt.Errorf("monitoring: falha ao adquirir lock no banco: %w", err)
	}

	pipelineCtx, cancelPipeline := context.WithCancel(ctx)

	lease := &Lease{
		db:             m.db,
		lockName:       m.lockName,
		holder:         holder,
		ttl:            m.ttl,
		nowFunc:        m.nowFunc,
		stopRenew:      make(chan struct{}),
		done:           make(chan struct{}),
		cancelPipeline: cancelPipeline,
	}

	// Inicia renovação periódica do lease em background
	go lease.renewLoop()

	return lease, pipelineCtx, nil
}

func (l *Lease) renewLoop() {
	defer close(l.done)

	// Intervalo conservador de renovação: 1/3 do TTL (mínimo 100ms)
	renewInterval := l.ttl / 3
	if renewInterval < 100*time.Millisecond {
		renewInterval = 100 * time.Millisecond
	}

	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.stopRenew:
			return
		case <-ticker.C:
			if err := l.renew(); err != nil {
				slog.Error("monitoring: falha crítica ao renovar lease de execução; cancelando pipeline para proteção",
					"lock", l.lockName, "holder", l.holder, "error", err)
				l.cancelPipeline()
				return
			}
		}
	}
}

func (l *Lease) renew() error {
	l.mu.Lock()
	if l.isReleased {
		l.mu.Unlock()
		return nil
	}
	l.mu.Unlock()

	now := l.nowFunc()
	expiresAt := now.Add(l.ttl)
	nowStr := now.Format(time.RFC3339Nano)
	expiresStr := expiresAt.Format(time.RFC3339Nano)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	queries := sqlc.New(l.db)
	_, err := queries.RenewMonitoringLock(ctx, sqlc.RenewMonitoringLockParams{
		ExpiresAt: expiresStr,
		UpdatedAt: nowStr,
		Name:      l.lockName,
		Holder:    l.holder,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLockLost
		}
		return fmt.Errorf("erro ao atualizar lease no SQLite: %w", err)
	}

	return nil
}

// Release encerra a renovação em background e libera o lock atomicamente no SQLite,
// condicional exclusivamente ao titular (holder).
func (l *Lease) Release(ctx context.Context) error {
	l.mu.Lock()
	if l.isReleased {
		l.mu.Unlock()
		return nil
	}
	l.isReleased = true
	l.mu.Unlock()

	// Interrompe goroutine de renovação e aguarda encerramento
	close(l.stopRenew)
	<-l.done

	// Executa liberação no SQLite com contexto de timeout curto e independente
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	queries := sqlc.New(l.db)
	rowsAffected, err := queries.ReleaseMonitoringLock(releaseCtx, sqlc.ReleaseMonitoringLockParams{
		Name:   l.lockName,
		Holder: l.holder,
	})
	if err != nil {
		return fmt.Errorf("monitoring: falha ao liberar lock no SQLite: %w", err)
	}

	if rowsAffected == 0 {
		// O lock já havia sido liberado ou tomado após expiração por outro processo
		return ErrLockNotHeld
	}

	return nil
}
