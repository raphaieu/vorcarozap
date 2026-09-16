package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// AdminDefenseStatementFilter encapsula os parâmetros de busca e paginação de manifestações no painel.
type AdminDefenseStatementFilter struct {
	Status      string
	Type        string
	ClaimID     string
	Period      string
	PeriodSince string
	Page        int
	PageSize    int
}

// AdminDefenseStatementsResult contém a lista paginada e os metadados.
type AdminDefenseStatementsResult struct {
	Statements []sqlc.ListAdminDefenseStatementsRow
	TotalCount int64
	Page       int
	PageSize   int
	TotalPages int
}

// ListAdminDefenseStatements executa a listagem paginada com filtros dinâmicos no SQLite.
func ListAdminDefenseStatements(ctx context.Context, db *sql.DB, f AdminDefenseStatementFilter) (*AdminDefenseStatementsResult, error) {
	page := f.Page
	if page < 1 {
		page = 1
	}
	pageSize := f.PageSize
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var conditions []string
	var args []any

	if f.Status != "" {
		conditions = append(conditions, "ds.status = ?")
		args = append(args, f.Status)
	}

	if f.Type != "" {
		conditions = append(conditions, "ds.statement_type = ?")
		args = append(args, f.Type)
	}

	if f.ClaimID != "" {
		conditions = append(conditions, "ds.claim_id = ?")
		args = append(args, f.ClaimID)
	}

	if f.PeriodSince != "" {
		conditions = append(conditions, "ds.created_at >= ?")
		args = append(args, f.PeriodSince)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Contagem total
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM defense_statements ds
		JOIN claims c ON c.id = ds.claim_id
		JOIN relationships r ON r.id = c.relationship_id
		JOIN entities e ON e.id = r.subject_entity_id
		%s;
	`, whereClause)

	var totalCount int64
	if err := db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("store: falha ao contar manifestações: %w", err)
	}

	// Consulta paginada
	selectQuery := fmt.Sprintf(`
		SELECT
			ds.id,
			ds.claim_id,
			ds.statement_type,
			ds.title,
			ds.content,
			ds.source_url,
			ds.contact_info,
			ds.status,
			ds.created_at,
			ds.updated_at,
			c.proposition AS claim_proposition,
			c.grade AS claim_grade,
			c.status AS claim_status,
			e.name AS entity_name,
			e.slug AS entity_slug
		FROM defense_statements ds
		JOIN claims c ON c.id = ds.claim_id
		JOIN relationships r ON r.id = c.relationship_id
		JOIN entities e ON e.id = r.subject_entity_id
		%s
		ORDER BY ds.created_at DESC, ds.id DESC
		LIMIT ? OFFSET ?;
	`, whereClause)

	selectArgs := append(args, pageSize, offset)
	rows, err := db.QueryContext(ctx, selectQuery, selectArgs...)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar manifestações: %w", err)
	}
	defer rows.Close()

	var statements []sqlc.ListAdminDefenseStatementsRow
	for rows.Next() {
		var row sqlc.ListAdminDefenseStatementsRow
		if err := rows.Scan(
			&row.ID,
			&row.ClaimID,
			&row.StatementType,
			&row.Title,
			&row.Content,
			&row.SourceUrl,
			&row.ContactInfo,
			&row.Status,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.ClaimProposition,
			&row.ClaimGrade,
			&row.ClaimStatus,
			&row.EntityName,
			&row.EntitySlug,
		); err != nil {
			return nil, fmt.Errorf("store: falha ao escanear linha de manifestação: %w", err)
		}
		statements = append(statements, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: erro na iteração de manifestações: %w", err)
	}

	totalPages := int((totalCount + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		totalPages = 1
	}

	return &AdminDefenseStatementsResult{
		Statements: statements,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetAdminDefenseStatementDetail recupera a inspeção completa de uma manifestação e seu histórico de moderação.
func GetAdminDefenseStatementDetail(ctx context.Context, db *sql.DB, id string) (*domain.AdminDefenseStatementDetail, error) {
	q := sqlc.New(db)
	stmt, err := q.GetDefenseStatementByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: falha ao buscar manifestação: %w", err)
	}

	rawDecisions, err := q.ListDefenseStatementDecisionsByStatementID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao buscar decisões da manifestação: %w", err)
	}

	var decisions []domain.DefenseStatementDecision
	for _, d := range rawDecisions {
		act := domain.StatementModerationAction(d.Action)
		decisions = append(decisions, domain.DefenseStatementDecision{
			ID:          d.ID,
			StatementID: d.StatementID,
			Action:      act,
			ActionLabel: act.Label(),
			Reason:      d.Reason,
			Actor:       d.Actor,
			CreatedAt:   d.CreatedAt,
		})
	}

	stType := domain.StatementType(stmt.StatementType)
	stStatus := domain.StatementStatus(stmt.Status)

	return &domain.AdminDefenseStatementDetail{
		ID:                 stmt.ID,
		ClaimID:            stmt.ClaimID,
		ClaimProposition:   stmt.ClaimProposition,
		ClaimGrade:         domain.EvidenceGrade(stmt.ClaimGrade),
		ClaimStatus:        domain.ClaimStatus(stmt.ClaimStatus),
		SubjectEntityName:  stmt.EntityName,
		SubjectEntitySlug:  stmt.EntitySlug,
		StatementType:      stType,
		StatementTypeLabel: stType.Label(),
		Title:              stmt.Title,
		Content:            stmt.Content,
		SourceURL:          stmt.SourceUrl,
		ContactInfo:        stmt.ContactInfo,
		Status:             stStatus,
		StatusLabel:        stStatus.Label(),
		CreatedAt:          stmt.CreatedAt,
		UpdatedAt:          stmt.UpdatedAt,
		Decisions:          decisions,
	}, nil
}

// ListPublicDefenseStatementsForClaim recupera todas as manifestações aceitas de um claim publicado.
func ListPublicDefenseStatementsForClaim(ctx context.Context, db *sql.DB, claimID string) ([]domain.PublicDefenseStatement, error) {
	q := sqlc.New(db)
	rows, err := q.ListPublicDefenseStatementsByClaimID(ctx, claimID)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar manifestações públicas: %w", err)
	}

	var list []domain.PublicDefenseStatement
	for _, r := range rows {
		stType := domain.StatementType(r.StatementType)
		list = append(list, domain.PublicDefenseStatement{
			ID:                 r.ID,
			ClaimID:            r.ClaimID,
			StatementType:      stType,
			StatementTypeLabel: stType.Label(),
			Title:              r.Title,
			Content:            r.Content,
			SourceURL:          r.SourceUrl,
			CreatedAt:          r.CreatedAt,
		})
	}
	return list, nil
}
