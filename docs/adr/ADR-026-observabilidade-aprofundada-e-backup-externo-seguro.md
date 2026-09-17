# ADR-026: Observabilidade Operacional Aprofundada e Backup Externo Seguro

## Status

Aceito

## Contexto

O VorcaroZAP opera como um monólito Go e SQLite WAL em VPS econômica única ([ADR-001](ADR-001-monolito-modular-em-go.md), [ADR-002](ADR-002-sqlite-com-wal.md), [ADR-020](ADR-020-deploy-economico-em-vps-cron-persistencia-backup-e-smoke-checklist.md)). Com a maturação das esteiras de monitoramento OpenRouter ([ADR-013](ADR-013-monitoring-runs-and-candidates-deduplication.md), [ADR-014](ADR-014-gate-estrutural-gate-semantico-e-politica-de-publicacao-automatica.md), [ADR-015](ADR-015-lock-de-execucao-janela-incremental-e-limites-de-custo-llm.md)), moderação granular de evidências e claims ([ADR-017](ADR-017-moderacao-humana-de-claims-transacoes-e-protecao-csrf.md), [ADR-018](ADR-018-moderacao-granular-de-evidence-sources-e-quarentena-por-perda-de-suporte.md)), módulo de contraditório ([ADR-024](ADR-024-contraditorio-estruturado-e-submissao-publica-de-manifestacoes.md)) e gestão multiusuário com RBAC e MFA ([ADR-025](ADR-025-gestao-multiusuario-rbac-mfa-e-sessoes.md)), a sustentação operacional em produção exige:

1. **Observabilidade Operacional Estruturada e Segura**:
   - Necessidade de coletar e inspecionar em tempo de execução o estado de saúde do monólito (runtime Go, goroutines, memória, GC), o desempenho e latências HTTP (média, p95, máxima, distribuição de códigos de status), as estatísticas internas do SQLite WAL (tamanho do `.db`, tamanho do `-wal`, contagem de páginas, freelist, versão ativa do schema), a contabilidade detalhada das execuções de monitoramento e custos reais de LLM em USD, o tamanho das filas de moderação e a telemetria das rotinas de backup.
   - **Requisito mandatório de sanitização:** Nenhum log ou métrica pode vazar segredos operacionais (API keys OpenRouter, chaves S3, chaves AES/MFA, hashes de senha, senhas, segredos TOTP, tokens de sessão, cabeçalhos de autorização) nem dados pessoais de contato de manifestações de contraditório (e-mails, telefones).

2. **Backup Externo Seguro em Camada Adicional de Resiliência**:
   - Embora o mecanismo local baseado no comando nativo `VACUUM INTO` e validação `PRAGMA integrity_check` / `store.CheckReady` funcione com consistência atômica comprovada no volume local `/backups`, a guarda de cópias de segurança em provedor de Object Storage externo geograficamente segregado (compatível com o padrão S3: AWS S3, Cloudflare R2, MinIO, Wasabi, Oracle Object Storage) protege a base contra eventos catastróficos de perda da VPS física.
   - **Princípio de isolamento de falha:** Falhas de rede, autenticação externa ou instabilidade no bucket remoto jamais podem comprometer, interromper ou deletar o backup local verificado e comitado.
   - **Padrão fail-closed e zero dependências pesadas:** A camada de backup remoto deve ser opcional e desabilitada por padrão (`BACKUP_REMOTE_ENABLED=false`). Quando habilitada, exige configuração completa e válida via HTTPS TLS 1.2+ e autenticação por assinatura canônica AWS Signature Version 4 (SigV4) implementada em Go puro, sem adicionar bibliotecas SDK de terceiros ou frameworks pesados.

---

## Decisão

Adota-se uma arquitetura integrada de observabilidade operacional sanitizada (`internal/observability`) e gerenciamento de backup externo seguro (`internal/backup`) no monólito Go e SQLite WAL.

### 1. Logger Estruturado com Redação Recursiva de Segredos

