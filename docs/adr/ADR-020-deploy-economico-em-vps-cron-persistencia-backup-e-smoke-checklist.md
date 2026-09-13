# ADR-020 — Deploy econômico em VPS, cron de monitoramento, persistência SQLite, backup operacional e smoke checklist

- **Status:** aceito
- **Data:** 2026-09-12
- **Fase:** VZ-018 (Fase 6 — Deploy MVP e validação econômica)

## Contexto

O VorcaroZAP foi concebido e arquitetado como um monólito modular em Go com renderização Server-Side (SSR via Templ), banco de dados SQLite em modo WAL (`modernc.org/sqlite`) e proxy reverso Caddy ([ADR-001](ADR-001-monolito-modular-em-go.md), [ADR-002](ADR-002-sqlite-com-wal.md), [ADR-003](ADR-003-renderizacao-server-side.md)).

Com a conclusão da esteira de monitoramento OpenRouter com lock de lease SQLite ([ADR-015](ADR-015-lock-de-execucao-janela-incremental-e-limites-de-custo-llm.md)), moderação humana de claims e evidências com auditoria imutável ([ADR-017](ADR-017-moderacao-humana-de-claims-transacoes-e-protecao-csrf.md), [ADR-018](ADR-018-moderacao-granular-de-evidence-sources-e-quarentena-por-perda-de-suporte.md)) e invalidação dinâmica imediata ([ADR-019](ADR-019-invalidacao-imediata-de-visualizacoes-publicas-metricas-e-exportacao.md)), torna-se necessário formalizar e padronizar o ambiente de implantação em produção econômica (ex.: VPS Oracle Cloud Free Tier / Ampere A1 ou x86), abordando:

1. **Topologia e Segregação de Rede:**
   - Evitar a exposição pública direta da porta interna da aplicação (`8080`), canalizando todo o tráfego externo através do proxy Caddy nas portas 80 (HTTP) e 443 (HTTPS).
   - Gerenciar certificados TLS automáticos de forma flexível via Caddy (`{$SITE_DOMAIN}`), sem embutir segredos, chaves ou domínios estáticos no repositório.

2. **Persistência de Dados e Consistência de Backup em SQLite WAL:**
   - Garantir que o banco de dados resida em volume Docker persistente fora da camada efêmera do container.
   - Em modo SQLite WAL, cópias simples do arquivo principal (`cp vorcarozap.db backup.db`) são perigosas e podem produzir arquivos corrompidos ou com transações parciais ativas nos arquivos `-wal` e `-shm`.
   - É mandatório adotar um mecanismo transacional atômico e online nativo do SQLite (`VACUUM INTO`), acompanhado de validação forense imediata de páginas e índices (`PRAGMA integrity_check`) e compatibilidade estrita de migrations (`CheckReady`).

3. **Automação do Cron e Proteção contra Concorrência:**
   - Executar a descoberta automatizada de fatos periodicamente via cron do host ou orquestrador local chamando o subcomando `vorcarozap monitor --query`.
   - Utilizar o lock distribuído de lease em banco (`monitoring_locks`) para impedir contenção de escrita ou gasto duplo de tokens em execuções concorrentes ou sobrepostas.
   - Registrar auditoria operacional em logs sem vazamento de chaves de API (`OPENROUTER_API_KEY`) ou credenciais administrativas.

4. **Procedimentos Operacionais e Checklist de Fumaça:**
   - Estabelecer fluxos auditáveis e determinísticos para atualização de versão, execução de migrations, testes não-destrutivos de restauração de backup e rollback de emergência.

---

## Decisão

### 1. Topologia de Serviços em Container Único com Caddy Ingress

