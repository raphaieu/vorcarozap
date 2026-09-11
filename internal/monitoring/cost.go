package monitoring

import (
	"fmt"
	"time"

	"github.com/raphaieu/vorcarozap/internal/research"
)

// Constante de conversão para micro-USD ($1.00 USD = 1.000.000 micro-USD).
const MicroUSDMultiplier = research.MicroUSDMultiplier

// ToMicroUSD converte um valor monetário em USD (float64) para micro-USD inteiros com arredondamento para cima.
func ToMicroUSD(costUSD float64) int64 {
	micros, err := research.ParseCostToMicroUSD(costUSD)
	if err != nil {
		return 0
	}
	return micros
}

// MicroUSDToFloat converte o valor em micro-USD de volta para float64 USD.
func MicroUSDToFloat(micros int64) float64 {
	return research.MicroUSDToFloat(micros)
}

// FormatUSD formata um valor em USD com precisão monetária legível para CLI e relatórios.
func FormatUSD(costUSD float64) string {
	if costUSD == 0.0 {
		return "$0.00"
	}
	if costUSD < 0.01 {
		return fmt.Sprintf("$%.6f", costUSD)
	}
	return fmt.Sprintf("$%.4f", costUSD)
}

// FormatMicroUSD formata um valor em micro-USD para exibição amigável.
func FormatMicroUSD(micros int64) string {
	return FormatUSD(MicroUSDToFloat(micros))
}

// StartOfUTCDay retorna o instante inicial 00:00:00.000000000Z do dia UTC correspondente a t.
func StartOfUTCDay(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

// BudgetTracker gerencia o controle determinístico de orçamento em micro-USD inteiros por execução e por dia.
type BudgetTracker struct {
	maxRunCostMicros       int64
	maxDayCostMicros       int64
	dailyPriorSpentMicros  int64
	currentRunCostMicros   int64
	discoveryCostMicros    int64
	verificationCostMicros int64
	maxVerifications       int
	verificationsExecuted  int
}

// NewBudgetTracker inicializa um BudgetTracker com os limites configurados e gasto prévio diário em micro-USD inteiros.
func NewBudgetTracker(maxRunCostMicros, maxDayCostMicros int64, dailyPriorSpentMicros int64, maxVerifications int) *BudgetTracker {
	return &BudgetTracker{
		maxRunCostMicros:      maxRunCostMicros,
		maxDayCostMicros:      maxDayCostMicros,
		dailyPriorSpentMicros: dailyPriorSpentMicros,
		maxVerifications:      maxVerifications,
	}
}

// NewBudgetTrackerFromUSD inicializa um BudgetTracker convertendo valores USD float64 para micro-USD.
func NewBudgetTrackerFromUSD(maxRunUSD, maxDayUSD float64, dailyPriorSpentMicros int64, maxVerifications int) *BudgetTracker {
	return NewBudgetTracker(ToMicroUSD(maxRunUSD), ToMicroUSD(maxDayUSD), dailyPriorSpentMicros, maxVerifications)
}

// CanAffordDiscovery verifica se o orçamento diário inicial já não está esgotado antes da descoberta.
func (b *BudgetTracker) CanAffordDiscovery() (bool, string) {
	if b.maxDayCostMicros > 0 && b.dailyPriorSpentMicros >= b.maxDayCostMicros {
		return false, fmt.Sprintf("Orçamento diário esgotado antes da descoberta: gasto prévio de %s excede limite de %s",
			FormatMicroUSD(b.dailyPriorSpentMicros), FormatMicroUSD(b.maxDayCostMicros))
	}
	return true, ""
}

// RecordDiscovery registra o custo real da etapa de descoberta em micro-USD inteiros.
func (b *BudgetTracker) RecordDiscovery(costMicros int64) {
	if costMicros < 0 {
		return
	}
	b.discoveryCostMicros += costMicros
	b.currentRunCostMicros += costMicros
}

// CanAffordVerification verifica se uma nova verificação semântica pode ser executada dentro dos limites.
func (b *BudgetTracker) CanAffordVerification() (bool, string) {
	if b.maxVerifications > 0 && b.verificationsExecuted >= b.maxVerifications {
		return false, fmt.Sprintf("Limite máximo de verificações por run atingido (%d)", b.maxVerifications)
	}

	if b.maxRunCostMicros > 0 && b.currentRunCostMicros >= b.maxRunCostMicros {
		return false, fmt.Sprintf("Limite de orçamento por run atingido: gasto atual de %s alcançou limite de %s",
			FormatMicroUSD(b.currentRunCostMicros), FormatMicroUSD(b.maxRunCostMicros))
	}

	totalDay := b.dailyPriorSpentMicros + b.currentRunCostMicros
	if b.maxDayCostMicros > 0 && totalDay >= b.maxDayCostMicros {
		return false, fmt.Sprintf("Limite de orçamento diário atingido: gasto acumulado de %s alcançou limite de %s",
			FormatMicroUSD(totalDay), FormatMicroUSD(b.maxDayCostMicros))
	}

	return true, ""
}

// RecordVerification registra o custo real de uma verificação semântica executada em micro-USD inteiros.
func (b *BudgetTracker) RecordVerification(costMicros int64) {
	if costMicros < 0 {
		return
	}
	b.verificationCostMicros += costMicros
	b.currentRunCostMicros += costMicros
	b.verificationsExecuted++
}

// CurrentRunCostMicros retorna o custo total acumulado do run em micro-USD inteiros.
func (b *BudgetTracker) CurrentRunCostMicros() int64 {
	return b.currentRunCostMicros
}

// CurrentRunCostUSD retorna o custo total acumulado do run em USD decimal.
func (b *BudgetTracker) CurrentRunCostUSD() float64 {
	return MicroUSDToFloat(b.currentRunCostMicros)
}

// DiscoveryCostMicros retorna o custo de descoberta em micro-USD inteiros.
func (b *BudgetTracker) DiscoveryCostMicros() int64 {
	return b.discoveryCostMicros
}

// DiscoveryCostUSD retorna o custo de descoberta em USD decimal.
func (b *BudgetTracker) DiscoveryCostUSD() float64 {
	return MicroUSDToFloat(b.discoveryCostMicros)
}

// VerificationCostMicros retorna o custo de verificações em micro-USD inteiros.
func (b *BudgetTracker) VerificationCostMicros() int64 {
	return b.verificationCostMicros
}

// VerificationCostUSD retorna o custo das verificações em USD decimal.
func (b *BudgetTracker) VerificationCostUSD() float64 {
	return MicroUSDToFloat(b.verificationCostMicros)
}

// TotalDayCostMicros retorna o gasto diário acumulado (gasto prévio + run atual) em micro-USD inteiros.
func (b *BudgetTracker) TotalDayCostMicros() int64 {
	return b.dailyPriorSpentMicros + b.currentRunCostMicros
}

// TotalDayCostUSD retorna o gasto diário acumulado em USD decimal.
func (b *BudgetTracker) TotalDayCostUSD() float64 {
	return MicroUSDToFloat(b.TotalDayCostMicros())
}

// DailyPriorSpentMicros retorna o gasto prévio diário em micro-USD.
func (b *BudgetTracker) DailyPriorSpentMicros() int64 {
	return b.dailyPriorSpentMicros
}

// VerificationsExecuted retorna a contagem de verificações executadas.
func (b *BudgetTracker) VerificationsExecuted() int {
	return b.verificationsExecuted
}
