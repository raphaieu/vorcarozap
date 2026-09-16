# ADR-025: Gestão Multiusuário do Painel Administrativo com RBAC, MFA (TOTP) e Sessões Seguras

## Status

Aceito

## Contexto

O VorcaroZAP adotou inicialmente no MVP uma proteção simplificada para a rota `/admin` baseada em HTTP Basic Authentication acoplada a um único par de credenciais configuradas em variáveis de ambiente (`ADMIN_USER` e `ADMIN_PASSWORD_HASH`), conforme formalizado no [ADR-016](ADR-016-protecao-simples-do-admin-por-basic-auth-e-bcrypt.md).

Com a evolução editorial da plataforma — incorporando moderação granular de claims ([ADR-017](ADR-017-moderacao-humana-de-claims-transacoes-e-protecao-csrf.md)), moderação de evidence_sources com quarentena por perda de suporte ([ADR-018](ADR-018-moderacao-granular-de-evidence-sources-e-quarentena-por-perda-de-suporte.md)), histórico de auditoria editorial ([ADR-023](ADR-023-historico-publico-de-alteracoes-editoriais-e-trilha-de-transparencia.md)) e recepção e triagem de manifestações de contraditório ([ADR-024](ADR-024-contraditorio-estruturado-e-submissao-publica-de-manifestacoes.md)) —, a existência de uma credencial administrativa única torna-se um gargalo operacional e de conformidade:
1. **Atribuição individual de responsabilidade:** Decisões editoriais e deliberações de moderação precisam registrar de maneira inequívoca o operador real (`actor`) no banco SQLite, eliminando o compartilhamento de senhas;
2. **Princípio do menor privilégio (PoLP):** Diferentes membros da equipe desempenham papéis distintos (administração técnica, moderação editorial, revisão consultiva, auditoria independente);
3. **Autenticação reforçada (MFA):** Operações com poder de alteração pública ou descarte editorial demandam comprovação por segundo fator independente (Time-based One-Time Password - TOTP);
4. **Ciclo de vida de sessões e controle de acesso:** Necessidade de revogação imediata de acessos, bloqueio contra ataques de força bruta (lockout), cookies seguros com rotação e trilha de auditoria administrativa completa.

## Decisão

Implementa-se no monólito Go e banco SQLite a gestão multiusuário individual do painel administrativo com controle de acesso baseado em funções (RBAC), autenticação reforçada por MFA TOTP (RFC 6238), gerenciamento de sessões com cookies seguros e rotação, proteção anti-brute-force com lockout e trilha imutável de auditoria administrativa.

### 1. Modelo de Dados de Usuários e Sessões

Criam-se três tabelas relacionais dedicadas no SQLite WAL:
- **`admin_users`**: Armazena as contas administrativas com campos para identidade (`id`, `username` normalizado minúsculo e único, `display_name`), credenciais (`password_hash` com bcrypt custo 12..14), controle de acesso (`role`), estado (`status` IN `'active'`, `'disabled'`, `'locked'`), segurança de autenticação (`failed_login_attempts`, `locked_until`), MFA com criptografia em repouso (`mfa_enabled`, `mfa_secret_encrypted`, `mfa_pending_secret_encrypted`, `mfa_pending_expires_at`, `mfa_enrolled_at`), telemetria de login (`last_login_at`) e timestamps UTC ISO-8601 (`created_at`, `updated_at`).
- **`admin_sessions`**: Armazena sessões ativas do servidor indexadas por tokens criptográficos aleatórios de 256 bits (32 bytes em hexadecimal), vinculadas ao `user_id` (`ON DELETE CASCADE`), registrando elevação de segundo fator (`mfa_verified`), telemetria (`ip_address`, `user_agent`), expiração UTC (`expires_at`), última atividade (`last_activity_at`) e data de criação (`created_at`).
- **`admin_audit_logs`**: Tabela imutável de auditoria de eventos administrativos e de autenticação (`id`, `user_id`, `username`, `action`, `actor_id`, `actor_username`, `target_id`, `details`, `ip_address`, `created_at`), garantindo rastreabilidade perene de logins, falhas, bloqueios, criação de usuários, alterações de papel, redefinições de senha e desativações.

