package contradiction_test

import (
	"sync"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/contradiction"
)

func TestRateLimiter(t *testing.T) {
	limiter := contradiction.NewRateLimiter(3, 1*time.Minute)
	now := time.Now().UTC()

	// 1. Primeiras 3 requisições devem passar
	for i := 1; i <= 3; i++ {
		if !limiter.AllowAt("192.168.1.1", now.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("requisição %d deveria ser permitida", i)
		}
	}

	// 2. 4ª requisição no mesmo minuto deve ser bloqueada
	if limiter.AllowAt("192.168.1.1", now.Add(4*time.Second)) {
		t.Fatalf("4ª requisição deveria ser bloqueada pelo rate limit")
	}

	// 3. Outro IP deve passar normalmente (isolamento de chaves)
	if !limiter.AllowAt("192.168.1.2", now.Add(5*time.Second)) {
		t.Fatalf("requisição de outro IP deveria ser permitida")
	}

	// 4. Após expirar a janela temporal, nova requisição deve passar
	future := now.Add(65 * time.Second)
	if !limiter.AllowAt("192.168.1.1", future) {
		t.Fatalf("requisição após expiração da janela deveria ser permitida")
	}

	// 5. Reset deve liberar o IP imediatamente
	limiter.Reset("192.168.1.1")
	if !limiter.AllowAt("192.168.1.1", now) {
		t.Fatalf("requisição após reset deveria ser permitida")
	}
}

func TestRateLimiterConcurrency(t *testing.T) {
	limiter := contradiction.NewRateLimiter(50, 1*time.Minute)
	var wg sync.WaitGroup
	allowedCount := 0
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.Allow("concurrent_ip") {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if allowedCount != 50 {
		t.Fatalf("esperado exatamente 50 requisições permitidas concorrentemente, obtido: %d", allowedCount)
	}
}