- Implementa-se um wrapper customizado de `slog.Handler` (`SanitizedHandler`) que intercepta recursivamente todos os atributos e grupos registrados nas emissões de log.
- **Lista de chaves sanitizadas:** Qualquer atributo com chave correspondente a termos sensíveis (`api_key`, `apikey`, `secret`, `password`, `hash`, `token`, `mfa_key`, `encryption_key`, `totp`, `auth`, `authorization`, `cookie`, `session_id`, `access_key`, `secret_key`, `contact_email`, `contact_phone`, etc.) tem seu valor substituído incondicionalmente pela constante `"[REDACTED]"`.
- **Sanitização de valores e payloads:** Padrões conhecidos de tokens no formato `Bearer <token>`, chaves OpenRouter (`sk-or-v1-...`), endereços de e-mail e números de telefone contidos em strings de log ou mensagens de erro são automaticamente redigidos antes da escrita no destino (`[REDACTED_BEARER]`, `[REDACTED_API_KEY]`, `[REDACTED_EMAIL]`, `[REDACTED_PHONE]`).
- **Formatos suportados:** Saída configurável via `LOG_FORMAT` (`text` em desenvolvimento e `json` estruturado em produção) e nível de log via `LOG_LEVEL` (`DEBUG`, `INFO`, `WARN`, `ERROR`).

### 2. Coletor Thread-Safe de Telemetria Operacional (`MetricsRegistry`)

- Cria-se um coletor central em memória (`internal/observability/metrics.go`) protegido por `sync.RWMutex` e contadores atômicos, capturando telemetria contínua sem impacto na latência das requisições públicas.
- **Dimensões monitoradas:**
  1. *HTTP Performance:* Contadores por método, classes de status (2xx, 3xx, 4xx, 5xx), erros críticos (401, 403, 404, 429, 500) e cálculo de latência média, p95 e máxima com base em janela deslizante de amostras;
  2. *Runtime Go:* Uptime do processo, versão da compilação Go, contagem de goroutines ativas, alocação de memória (Alloc, TotalAlloc, Sys) e ciclos de Garbage Collection;
  3. *SQLite WAL Health:* Tamanho do arquivo `.db`, tamanho do arquivo de journal `-wal`, total de páginas alocadas, páginas na freelist, tamanho de página e versão das migrations ativas;
  4. *Monitoring & LLM Accounting:* Total de execuções completadas, parciais e falhas, candidatos descobertos, verificações semânticas executadas, publicações realizadas, quarentenas e rejeições, além do consumo total de tokens e custos reais acumulados e diários em USD;
  5. *Filas Editoriais e Segurança:* Contagem de alegações ativas, quarentenadas e rejeitadas, manifestações de contraditório pendentes e decididas, e contas de usuário ativas, desabilitadas e bloqueadas por segurança;
  6. *Telemetria de Backups:* Timestamp, duração em milissegundos, tamanho em bytes, status (`ok`, `error`, `disabled`), checksum SHA-256 e mensagens de erro do último backup local e do último backup remoto.
- **Controle de Acesso:** O acesso à tela SSR de inspeção (`GET /admin/observabilidade`) e ao endpoint de telemetria JSON (`GET /admin/api/metrics`) é estritamente restrito aos usuários com permissão `PermViewObservability` (papéis `RoleAdmin` e `RoleAuditor`).

### 3. Cliente S3 em Go Puro com Assinatura AWS Signature Version 4