### 2. Papéis (Roles) e Matriz Granular de Permissões (RBAC)

Definem-se quatro papéis institucionais bem delimitados, mapeados diretamente em Go puro no pacote `internal/domain`:
- **`admin`**: Gestão integral de usuários, configurações de segurança, auditoria completa e todas as ações editoriais;
- **`editor`**: Moderação editorial ativa de claims, evidence_sources e manifestações de defesa/contraditório;
- **`reviewer`**: Inspeção e revisão administrativa consultiva ampla, incluindo visualização de dados de contato de manifestações, sem permissão para executar mutações destrutivas ou aprovações;
- **`auditor`**: Inspeção estritamente consultiva da base e leitura da trilha de auditoria privada, sem acesso a dados pessoais de contato de terceiros nem execução de mutações.

A matriz explícita de permissões é verificada no servidor em cada endpoint:

| Permissão (`Permission`) | Descrição | `admin` | `editor` | `reviewer` | `auditor` |
|---|---|:---:|:---:|:---:|:---:|
| `PermViewDashboard` | Visualização do dashboard e estatísticas | Sim | Sim | Sim | Sim |
| `PermViewCandidates` | Listagem e inspeção de candidatos do monitoramento | Sim | Sim | Sim | Sim |
| `PermViewSources` | Listagem e inspeção de fontes e sequências | Sim | Sim | Sim | Sim |
| `PermViewEvidences` | Inspeção de evidências e usos | Sim | Sim | Sim | Sim |
| `PermViewClaims` | Inspeção detalhada de alegações | Sim | Sim | Sim | Sim |
| `PermViewManifestations` | Listagem e leitura de manifestações de defesa | Sim | Sim | Sim | Sim |
| `PermViewManifestationContact` | Visualização de dados de contato do autor da manifestação | Sim | Sim | Sim | Não |
| `PermModerateClaims` | Aprovar, rejeitar e restaurar alegações | Sim | Sim | Não | Não |
| `PermModerateEvidenceSources` | Desaprovar e restaurar usos de evidência | Sim | Sim | Não | Não |
| `PermModerateManifestations` | Aceitar, rejeitar, revisar e arquivar manifestações | Sim | Sim | Não | Não |
| `PermManageUsers` | Criar usuários, alterar papéis, status, senhas e reset de MFA | Sim | Não | Não | Não |
| `PermViewAuditLogs` | Consulta à trilha de auditoria administrativa interna | Sim | Não | Não | Sim |

Toda requisição mutável verifica autorização no servidor (`RequirePermission` / checagem em handler) e devolve `HTTP 403 Forbidden` imediato em caso de ausência de permissão, sem executar qualquer mutação.

### 3. Autenticação Reforçada por MFA TOTP (RFC 6238) e Criptografia em Repouso

- **Algoritmo Padrão:** Implementação em Go puro de TOTP conforme RFC 6238 e RFC 4226 (HMAC-SHA1, passo de 30 segundos, códigos de 6 dígitos numéricos).
- **Geração e Criptografia de Segredo:** Segredos base32 gerados com 160 bits (20 bytes) de entropia criptográfica segura via `crypto/rand`. Segredos em repouso são sempre criptografados com AES-256-GCM (`internal/auth/crypto.go`) utilizando uma chave simétrica operacional independente de 256 bits (`ADMIN_MFA_ENCRYPTION_KEY`).
- **Estado de Ativação do Lado do Servidor:** O provisionamento de MFA salva o segredo encriptado temporariamente em `mfa_pending_secret_encrypted` com expiração de 15 minutos (`mfa_pending_expires_at`). O segredo nunca é enviado como campo oculto (`input type="hidden"`) no formulário de confirmação. O cliente envia unicamente o código TOTP digitado de 6 dígitos, que é validado contra o segredo pendente decriptado no servidor antes da confirmação definitiva.
- **Compatibilidade Ampla:** Formatação de URI canônica `otpauth://totp/VorcaroZAP:<username>?secret=...&issuer=VorcaroZAP&digits=6&period=30`, suportando qualquer aplicativo autenticador padrão de mercado (Google Authenticator, Microsoft Authenticator, 1Password, Bitwarden, Aegis, etc.).
- **Tolerância a Clock Drift e Anti-Replay:** Validação com janela de $\pm 1$ passo (tolerância total de 30s para trás ou para frente) e controle do último timestep utilizado para impedir reutilização do mesmo código no mesmo intervalo.
- **Segurança e Não-Vazamento:** Segredos de MFA (em claro ou cifrados) nunca aparecem em logs, mensagens de erro, audit logs, queries administrativas ou respostas JSON.

