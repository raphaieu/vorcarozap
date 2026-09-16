package contradiction

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/normalize"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

var (
	ErrClaimNotFound          = errors.New("contradiction: alegação vinculada não encontrada")
	ErrStatementNotFound      = errors.New("contradiction: manifestação não encontrada")
	ErrMissingExpectedVersion = errors.New("contradiction: versão esperada (expected_updated_at) é obrigatória")
	ErrInvalidTransition      = errors.New("contradiction: transição de estado não permitida")
	ErrInvalidReason          = errors.New("contradiction: justificativa de moderação inválida")
	ErrInvalidActor           = errors.New("contradiction: operador de moderação inválido")
	ErrInvalidAction          = errors.New("contradiction: ação de moderação inválida")
	ErrConflict               = errors.New("contradiction: conflito de concorrência na moderação da manifestação")
)

// SubmitStatementParams encapsula os parâmetros necessários para registrar uma nova manifestação.
type SubmitStatementParams struct {
	ClaimID       string
	StatementType domain.StatementType
	Title         string
	Content       string
	SourceURL     string
	ContactInfo   string
}

// SubmitStatementResult contém o resultado da submissão.
type SubmitStatementResult struct {
	ID        string
	ClaimID   string
	Status    domain.StatementStatus
	CreatedAt string
}

// ModerateStatementParams encapsula os parâmetros necessários para moderar uma manifestação.
type ModerateStatementParams struct {
	StatementID       string
	ExpectedUpdatedAt string
	Action            domain.StatementModerationAction
	Reason            string
	Actor             string
}

// ModerateStatementResult contém o resultado da deliberação de moderação sobre a manifestação.
type ModerateStatementResult struct {
	StatementID    string
	ClaimID        string
	PreviousStatus domain.StatementStatus
	NewStatus      domain.StatementStatus
	Action         domain.StatementModerationAction
	DecisionID     string
	UpdatedAt      string
}

// Service implementa a lógica transacional para manifestações de contraditório e sua moderação.
type Service struct {
	db *sql.DB
}

// NewService inicializa uma nova instância do serviço de contraditório.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// SubmitStatement valida e persiste uma nova manifestação de defesa, garantindo quarentena inicial obrigatória.
func (s *Service) SubmitStatement(ctx context.Context, params SubmitStatementParams) (*SubmitStatementResult, error) {
	// 1. Normalização Unicode NFC e validação estrutural no domínio puro
	rawSub := domain.DefenseStatementSubmission{
		ClaimID:       strings.TrimSpace(params.ClaimID),
		StatementType: params.StatementType,
		Title:         normalize.Unicode(strings.TrimSpace(params.Title)),
		Content:       normalize.Unicode(strings.TrimSpace(params.Content)),
		SourceURL:     strings.TrimSpace(params.SourceURL),
		ContactInfo:   normalize.Unicode(strings.TrimSpace(params.ContactInfo)),
	}

	validSub, err := domain.ValidateStatementSubmission(rawSub)
	if err != nil {
		return nil, fmt.Errorf("contradiction: dados de submissão inválidos: %w", err)
	}

	// 2. Verificar se o claim vinculado existe e está publicado na fronteira pública canônica
	q := sqlc.New(s.db)
	claim, err := q.GetPublicClaimForManifestation(ctx, validSub.ClaimID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrClaimNotFound
		}
		return nil, fmt.Errorf("contradiction: erro ao verificar claim vinculado: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	statementID := uuid.NewString()
	initialStatus := string(domain.StatementStatusQuarantined)

	// 3. Persistir a manifestação com status 'quarantined'
	err = q.InsertDefenseStatement(ctx, sqlc.InsertDefenseStatementParams{
		ID:            statementID,
		ClaimID:       claim.ClaimID,
		StatementType: string(validSub.StatementType),
		Title:         validSub.Title,
		Content:       validSub.Content,
		SourceUrl:     validSub.SourceURL,
		ContactInfo:   validSub.ContactInfo,
		Status:        initialStatus,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return nil, fmt.Errorf("contradiction: erro ao persistir manifestação: %w", err)
	}

	return &SubmitStatementResult{
		ID:        statementID,
		ClaimID:   claim.ClaimID,
		Status:    domain.StatementStatusQuarantined,
		CreatedAt: now,
	}, nil
}

// ModerateStatement executa a deliberação de moderação em uma transação atômica SQLite curta com OCC.
func (s *Service) ModerateStatement(ctx context.Context, params ModerateStatementParams) (*ModerateStatementResult, error) {
	statementID := strings.TrimSpace(params.StatementID)
	if statementID == "" {
		return nil, ErrStatementNotFound
	}

	expectedUpdatedAt := strings.TrimSpace(params.ExpectedUpdatedAt)
	if expectedUpdatedAt == "" {
		return nil, ErrMissingExpectedVersion
	}

	actor, err := domain.ValidateStatementActor(params.Actor)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidActor, err)
	}

	reason, err := domain.ValidateStatementReason(params.Reason)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidReason, err)
	}

	if !params.Action.IsValid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidAction, params.Action)
	}

	var result ModerateStatementResult

	err = store.ExecTx(ctx, s.db, func(q *sqlc.Queries) error {
		// 1. Ler e verificar concorrência otimista dentro da transação
		stmt, err := q.GetDefenseStatementForModeration(ctx, statementID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrStatementNotFound
			}
			return fmt.Errorf("contradiction: falha ao consultar manifestação: %w", err)
		}

		if stmt.UpdatedAt != expectedUpdatedAt {
			return ErrConflict
		}

		currentStatus := domain.StatementStatus(stmt.Status)
		result.PreviousStatus = currentStatus
		result.ClaimID = stmt.ClaimID

		// 2. Validar transição no domínio puro
		nextStatus, err := domain.ValidateStatementTransition(currentStatus, params.Action)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidTransition, err)
		}
		result.NewStatus = nextStatus

		now := time.Now().UTC().Format(time.RFC3339Nano)
		decisionID := uuid.NewString()

		// 3. Gravar decisão imutável de moderação
		err = q.InsertDefenseStatementDecision(ctx, sqlc.InsertDefenseStatementDecisionParams{
			ID:          decisionID,
			StatementID: statementID,
			Action:      string(params.Action),
			Reason:      reason,
			Actor:       actor,
			CreatedAt:   now,
		})
		if err != nil {
			return fmt.Errorf("contradiction: falha ao gravar decisão de moderação: %w", err)
		}

		// 4. Atualizar estado da manifestação com OCC atômico no WHERE
		rowsAffected, err := q.UpdateDefenseStatementStatusWithVersion(ctx, sqlc.UpdateDefenseStatementStatusWithVersionParams{
			NewStatus:         string(nextStatus),
			UpdatedAt:         now,
			ID:                statementID,
			ExpectedUpdatedAt: expectedUpdatedAt,
		})
		if err != nil {
			return fmt.Errorf("contradiction: falha ao atualizar status da manifestação: %w", err)
		}
		if rowsAffected == 0 {
			return ErrConflict
		}

		result.StatementID = statementID
		result.Action = params.Action
		result.DecisionID = decisionID
		result.UpdatedAt = now

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &result, nil
}
