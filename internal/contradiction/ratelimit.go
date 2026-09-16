package contradiction

import (
	"sync"
	"time"
)

// RateLimiter implementa um limitador de taxa em memória por chave (ex: IP do cliente)
// utilizando janela deslizante com limpeza automática, seguro para concorrência.
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	records map[string][]time.Time
}

// NewRateLimiter inicializa um novo limitador de taxa.
// limit: quantidade máxima de requisições permitidas dentro da janela.
// window: duração da janela temporal.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 5
	}
	if window <= 0 {
		window = 10 * time.Minute
	}
	return &RateLimiter{
		limit:   limit,
		window:  window,
		records: make(map[string][]time.Time),
	}
}

// Allow verifica se a chave fornecida pode executar uma nova requisição.
// Retorna true se a requisição for permitida; false se excedeu o limite.
func (r *RateLimiter) Allow(key string) bool {
	return r.AllowAt(key, time.Now().UTC())
}

// AllowAt verifica a permissão em um timestamp específico (útil para testes determinísticos).
func (r *RateLimiter) AllowAt(key string, now time.Time) bool {
	if key == "" {
		key = "unknown"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := now.Add(-r.window)

	// Filtra registros da chave dentro da janela
	timestamps := r.records[key]
	var validTimestamps []time.Time
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			validTimestamps = append(validTimestamps, ts)
		}
	}

	if len(validTimestamps) >= r.limit {
		r.records[key] = validTimestamps
		return false
	}

	// Adiciona o carimbo atual
	validTimestamps = append(validTimestamps, now)
	r.records[key] = validTimestamps

	// Limpeza periódica leve de chaves vazias ou expiradas
	if len(r.records) > 1000 {
		for k, v := range r.records {
			var remaining []time.Time
			for _, ts := range v {
				if ts.After(cutoff) {
					remaining = append(remaining, ts)
				}
			}
			if len(remaining) == 0 {
				delete(r.records, k)
			} else {
				r.records[k] = remaining
			}
		}
	}

	return true
}

// Reset limpa os registros de uma chave específica (útil para testes).
func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, key)
}
