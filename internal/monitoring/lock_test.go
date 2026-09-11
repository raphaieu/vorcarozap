package monitoring_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/monitoring"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func TestLockManagerAcquireAndRelease(t *testing.T) {
	db, ctx := setupTestDB(t)

	lm, err := monitoring.NewLockManager(monitoring.LockConfig{
		DB:       db,
		LockName: "test_lock",
		TTL:      5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("erro ao criar LockManager: %v", err)
	}

	// 1. Primeira aquisição deve ter sucesso
	lease1, pipelineCtx1, err := lm.Acquire(ctx)
	if err != nil {
		t.Fatalf("esperava sucesso na aquisição do lease 1: %v", err)
	}
	if lease1 == nil || lease1.Holder() == "" {
		t.Fatalf("lease1 inválido ou sem holder")
	}
	if pipelineCtx1.Err() != nil {
		t.Errorf("pipelineCtx1 não deveria estar cancelado")
	}

	// 2. Segunda aquisição concorrente deve ser bloqueada (ErrLockHeld)
	_, _, err = lm.Acquire(ctx)
	if !errors.Is(err, monitoring.ErrLockHeld) {
		t.Errorf("esperava ErrLockHeld para aquisição concorrente, obtido: %v", err)
	}

	// 3. Liberação pelo titular deve ter sucesso
	if err := lease1.Release(ctx); err != nil {
		t.Fatalf("erro ao liberar lease 1: %v", err)
	}

	// 4. Terceira aquisição após liberação deve ter sucesso imediato
	lease2, _, err := lm.Acquire(ctx)
	if err != nil {
		t.Fatalf("esperava sucesso ao adquirir lease 2 após liberação: %v", err)
	}
	if lease2.Holder() == lease1.Holder() {
		t.Errorf("novo lease deve possuir holder distinto do anterior")
	}

	_ = lease2.Release(ctx)
}

func TestLockManagerTakeoverAfterExpiration(t *testing.T) {
	db, ctx := setupTestDB(t)

	currentTime := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	timeFunc := func() time.Time { return currentTime }

	lm1, err := monitoring.NewLockManager(monitoring.LockConfig{
		DB:       db,
		LockName: "expiring_lock",
		TTL:      10 * time.Minute,
		NowFunc:  timeFunc,
	})
	if err != nil {
		t.Fatalf("erro ao criar lm1: %v", err)
	}

	// 1. Adquire lock em T0 (10:00) expirando em T+10m (10:10)
	lease1, _, err := lm1.Acquire(ctx)
	if err != nil {
		t.Fatalf("falha ao adquirir lease 1: %v", err)
	}

	// 2. Tenta adquirir em T+5m (10:05) -> ainda ativo, deve falhar
	currentTime = time.Date(2026, 9, 11, 10, 5, 0, 0, time.UTC)
	_, _, err = lm1.Acquire(ctx)
	if !errors.Is(err, monitoring.ErrLockHeld) {
		t.Errorf("esperava ErrLockHeld em 10:05, obtido %v", err)
	}

	// 3. Avança o tempo para T+15m (10:15) -> expirado, takeover deve ocorrer
	currentTime = time.Date(2026, 9, 11, 10, 15, 0, 0, time.UTC)
	lease2, _, err := lm1.Acquire(ctx)
	if err != nil {
		t.Fatalf("esperava takeover bem-sucedido após expiração: %v", err)
	}
	if lease2.Holder() == lease1.Holder() {
		t.Errorf("lease2 deve possuir holder diferente de lease1")
	}

	// 4. Se o antigo holder (lease1) tentar liberar após ter sido roubado, não deve apagar o lease2
	err = lease1.Release(ctx)
	if !errors.Is(err, monitoring.ErrLockNotHeld) {
		t.Errorf("lease1 deveria retornar ErrLockNotHeld ao tentar liberar lock roubado, obtido %v", err)
	}

	// Verifica se o lock no banco ainda pertence ao lease2
	queries := sqlc.New(db)
	lockRow, err := queries.GetMonitoringLock(ctx, "expiring_lock")
	if err != nil {
		t.Fatalf("falha ao consultar lock no banco: %v", err)
	}
	if lockRow.Holder != lease2.Holder() {
		t.Errorf("lock no banco deveria ser do lease2 (%s), obtido %s", lease2.Holder(), lockRow.Holder)
	}

	_ = lease2.Release(ctx)
}

func TestLockManagerBackgroundRenewalAndContextCancellation(t *testing.T) {
	db, ctx := setupTestDB(t)

	lm, err := monitoring.NewLockManager(monitoring.LockConfig{
		DB:       db,
		LockName: "renew_lock",
		TTL:      300 * time.Millisecond, // TTL curto para teste rápido de renovação
	})
	if err != nil {
		t.Fatalf("erro ao criar LockManager: %v", err)
	}

	lease, pipelineCtx, err := lm.Acquire(ctx)
	if err != nil {
		t.Fatalf("erro ao adquirir lease: %v", err)
	}

	// Aguarda algumas renovações
	time.Sleep(250 * time.Millisecond)

	queries := sqlc.New(db)
	lockRow, err := queries.GetMonitoringLock(ctx, "renew_lock")
	if err != nil {
		t.Fatalf("erro ao consultar lock: %v", err)
	}

	// O updated_at deve ter sido atualizado pela renovação
	if lockRow.UpdatedAt == lockRow.AcquiredAt {
		t.Logf("aviso: renovação ainda não disparou ou ocorreu no mesmo ms")
	}

	if pipelineCtx.Err() != nil {
		t.Errorf("pipelineCtx não deveria estar cancelado enquanto renovação funciona")
	}

	// Simula perda forçada do lock (ex.: intervenção manual no banco ou takeover)
	_, _ = queries.ReleaseMonitoringLock(ctx, sqlc.ReleaseMonitoringLockParams{
		Name:   "renew_lock",
		Holder: lease.Holder(),
	})

	// Aguarda próximo ciclo de renovação detectar a perda do lock
	select {
	case <-pipelineCtx.Done():
		// Sucesso: cancelamento propagado!
	case <-time.After(500 * time.Millisecond):
		t.Errorf("pipelineCtx deveria ter sido cancelado após perda do lock")
	}

	_ = lease.Release(ctx)
}

func TestLockManagerConcurrency(t *testing.T) {
	db, ctx := setupTestDB(t)

	lm, err := monitoring.NewLockManager(monitoring.LockConfig{
		DB:       db,
		LockName: "concurrent_lock",
		TTL:      5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("erro ao criar LockManager: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, _, err := lm.Acquire(ctx)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				_ = lease.Release(ctx)
			}
		}()
	}

	wg.Wait()

	if successCount == 0 {
		t.Errorf("esperava ao menos 1 aquisição bem-sucedida")
	}
}
