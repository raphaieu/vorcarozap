package moderation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

var (
	ErrClaimNotFound          = errors.New("moderation: alegação não encontrada")
	ErrMissingExpectedVersion = errors.New("moderation: versão esperada (expected_updated_at) é obrigatória")
	ErrInvalidTransition      = errors.New("moderation: transição de estado não permitida")
	ErrInvalidReason          = errors.New("moderation: motivo de moderação inválido")
	ErrInvalidActor           = errors.New("moderation: operador de moderação inválido")
	ErrInvalidAction          = errors.New("moderation: ação de moderação inválida")
	ErrNoActiveSupport        = errors.New("moderation: alegação não possui evidência ativa com papel supports")
	ErrConflict               = errors.New("moderation: conflito de concorrência na moderação da alegação")
)

// ModerateClaimParams encapsula os parâmetros necessários para moderar uma alegação.
type ModerateClaimParams struct {
	ClaimID           string
	ExpectedUpdatedAt string
	Action            domain.ModerationAction
	Reason            string
	Actor             string
}

// ModerateClaimResult contém o resultado da deliberação de moderação.
type ModerateClaimResult struct {
	ClaimID        string
	PreviousStatus domain.ClaimStatus
	NewStatus      domain.ClaimStatus
	Action         domain.ModerationAction
	DecisionID     string
	UpdatedAt      string
}

// Service implementa a lógica transacional e regras de pós-moderação humana.
type Service struct {
	db *sql.DB
}

// NewService cria uma nova instância do serviço de moderação.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// ModerateClaim executa a transição de estado de uma alegação em uma única transação SQLite atômica e curta,
// garantindo revalidação de estado, controle de versão otimista obrigatório, integridade relacional, auditoria em moderation_decisions e bloqueio de candidatos.
func (s *Service) ModerateClaim(ctx context.Context, params ModerateClaimParams) (*ModerateClaimResult, error) {
	claimID := strings.TrimSpace(params.ClaimID)
	if claimID == "" {
		return nil, ErrClaimNotFound
	}

	expectedUpdatedAt := strings.TrimSpace(params.ExpectedUpdatedAt)
	if expectedUpdatedAt == "" {
		return nil, ErrMissingExpectedVersion
	}

	actor, err := domain.ValidateModerationActor(params.Actor)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidActor, err)
	}

	reason, err := domain.ValidateModerationReason(params.Reason)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidReason, err)
	}

	if !params.Action.IsValid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidAction, params.Action)
	}

	var result ModerateClaimResult

	err = store.ExecTx(ctx, s.db, func(q *sqlc.Queries) error {
		// 1. Ler e revalidar estado atual do claim dentro da transação
		claim, err := q.GetClaimByIDForModeration(ctx, claimID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrClaimNotFound
			}
			return fmt.Errorf("moderation: falha ao consultar alegação: %w", err)
		}

		// Revalidar controle de versão otimista obrigatório contra a versão lida no banco
		if claim.UpdatedAt != expectedUpdatedAt {
			return ErrConflict
		}

		currentStatus := domain.ClaimStatus(claim.Status)
		result.PreviousStatus = currentStatus

		// 2. Validar transição permitida via domínio puro contra o estado efetivamente lido no banco
		nextStatus, err := domain.ValidateClaimTransition(currentStatus, params.Action)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidTransition, err)
		}
		result.NewStatus = nextStatus

		// 3. Para approve: verificar que o claim possui ao menos um evidence_source com role = 'supports' e status = 'active'
		if params.Action == domain.ModerationActionApprove {
			activeSupports, err := q.CountActiveSupportsEvidenceSourcesByClaimID(ctx, claimID)
			if err != nil {
				return fmt.Errorf("moderation: falha ao verificar evidências de suporte ativo: %w", err)
			}
			if activeSupports == 0 {
				return ErrNoActiveSupport
			}
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		result.UpdatedAt = now
		result.ClaimID = claimID
		result.Action = params.Action

		// 4. Atualizar status e timestamp da alegação de forma condicional por versão
		rowsAffected, err := q.UpdateClaimStatusWithVersion(ctx, sqlc.UpdateClaimStatusWithVersionParams{
			ID:                claimID,
			NewStatus:         string(nextStatus),
			UpdatedAt:         now,
			ExpectedUpdatedAt: expectedUpdatedAt,
		})
		if err != nil {
			return fmt.Errorf("moderation: falha ao atualizar status da alegação: %w", err)
		}
		if rowsAffected == 0 {
			return ErrConflict
		}

		// 5. Em reject: marcar como rejected todos os monitoring_candidates canônicos (is_duplicate = 0) associados por published_claim_id
		if params.Action == domain.ModerationActionReject {
			_, err = q.RejectMonitoringCandidatesByPublishedClaimID(ctx, sqlc.RejectMonitoringCandidatesByPublishedClaimIDParams{
				PublishedClaimID: sql.NullString{String: claimID, Valid: true},
				UpdatedAt:        now,
			})
			if err != nil {
				return fmt.Errorf("moderation: falha ao rejeitar candidatos de monitoramento associados: %w", err)
			}
		}

		// 6. Consultar fingerprint do candidato associado canônico, se houver
		fingerprint, err := q.GetCandidateFingerprintByPublishedClaimID(ctx, sql.NullString{String: claimID, Valid: true})
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("moderation: falha ao consultar fingerprint do candidato associado: %w", err)
			}
			// Se não houver candidato de monitoramento associado (ex.: carga curada ou inserção direta), fingerprint permanece vazio
			fingerprint = ""
		}

		// 7. Inserir registro auditável em moderation_decisions com restrição XOR
		decisionID := uuid.NewString()
		result.DecisionID = decisionID

		_, err = q.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
			ID:                   decisionID,
			ClaimID:              sql.NullString{String: claimID, Valid: true},
			EvidenceSourceID:     sql.NullString{Valid: false},
			Action:               string(params.Action),
			Reason:               reason,
			Actor:                actor,
			CandidateFingerprint: fingerprint,
			CreatedAt:            now,
		})
		if err != nil {
			return fmt.Errorf("moderation: falha ao registrar decisão de moderação: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &result, nil
}
