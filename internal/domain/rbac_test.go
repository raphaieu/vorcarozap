package domain_test

import (
	"strings"
	"testing"

	"github.com/raphaieu/vorcarozap/internal/domain"
)

func TestUserRole_IsValid(t *testing.T) {
	validRoles := []domain.UserRole{
		domain.RoleAdmin,
		domain.RoleEditor,
		domain.RoleReviewer,
		domain.RoleAuditor,
	}

	for _, r := range validRoles {
		if !r.IsValid() {
			t.Errorf("esperava que o papel %q fosse válido", r)
		}
	}

	invalidRoles := []domain.UserRole{
		"",
		"superadmin",
		"guest",
		"ADMIN",
	}

	for _, r := range invalidRoles {
		if r.IsValid() {
			t.Errorf("esperava que o papel %q fosse inválido", r)
		}
	}
}

func TestUserRole_HasPermission_CompleteMatrix(t *testing.T) {
	allPermissions := []domain.Permission{
		domain.PermViewDashboard,
		domain.PermViewCandidates,
		domain.PermViewSources,
		domain.PermViewEvidences,
		domain.PermViewClaims,
		domain.PermViewManifestations,
		domain.PermViewManifestationContact,
		domain.PermModerateClaims,
		domain.PermModerateEvidenceSources,
		domain.PermModerateManifestations,
		domain.PermManageUsers,
		domain.PermViewAuditLogs,
	}

	for _, p := range allPermissions {
		if !p.IsValid() {
			t.Fatalf("permissão %q deveria ser válida", p)
		}
	}

	tests := []struct {
		name        string
		role        domain.UserRole
		expectedMap map[domain.Permission]bool
	}{
		{
			name: "Admin possui todas as permissões",
			role: domain.RoleAdmin,
			expectedMap: map[domain.Permission]bool{
				domain.PermViewDashboard:            true,
				domain.PermViewCandidates:           true,
				domain.PermViewSources:              true,
				domain.PermViewEvidences:            true,
				domain.PermViewClaims:               true,
				domain.PermViewManifestations:       true,
				domain.PermViewManifestationContact: true,
				domain.PermModerateClaims:           true,
				domain.PermModerateEvidenceSources:  true,
				domain.PermModerateManifestations:   true,
				domain.PermManageUsers:              true,
				domain.PermViewAuditLogs:            true,
			},
		},
		{
			name: "Editor possui permissões editoriais completas e visualização de contatos, sem gestão de usuários nem auditoria restrita",
			role: domain.RoleEditor,
			expectedMap: map[domain.Permission]bool{
				domain.PermViewDashboard:            true,
				domain.PermViewCandidates:           true,
				domain.PermViewSources:              true,
				domain.PermViewEvidences:            true,
				domain.PermViewClaims:               true,
				domain.PermViewManifestations:       true,
				domain.PermViewManifestationContact: true,
				domain.PermModerateClaims:           true,
				domain.PermModerateEvidenceSources:  true,
				domain.PermModerateManifestations:   true,
				domain.PermManageUsers:              false,
				domain.PermViewAuditLogs:            false,
			},
		},
		{
			name: "Reviewer possui leitura ampla e visualização de contatos, sem mutações",
			role: domain.RoleReviewer,
			expectedMap: map[domain.Permission]bool{
				domain.PermViewDashboard:            true,
				domain.PermViewCandidates:           true,
				domain.PermViewSources:              true,
				domain.PermViewEvidences:            true,
				domain.PermViewClaims:               true,
				domain.PermViewManifestations:       true,
				domain.PermViewManifestationContact: true,
				domain.PermModerateClaims:           false,
				domain.PermModerateEvidenceSources:  false,
				domain.PermModerateManifestations:   false,
				domain.PermManageUsers:              false,
				domain.PermViewAuditLogs:            false,
			},
		},
		{
			name: "Auditor possui leitura ampla e auditoria, sem contato de terceiros nem mutações",
			role: domain.RoleAuditor,
			expectedMap: map[domain.Permission]bool{
				domain.PermViewDashboard:            true,
				domain.PermViewCandidates:           true,
				domain.PermViewSources:              true,
				domain.PermViewEvidences:            true,
				domain.PermViewClaims:               true,
				domain.PermViewManifestations:       true,
				domain.PermViewManifestationContact: false,
				domain.PermModerateClaims:           false,
				domain.PermModerateEvidenceSources:  false,
				domain.PermModerateManifestations:   false,
				domain.PermManageUsers:              false,
				domain.PermViewAuditLogs:            true,
			},
		},
		{
			name: "Papel desconhecido não possui nenhuma permissão",
			role: domain.UserRole("desconhecido"),
			expectedMap: map[domain.Permission]bool{
				domain.PermViewDashboard:            false,
				domain.PermViewCandidates:           false,
				domain.PermViewSources:              false,
				domain.PermViewEvidences:            false,
				domain.PermViewClaims:               false,
				domain.PermViewManifestations:       false,
				domain.PermViewManifestationContact: false,
				domain.PermModerateClaims:           false,
				domain.PermModerateEvidenceSources:  false,
				domain.PermModerateManifestations:   false,
				domain.PermManageUsers:              false,
				domain.PermViewAuditLogs:            false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, perm := range allPermissions {
				expected := tt.expectedMap[perm]
				got := tt.role.HasPermission(perm)
				if got != expected {
					t.Errorf("role %q, permission %q: esperado %v, obtido %v", tt.role, perm, expected, got)
				}
			}
		})
	}
}

