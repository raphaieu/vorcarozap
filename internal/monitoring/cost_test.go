package monitoring_test

import (
	"math"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/monitoring"
	"github.com/raphaieu/vorcarozap/internal/research"
)

func TestToMicroUSD(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected int64
	}{
		{"zero", 0.0, 0},
		{"negative", -0.5, 0},
		{"nan", math.NaN(), 0},
		{"inf", math.Inf(1), 0},
		{"exact 1 USD", 1.0, 1000000},
		{"exact 0.25 USD", 0.25, 250000},
		{"exact 0.000001 (1 micro)", 0.000001, 1},
		{"fractional micro ceil (0.0000001 -> 1 micro)", 0.0000001, 1},
		{"fractional micro ceil (0.0000011 -> 2 micros)", 0.0000011, 2},
		{"provider real cost 0.0001234", 0.0001234, 124},
		{"large value 10.50 USD", 10.50, 10500000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitoring.ToMicroUSD(tt.input)
			if got != tt.expected {
				t.Errorf("ToMicroUSD(%v) = %d; esperado %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseCostToMicroUSD_Defensive(t *testing.T) {
	// Valores inválidos retornam erro
	invalidCosts := []float64{
		-0.00001,
		-1.0,
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
	}
	for _, c := range invalidCosts {
		_, err := research.ParseCostToMicroUSD(c)
		if err == nil {
			t.Errorf("ParseCostToMicroUSD(%v) esperava erro, retornou nil", c)
		}
	}

	// Custo zero retorna 0 sem erro
	zeroCost, err := research.ParseCostToMicroUSD(0.0)
	if err != nil || zeroCost != 0 {
		t.Errorf("ParseCostToMicroUSD(0.0) = (%d, %v); esperado (0, nil)", zeroCost, err)
	}

	// Soma de múltiplos custos sub-micro: 10 chamadas de $0.0000001 (0.1 micro-USD cada)
	// Com arredondamento para cima, cada uma vira 1 micro-USD, totalizando 10 micro-USD (nunca subconta para 0)
	var totalMicros int64
	for i := 0; i < 10; i++ {
		m, err := research.ParseCostToMicroUSD(0.0000001)
		if err != nil {
			t.Fatalf("falha ao converter sub-micro: %v", err)
		}
		if m != 1 {
			t.Errorf("esperava sub-micro arredondado para cima para 1, obtido %d", m)
		}
		totalMicros += m
	}
	if totalMicros != 10 {
		t.Errorf("total de 10 sub-micros = %d; esperado 10", totalMicros)
	}
}

func TestMicroUSDToFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected float64
	}{
		{"zero", 0, 0.0},
		{"negative", -10, 0.0},
		{"1 micro", 1, 0.000001},
		{"1000000 micros", 1000000, 1.0},
		{"250000 micros", 250000, 0.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitoring.MicroUSDToFloat(tt.input)
			if got != tt.expected {
				t.Errorf("MicroUSDToFloat(%d) = %v; esperado %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestStartOfUTCDay(t *testing.T) {
	// Teste com fuso horário diferente de UTC (ex: UTC-3 Brasil)
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.FixedZone("BRT", -3*3600)
	}

	// 2026-09-11 22:30:00 BRT = 2026-09-12 01:30:00 UTC
	brtTime := time.Date(2026, 9, 11, 22, 30, 0, 0, loc)
	startUTC := monitoring.StartOfUTCDay(brtTime)

	expectedStart := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	if !startUTC.Equal(expectedStart) {
		t.Errorf("StartOfUTCDay(%v) = %v; esperado %v", brtTime, startUTC, expectedStart)
	}

	// Início exato do dia 2026-09-11 00:00:00 UTC
	utcExact := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	if !monitoring.StartOfUTCDay(utcExact).Equal(utcExact) {
		t.Errorf("StartOfUTCDay no exato início do dia diverge")
	}

	// 23:59:59.999999999 UTC pertence ao mesmo dia
	utcEnd := time.Date(2026, 9, 11, 23, 59, 59, 999999999, time.UTC)
	if !monitoring.StartOfUTCDay(utcEnd).Equal(utcExact) {
		t.Errorf("StartOfUTCDay no final do dia diverge: %v != %v", monitoring.StartOfUTCDay(utcEnd), utcExact)
	}
}

func TestBudgetTracker(t *testing.T) {
	t.Run("Daily budget already exhausted before discovery", func(t *testing.T) {
		tracker := monitoring.NewBudgetTracker(250000, 1000000, 1000000, 10) // 1.00 USD já gasto
		canDiscover, reason := tracker.CanAffordDiscovery()
		if canDiscover {
			t.Errorf("esperava CanAffordDiscovery = false quando gasto prévio >= limite diário")
		}
		if reason == "" {
			t.Errorf("esperava motivo técnico preenchido")
		}
	})

	t.Run("Discovery cost exceeds run budget", func(t *testing.T) {
		tracker := monitoring.NewBudgetTracker(250000, 1000000, 0, 10)
		canDiscover, _ := tracker.CanAffordDiscovery()
		if !canDiscover {
			t.Fatalf("esperava poder fazer descoberta")
		}

		tracker.RecordDiscovery(260000) // 0.26 USD excedeu teto do run (0.25 USD = 250000 micros)
		canVerify, reason := tracker.CanAffordVerification()
		if canVerify {
			t.Errorf("esperava CanAffordVerification = false após descoberta exceder orçamento do run")
		}
		if reason == "" {
			t.Errorf("esperava motivo de interrupção")
		}
	})

	t.Run("Max verifications limit stops execution", func(t *testing.T) {
		tracker := monitoring.NewBudgetTracker(1000000, 5000000, 0, 2)
		tracker.RecordDiscovery(10000)

		can1, _ := tracker.CanAffordVerification()
		if !can1 {
			t.Fatalf("esperava poder executar verificação 1")
		}
		tracker.RecordVerification(10000)

		can2, _ := tracker.CanAffordVerification()
		if !can2 {
			t.Fatalf("esperava poder executar verificação 2")
		}
		tracker.RecordVerification(10000)

		can3, reason := tracker.CanAffordVerification()
		if can3 {
			t.Errorf("esperava CanAffordVerification = false após atingir maxVerifications=2")
		}
		if reason == "" {
			t.Errorf("esperava motivo de limite de verificações")
		}
		if tracker.VerificationsExecuted() != 2 {
			t.Errorf("esperava 2 verificações executadas, obtido %d", tracker.VerificationsExecuted())
		}
	})

	t.Run("Daily budget limit reached during verifications", func(t *testing.T) {
		tracker := monitoring.NewBudgetTracker(500000, 1000000, 900000, 10) // Restam 100.000 micros no dia
		tracker.RecordDiscovery(50000)                                      // Restam 50.000 micros

		can1, _ := tracker.CanAffordVerification()
		if !can1 {
			t.Fatalf("esperava poder executar verificação 1")
		}
		tracker.RecordVerification(60000) // Total diário agora: 900.000 + 50.000 + 60.000 = 1.010.000 > 1.000.000

		can2, reason := tracker.CanAffordVerification()
		if can2 {
			t.Errorf("esperava CanAffordVerification = false após estourar orçamento diário")
		}
		if reason == "" {
			t.Errorf("esperava motivo de limite diário")
		}
	})
}