- Implementa-se em `internal/backup/remote.go` a interface `RemoteStorageProvider` e a classe `S3Provider` utilizando unicamente a biblioteca padrão de Go (`crypto/hmac`, `crypto/sha256`, `net/http`, `encoding/xml`).
- **Algoritmo de Assinatura:** Implementação estrita do padrão AWS Signature Version 4 (`AWS4-HMAC-SHA256`), com canonização de headers, URI e query string, geração de chaves derivadas (`kDate`, `kRegion`, `kService`, `kSigning`) e cabeçalho `Authorization: AWS4-HMAC-SHA256 Credential=...`.
- **Operações suportadas:**
  - `PutObject`: Upload seguro com cabeçalhos `x-amz-content-sha256`, `x-amz-meta-sha256`, `x-amz-meta-created-at` e `x-amz-meta-schema-version`;
  - `HeadObject`: Verificação remota de existência, tamanho e metadados sem transferência do corpo do arquivo;
  - `ListObjects`: Listagem com prefixo e parsing do XML canônico `ListBucketResult` para controle de rotação e retenção;
  - `DeleteObject`: Exclusão segura de backups obsoletos que excedam a política de retenção;
  - `GetObject`: Download seguro em stream para realização de testes não-destrutivos de restauração em sandbox temporário.
- **Universalidade:** Suporte comprovado a AWS S3, Cloudflare R2, MinIO, Wasabi e Oracle Object Storage mediante configuração do endpoint HTTPS.

### 4. Orquestrador de Backup (`BackupManager`) e Isolamento Estrito de Falhas

- O fluxo de backup é gerenciado pelo `BackupManager` na seguinte ordem determinística:
  1. Geração de snapshot local via `store.Backup(ctx, db, destPath)` com comando nativo `VACUUM INTO`, verificação `PRAGMA integrity_check` e `store.CheckReady`;
  2. Aplicação da política de rotação local (`store.RotateBackups`) no volume `/backups`;
  3. Se `BACKUP_REMOTE_ENABLED=true`:
     - Cálculo do checksum SHA-256 local do arquivo gerado;
     - Upload HTTPS com assinatura SigV4 e metadados;
     - Verificação remota com `HeadObject` para garantir integridade e persistência no storage;
     - Aplicação da retenção remota (`ListObjects` + `DeleteObject` mantendo os `BACKUP_REMOTE_RETENTION_COUNT` mais recentes);
  4. **Isolamento de Falha:** Se qualquer erro ocorrer no envio remoto (erro 5xx do provedor, falha de autenticação, timeout de rede), o erro é registrado no log em nível WARN/ERROR e atualizado na telemetria, mas **o backup local concluído com sucesso é preservado intacto**. A rotina principal de backup retorna sucesso local com diagnóstico remoto.
  5. Atualização da telemetria de backups no `MetricsRegistry`.

### 5. Procedimentos Operacionais e Restauração Não-Destrutiva em Sandbox

- Subcomando CLI `vorcarozap backup` com suporte à flag `--remote`.
- Subcomando CLI `vorcarozap verify-remote` para auditoria e conferência de integridade de snapshots no bucket S3.
- Subcomando CLI `vorcarozap restore-sandbox` para validação forense e teste completo de restauração em banco temporário isolado sem qualquer alteração no banco ativo `/data/vorcarozap.db`.

---

## Consequências

### Positivas
- **Visibilidade Operacional Total:** Diagnóstico em tempo real de latência, saúde do SQLite WAL, custos de LLM e integridade de backups no próprio painel administrativo sem necessidade de infraestrutura pesada externa (Prometheus, Grafana).
- **Segurança e Conformidade:** Redação incondicional de credenciais e dados pessoais em todos os canais de telemetria e logs.
- **Resiliência Catastrófica:** Backups consistentes externos com integridade validada criptograficamente por SHA-256 e rotação automatizada em provedores S3/R2.
- **Isolamento Arquitetural:** Zero impacto sobre a operação local caso o provedor remoto fique indisponível.
- **Zero Dependências Pesadas:** Monólito Go autônomo, compilado estaticamente, mantendo o deploy simples e econômico em VPS.

### Neutras / Considerações Operacionais
- A retenção remota depende da permissão de deleção de objetos no bucket S3 (ou ciclo de vida S3 Lifecycle configurado no bucket).
- O tráfego de saída (egress) durante o upload remoto utiliza banda de rede da VPS, devidamente controlado pelo timeout configurável (`BACKUP_REMOTE_TIMEOUT`, padrão 60s).