### 4. Gestão de Sessões, Cookies e Rotação

- **Cookies Seguros:** Cookie de sessão `vorcarozap_admin_session` emitido com `HttpOnly: true`, `SameSite: Lax` (ou `Strict`), `Path: /admin` e `Secure: true` (habilitado automaticamente em conexões TLS/produção).
- **Rotação Obrigatória:** A chave de sessão é destruída e rotacionada no SQLite após o login inicial por senha e novamente após a confirmação bem-sucedida do desafio MFA, prevenindo ataques de Session Fixation.
- **Revogação Instantânea:** O logout (`POST /admin/logout`), a alteração de senha, a desativação da conta (`status = 'disabled'`) ou o bloqueio (`status = 'locked'`) invalidam atomicamente as sessões correspondentes no banco.
- **Controle de Inatividade e Expiração:** Sessões expiram após prazo configurável (`AdminSessionTTL`, padrão 8 horas). A cada requisição válida, o campo `last_activity_at` é atualizado no SQLite.

### 5. Mitigação contra Força Bruta, Contadores Independentes e Lockout no Desafio e Setup MFA

- **Equalização de Tempo:** Tentativas de login com usuários inexistentes executam uma computação dummy de bcrypt com custo idêntico, equalizando o tempo de resposta e prevenindo ataques de temporização.
- **Respostas Indistinguíveis:** Mensagens de erro de login utilizam o texto genérico `"Credenciais inválidas"` para impedir a enumeração de contas existentes.
- **Contadores Separados de Falhas:** Separação estrita entre `failed_login_attempts` (erros de senha) e `mfa_failed_attempts` (erros de TOTP). A validação bem-sucedida de senha em `AuthenticatePassword` zera unicamente `failed_login_attempts`, preservando intacto o contador `mfa_failed_attempts`. Isso impede que um atacante de posse da senha legítima zere repetidamente as falhas de segundo fator mediante logins sucessivos.
- **Operações Atômicas de Incremento e Bloqueio (Lockout):** Uso de queries SQL atômicas com `RETURNING` (`RecordPasswordFailedAttemptAndLock` e `RecordMFAFailedAttemptAndLock`) para avaliar o limiar de tentativas (`maxAttempts`) diretamente no motor SQLite, eliminando condições de corrida sob requisições concorrentes.
- **Lockout Temporário por Conta:** Ao atingir o limite configurado de falhas em qualquer um dos fatores, a conta é bloqueada temporariamente (`status = 'locked'`, `locked_until = now() + 15m`).
- **Proteção Própria no Desafio MFA (`VerifyMFALogin`):** Cada código TOTP incorreto submetido incrementa exclusivamente `mfa_failed_attempts`. Ao atingir o limite, a conta é imediatamente bloqueada, a sessão pendente é revogada no banco e o evento é registrado na auditoria administrativa (`mfa_lockout_triggered`).
- **Proteção Própria no Setup Inicial MFA (`ConfirmMFASetup`):** A submissão consecutiva de códigos inválidos na confirmação inicial do setup também incrementa `mfa_failed_attempts` e dispara o lockout da conta, revogando a sessão e purgando o segredo pendente efêmero (`mfa_setup_lockout_triggered`).
- **Reset Restrito de Falhas MFA:** O contador `mfa_failed_attempts` é resetado para zero exclusivamente após a validação bem-sucedida do segundo fator (`RecordUserMFASuccess`), desbloqueio administrativo ou redefinição de credenciais.
- **Auditoria Segura sem Vazamento:** Todas as falhas de segundo fator (`mfa_verification_failed`, `mfa_setup_failed`) e bloqueios são auditadas em `admin_audit_logs` sem registrar ou expor os códigos TOTP submetidos.
- **Tratamento Seguro de Limpezas Secundárias:** Falhas na revogação de sessões secundárias ou na limpeza de segredos pendentes de contas inativas/bloqueadas são propagadas ou registradas com `slog.Error` para auditoria e prevenção de resíduos.
- **Rate Limiting por Origem:** Limitador de taxa em memória por IP para mitigar ataques distribuídos de força bruta contra os formulários de login e desafio MFA.