func TestUserRole_RequiresMFA(t *testing.T) {
	if !domain.RoleAdmin.RequiresMFA() {
		t.Errorf("RoleAdmin deveria exigir MFA")
	}
	if !domain.RoleEditor.RequiresMFA() {
		t.Errorf("RoleEditor deveria exigir MFA")
	}
	if domain.RoleReviewer.RequiresMFA() {
		t.Errorf("RoleReviewer não deveria exigir MFA compulsoriamente")
	}
	if domain.RoleAuditor.RequiresMFA() {
		t.Errorf("RoleAuditor não deveria exigir MFA compulsoriamente")
	}
}

func TestUserStatus_Validation(t *testing.T) {
	validStatuses := []domain.UserStatus{
		domain.UserStatusActive,
		domain.UserStatusDisabled,
		domain.UserStatusLocked,
	}

	for _, s := range validStatuses {
		if !s.IsValid() {
			t.Errorf("status %q deveria ser válido", s)
		}
	}

	if !domain.UserStatusActive.CanAuthenticate() {
		t.Errorf("status active deveria poder autenticar")
	}
	if domain.UserStatusDisabled.CanAuthenticate() {
		t.Errorf("status disabled NÃO deveria poder autenticar")
	}
	if domain.UserStatusLocked.CanAuthenticate() {
		t.Errorf("status locked NÃO deveria poder autenticar")
	}
}

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		input       string
		expected    string
		shouldError bool
	}{
		{"admin", "admin", false},
		{"Admin.Editor", "admin.editor", false},
		{"  joao_silva-01  ", "joao_silva-01", false},
		{"ab", "", true},                    // curto demais
		{strings.Repeat("a", 65), "", true}, // longo demais
		{"user:name", "", true},             // dois-pontos proibido
		{"user name", "", true},             // espaço interno proibido
		{"user@mail", "", true},             // @ proibido
		{"", "", true},                      // vazio
	}

	for _, tt := range tests {
		got, err := domain.ValidateUsername(tt.input)
		if (err != nil) != tt.shouldError {
			t.Errorf("ValidateUsername(%q): erro esperado %v, obtido: %v", tt.input, tt.shouldError, err)
		}
		if !tt.shouldError && got != tt.expected {
			t.Errorf("ValidateUsername(%q): esperado %q, obtido %q", tt.input, tt.expected, got)
		}
	}
}

func TestValidateDisplayName(t *testing.T) {
	tests := []struct {
		input       string
		expected    string
		shouldError bool
	}{
		{"Raphael Vorcaro", "Raphael Vorcaro", false},
		{"  Ana Silva  ", "Ana Silva", false},
		{"A", "", true},                         // curto demais
		{strings.Repeat("a", 129), "", true},    // longo demais
		{"<script>alert(1)</script>", "", true}, // tags proibidas
		{"", "", true},
	}

	for _, tt := range tests {
		got, err := domain.ValidateDisplayName(tt.input)
		if (err != nil) != tt.shouldError {
			t.Errorf("ValidateDisplayName(%q): erro esperado %v, obtido: %v", tt.input, tt.shouldError, err)
		}
		if !tt.shouldError && got != tt.expected {
			t.Errorf("ValidateDisplayName(%q): esperado %q, obtido %q", tt.input, tt.expected, got)
		}
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	if err := domain.ValidatePasswordStrength("curta"); err == nil {
		t.Errorf("senha curta deveria retornar erro")
	}
	if err := domain.ValidatePasswordStrength("senha-longa-segura-123"); err != nil {
		t.Errorf("senha válida retornou erro: %v", err)
	}
}