Adota-se uma composição padronizada no Docker Compose com perfis segregados (`dev`, `prod`, `proxy`):
- **Serviço `app`:** Container enxuto Alpine não-root (`appuser:appgroup`), escutando na porta interna `8080`, com bind no host configurável e restrito ao loopback (`${APP_BIND_ADDR:-127.0.0.1}:${HOST_PORT:-8090}:8080`), impedindo acesso direto via internet pública na VPS.
- **Serviço `caddy`:** Proxy reverso oficial (`caddy:2-alpine`), expondo exclusivamente as portas `80` (HTTP) e `443` (HTTPS), com `depends_on: app` condicionado a `service_healthy`.
- **Caddyfile Parametrizado:** O bloco de escuta utiliza `{$SITE_DOMAIN::80}`. Quando `SITE_DOMAIN` não for definido (ou configurado como `:80`), opera localmente em HTTP simples sem tentar requisições ACME; quando definido com um domínio válido (FQDN), provisiona e renova automaticamente certificados SSL/TLS (Let's Encrypt / ZeroSSL) e injeta headers estritos de segurança (`Strict-Transport-Security`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`).
- **Healthcheck de Prontidão Real:** O container `app` define `HEALTHCHECK` via `/health/ready`, que executa ping de conexão e valida que todas as migrations do Goose esperadas pelo binário estão ativas no SQLite.

### 2. Volumes Persistentes e Backup Consistente com SQLite WAL

- **Volumes Persistentes Declarados:**
  - `vorcarozap_data:/data`: armazena o banco ativo `vorcarozap.db`, `vorcarozap.db-wal` e `vorcarozap.db-shm`.
  - `vorcarozap_backups:/backups`: armazena snapshots consolidados e independentes de backup.
  - `caddy_data` e `caddy_config`: preservam certificados e estado de TLS do Caddy.
- **Mecanismo de Backup Consistente e Atômico:**
  - Implementado no pacote `internal/store` e exposto via subcomando `vorcarozap backup [--out <caminho.db>] [--retention <N>]` e script operacional `scripts/backup.sh`.
  - **Consistência SQLite:** O comando nativo `VACUUM INTO` compacta e consolida todas as páginas do banco e transações comitadas do WAL em um arquivo snapshot temporário (`.tmp.<timestamp>`), sem travar leituras concorrentes.
  - **Atomicidade no Filesystem:** O snapshot temporário é verificado forensicamente (`PRAGMA integrity_check` e `store.CheckReady`) e renomeado atomicamente (`os.Rename`) para o destino final definitivo, garantindo que leitores nunca acessem backups incompletos e impedindo a sobrescrita acidental de backups existentes.
- **Política de Retenção no Volume (`store.RotateBackups`):**
  - Aplicada diretamente no volume `/backups` do container após a validação do novo backup.
  - Preserva os N backups mais recentes correspondendo a `vorcarozap-*.db`, ignora symlinks (`os.Lstat`), rejeita valores negativos e nunca apaga backups prévios em caso de falha de geração do novo snapshot.
- **Validação de Integridade e Migrations (`VerifyBackup`):**
  - Todo backup gerado é imediatamente validado abrindo uma conexão isolada e executando `PRAGMA integrity_check;` (confirmando resposta `"ok"`) e `store.CheckReady` (confirmando compatibilidade de migrations do Goose).
  - O subcomando `vorcarozap verify-backup --file <caminho.db>` permite inspeção forense de qualquer snapshot a qualquer momento.
- **Procedimento Não-Destrutivo de Restauração (`scripts/restore-test.sh`):**
  - Testes periódicos de restauração são executados em arquivos temporários de teste (`/tmp/vorcarozap_restore_test_<timestamp>.db`), verificando integridade e contagem de entidades sem sobrescrever ou desestabilizar o banco de produção.

### 3. Agendamento de Monitoramento e Concorrência

- **Script Operacional (`scripts/cron-monitor.sh`):**
  - Permite agendamento via cron tradicional do host (`crontab -e`) ou systemd timers.
  - Itera sobre as consultas temáticas configuradas (`MONITOR_QUERIES`) e invoca `docker compose exec -T app vorcarozap monitor --query "<consulta>"`.
  - O lock distribuído de lease no SQLite (`monitoring_locks`) impede que jobs sobrepostos executem simultaneamente: se uma run anterior ainda estiver ativa, a nova tentativa é rejeitada com código não-zero e mensagem auditável.
  - Logs são centralizados em `logs/monitor.log` com data/hora UTC e sanitização absoluta de chaves (`OPENROUTER_API_KEY`) e senhas.

### 4. Fluxo Seguro de Atualização e Rollback

1. **Backup Pré-Deploy:** `docker compose exec -T app vorcarozap backup`
2. **Atualização da Imagem:** `docker compose build app` (ou `docker compose pull`)
3. **Execução Explícita de Migrations:** `docker compose run --rm app migrate`
4. **Reinício Controlado:** `docker compose up -d app`
5. **Verificação de Prontidão:** `curl -f http://127.0.0.1:8090/health/ready`
6. **Smoke Checklist:** Execução do script `scripts/smoke-check.sh`
7. **Rollback de Emergência (se necessário):**
   - Parar aplicação: `docker compose stop app`
   - Reverter migration ou restaurar snapshot: `docker compose run --rm app vorcarozap restore --backup /backups/... --target /data/vorcarozap.db --confirm`
   - Reiniciar aplicação e validar `/health/ready`.

---

## Consequências

### Positivas
- **Custo Quase Nulo:** Execução viável e robusta em instâncias gratuitas (Oracle Cloud Free Tier Ampere A1 com 4 OCPUs e 24 GB RAM ou instâncias de $5/mês).
- **Consistência Absoluta em WAL:** Eliminação do risco de backups corrompidos causados por cópias brutas de arquivos enquanto transações estão no buffer WAL.
- **HTTPS Zero-Ops:** Certificados SSL emitidos e renovados automaticamente pelo Caddy sem intervenção manual.
- **Segurança de Borda:** Porta da aplicação fechada para a internet externa; apenas Caddy (80/443) exposto.
- **Auditabilidade Operacional:** Cada backup, monitoramento ou restauração gera logs estruturados e verificáveis sem vazar segredos.

### Limitações e Evoluções Futuras
- O backup do MVP é armazenado localmente no volume persistente da VPS. No pós-MVP (Fase 7), pode-se adicionar replicação secundária para armazenamento off-site (ex.: bucket S3/R2 criptografado).
- A aplicação permanece estritamente *single-instance*, o que é perfeitamente aderente ao modelo SQLite WAL e ao volume planejado.