### 6. Isolamento Estrito de Autenticação e Eliminação do Bypass Basic Auth

- **Eliminação de Fallback em Rotas Administrativas:** O painel administrativo web (`/admin/*`) aceita **estritamente** sessões web baseadas em cookies com MFA verificado (`RequireMFAVerified`), eliminando qualquer possibilidade de bypass das barreiras de RBAC ou MFA via cabeçalhos HTTP Basic Auth legados.
- **Bootstrap Seguro:** No primeiro arranque com o banco migrado, se a tabela `admin_users` estiver vazia e as variáveis legadas `ADMIN_USER` e `ADMIN_PASSWORD_HASH` estiverem presentes, o sistema insere automaticamente o primeiro usuário administrador (`role = 'admin'`, `status = 'active'`, `mfa_enabled = 0`).

### 7. Auditoria Individual e Privacidade na Projeção Pública

- Toda deliberação editorial (`moderation_decisions`, `defense_statement_decisions`) e todo evento administrativo (`admin_audit_logs`) grava como `actor` o `username` do operador individual autenticado no contexto HTTP (`web.AuthenticatedUserFromContext`).
- A projeção pública (`domain.PublicEditorialEvent` e endpoints `/pessoas/{slug}`, `/documentos/{id}`) continua redigindo estritamente a identidade dos operadores sob a denominação institucional "Equipe Editorial" ([ADR-023](ADR-023-historico-publico-de-alteracoes-editoriais-e-trilha-de-transparencia.md)), resguardando a privacidade e a segurança dos moderadores.

## Consequências

### Positivas
- Elimina o risco do compartilhamento de credenciais administrativas únicas.
- Remove qualquer contorno de MFA através de Basic Auth.
- Garante proteção criptográfica de segredos TOTP em repouso com chave simétrica operacional.
- Mantém o estado de setup de MFA protegido exclusivamente no servidor com tempo de vida limitado.
- Implementa o princípio do menor privilégio com separação clara de papéis editoriais, revisores, auditores e administradores.
- Rastreabilidade perene e imutável de todas as ações administrativas e de acesso.
- Mantém o monólito Go e SQLite WAL simples, sem Redis, PostgreSQL ou microsserviços.

### Negativas e Riscos Mitigados
- **Perda de chave de criptografia operacional (`ADMIN_MFA_ENCRYPTION_KEY`):** Chave de 32 bytes gerada e mantida em ambiente seguro; se perdida, o administrador pode redefinir o MFA dos usuários via console/bootstrap.
- **Perda de acesso ao MFA por operador:** Mitigado pela capacidade de administradores desativarem o MFA de um usuário específico via painel com registro auditável em `admin_audit_logs`.
- **Concorrência em alterações de papel e status:** Mitigado pelo controle de versão otimista (OCC) com validação de `expected_updated_at` e retorno determinístico de `HTTP 409 Conflict`.
