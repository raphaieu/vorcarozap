# Guia Operacional: Deploy em VPS, Cron, Backups e Recuperação

Este documento estabelece os procedimentos operacionais padrão (SOP) para implantação econômica, monitoramento agendado, rotina de backups consistentes, testes de restauração, atualizações e recuperação de desastres do **VorcaroZAP** em uma VPS (ex.: Oracle Cloud Free Tier Ampere A1 ou x86).

---

## 1. Topologia de Produção

A arquitetura de produção do MVP consiste em dois containers isolados em uma rede bridge privada do Docker:

```mermaid
flowchart TB
  subgraph Host["VPS (ex.: Oracle Cloud)"]
    subgraph DockerNet["Rede Docker (vorcarozap_default)"]
      CADDY["Caddy (Portas 80, 443)\nHTTPS Automático / Headers"]
      APP["App Go Monólito\n:8080 (Privado)"]
      CADDY -->|reverse_proxy http://app:8080| APP
    end

    subgraph Volumes["Volumes Persistentes Docker"]
      DATA_VOL[("vorcarozap_data (/data)\nvorcarozap.db + WAL")]
      BACKUP_VOL[("vorcarozap_backups (/backups)\nSnapshots VACUUM INTO")]
      CADDY_VOL[("caddy_data / caddy_config\nCertificados TLS")]
    end

    APP --> DATA_VOL
    APP --> BACKUP_VOL
    CADDY --> CADDY_VOL

    subgraph CronHost["Host Cron"]
      CRON_MON["cron-monitor.sh\n(6h/6h)"]
      CRON_BAK["backup.sh\n(Diário)"]
      CRON_MON -->|docker compose exec| APP
      CRON_BAK -->|docker compose exec| APP
    end
  end
```

- **Isolamento de Portas:** A porta `8080` do Go fica restrita ao loopback local (`127.0.0.1:8090:8080`) e não é exposta na interface de rede pública da VPS. O tráfego externo acessa exclusivamente o Caddy nas portas 80 e 443.
- **SQLite em Volume Persistente:** O banco reside no volume `vorcarozap_data` montado em `/data`, operando em modo WAL (`journal_mode=WAL`), com busy timeout de 5s e foreign keys ativadas.
- **Backups no Volume Isolado:** Snapshots consistentes e a política de retenção operam diretamente no volume Docker persistente `vorcarozap_backups` montado em `/backups` no container.

---

## 2. Pré-requisitos de Infraestrutura

1. **VPS:** Instância Linux (Ubuntu 22.04 / 24.04 LTS ou Oracle Linux 8/9). Exemplo econômico: *Oracle Cloud Free Tier VM.Standard.A1.Flex* (ARM64) ou *VM.Standard.E2.1.Micro* (x86_64).
2. **Docker & Docker Compose:** Docker Engine 24+ e Docker Compose v2 instalados.
3. **Firewall / Security List:** Portas `80/TCP` e `443/TCP` liberadas para a internet (Ingress Rules na Oracle Cloud e no `ufw`/`iptables` local).
4. **DNS:** Registro tipo `A` (ou `AAAA`) do domínio (ex.: `vorcarozap.exemplo.org`) apontando para o IP público da VPS com TTL baixo (ex.: 300s) durante a implantação.

---

## 3. Provisionamento Inicial

### 3.1 Clonar o Repositório e Configurar Ambiente

```bash
# 1. Clonar repositório na VPS
git clone <URL_DO_REPOSITORIO> /opt/vorcarozap
cd /opt/vorcarozap

# 2. Criar arquivo de configuração .env a partir do modelo
cp .env.example .env
chmod 600 .env
```

### 3.2 Variáveis de Ambiente Críticas para Produção

Edite `/opt/vorcarozap/.env` com as configurações reais de produção:

```ini
# Configuração básica
APP_ENV=production
APP_PORT=8080
HOST_PORT=8090
APP_BIND_ADDR=127.0.0.1

# Domínio e HTTPS Automático (Caddy gerencia os certificados SSL)
SITE_DOMAIN=vorcarozap.exemplo.org
CADDY_HTTP_PORT=80
CADDY_HTTPS_PORT=443

# OpenRouter para monitoramento factual automatizado
OPENROUTER_API_KEY=sk-or-v1-sua-chave-aqui-sem-vazar
OPENROUTER_DISCOVERY_MODEL=openai/gpt-4.1-mini
OPENROUTER_VERIFICATION_MODEL=openai/gpt-4.1-mini

# Painel Administrativo (/admin) e Proteção CSRF
ADMIN_USER=admin_operador
ADMIN_PASSWORD_HASH=$2a$12$SeuHashBcryptGeradoComCusto12a14Aqui...
ADMIN_ALLOWED_ORIGIN=https://vorcarozap.exemplo.org

# Política de Retenção de Backups (quantidade de backups diários mantidos)
BACKUP_RETENTION_COUNT=7
```

> [!CAUTION]
> - NUNCA compartilhe, imprima ou versione o arquivo `.env`.
> - O hash da senha administrativa DEVE ser gerado com custo bcrypt entre 12 e 14.
> - `ADMIN_ALLOWED_ORIGIN` deve corresponder exatamente à URL pública HTTPS de acesso (ex.: `https://vorcarozap.exemplo.org`).

---

## 4. Inicialização e Subida da Stack

```bash
# 1. Compilar e construir as imagens
make compose-build

# 2. Executar migrations pendentes no banco SQLite
make migrate

# 3. Importar a base curada inicial (idempotente)
make import

# 4. Iniciar os serviços de produção (app + caddy HTTPS)
make prod-up

# 5. Validar prontidão da aplicação
curl -f http://127.0.0.1:8090/health/ready
```

---

## 5. Rotina Operacional de Backups SQLite (Modo WAL) e Retenção

### 5.1 Onde os Backups Realmente Ficam

Os backups são armazenados exclusivamente no volume Docker persistente **`vorcarozap_backups`**, montado internamente em **`/backups`** no container `app`.
O padrão de nomenclatura dos arquivos é:
`/backups/vorcarozap-YYYYMMDD_HHMMSSZ.db`

Para listar os backups existentes no volume real:
```bash
docker compose exec -T app ls -lh /backups
```

### 5.2 Semântica de Snapshot SQLite vs. Atomicidade no Filesystem

Em modo WAL, cópias diretas com `cp` são inseguras porque ignoram transações comitadas nos arquivos `-wal` e `-shm`. O VorcaroZAP utiliza uma estratégia em duas camadas:
1. **Consistência SQLite (`VACUUM INTO`):** O comando nativo do SQLite consolida transações do WAL e gera um snapshot transacional consistente em um arquivo temporário exclusivo (`.tmp.<timestamp>`).
2. **Atomicidade no Filesystem (`os.Rename`):** O snapshot temporário é verificado com `PRAGMA integrity_check` e `CheckReady`. Apenas após a validação completa com sucesso, o arquivo é renomeado atomicamente para o destino definitivo, garantindo que o leitor nunca acesse backups incompletos ou corrompidos e impedindo a sobrescrita acidental de backups existentes.

### 5.3 Como a Retenção Funciona

A retenção é executada diretamente no volume `/backups` pelo binário Go (`store.RotateBackups`):
- Ordena os arquivos correspondentes a `vorcarozap-*.db` por data de modificação decrescente (mais recentes primeiro).
- Preserva os **N** backups mais recentes (`BACKUP_RETENTION_COUNT`, padrão 7) e remove com segurança os excedentes antigos.
- **Invariantes de segurança:**
  - Nunca remove arquivos fora do diretório configurado.
  - Ignora arquivos fora do padrão `vorcarozap-*.db` (ex.: backups manuais, scripts, notas).
  - Ignora symlinks (não segue nem remove links simbólicos).
  - Rejeita valores negativos em `BACKUP_RETENTION_COUNT`.
  - Jamais remove backups anteriores se a geração ou verificação do novo backup falhar.

### 5.4 Execução Manual de Backup

```bash
# Executa backup e aplica retenção no volume Docker
./scripts/backup.sh

# Com opções explícitas (preservando argumentos e caminhos com espaços)
./scripts/backup.sh --retention 14
./scripts/backup.sh --out "/backups/vorcarozap-especial.db" --retention 5
```

### 5.5 Verificação Forense de um Arquivo de Backup

Para inspecionar a integridade de qualquer arquivo de backup no volume:

```bash
docker compose exec -T app vorcarozap verify-backup --file /backups/vorcarozap-20260912_180000Z.db
```

### 5.6 Agendamento de Backup Diário no Host Cron

Adicione a seguinte linha ao crontab do host (`crontab -e`):

```cron
# Backup diário às 03:00 UTC com verificação de integridade e rotação no volume Docker
0 3 * * * /opt/vorcarozap/scripts/backup.sh >> /opt/vorcarozap/logs/backup.log 2>&1
```

---

## 6. Procedimento de Teste de Restauração e Restauração em Produção

### 6.1 Teste Não-Destrutivo de Restauração em Sandbox

Antes de qualquer intervenção em produção, teste a restauração do snapshot em um sandbox temporário **sem sobrescrever ou afetar o banco SQLite ativo em produção** (`/data/vorcarozap.db`):

```bash
./scripts/restore-test.sh /backups/vorcarozap-20260912_180000Z.db
```

O script:
1. Valida a integridade prévia do snapshot original com `PRAGMA integrity_check` e `CheckReady`;
2. Restaura o arquivo em um banco temporário isolado (`/tmp/vorcarozap_restore_test_<timestamp>.db`);
3. Revalida migrations e integridade no banco temporário restaurado;
4. Remove os arquivos temporários de teste e emite relatório de conformidade sem tocar em `/data/vorcarozap.db`.

### 6.2 Regras Críticas para Restauração em Produção

> [!IMPORTANT]
> - **Parada Obrigatória:** Em produção, NUNCA execute `restore` diretamente sobre o banco ativo com o container `app` em execução. Sempre pare o serviço primeiro com `docker compose stop app` para garantir que nenhuma transação WAL esteja em andamento.
> - **Backup Pré-Restore:** Sempre execute um snapshot de segurança antes de restaurar qualquer versão.
> - **Garantia de Não-Truncamento:** O mecanismo `store.Restore` copia os dados para um arquivo temporário exclusivo (`os.CreateTemp`), valida o arquivo temporário completamente e só substitui o destino por renomeação atômica (`os.Rename`). Se a restauração falhar em qualquer etapa, o destino original permanece 100% intacto.

---

## 7. Agendamento do Monitoramento (Cron) e Gestão de Concorrência

O monitoramento autônomo via OpenRouter é executado periodicamente via CLI. O lock distribuído com lease em SQLite (`monitoring_locks`) impede sobreposição: se uma execução estiver em andamento, novas execuções abortam com segurança sem gastar orçamento de LLM.

### 7.1 Execução Manual

```bash
docker compose exec -T app vorcarozap monitor --query "Daniel Vorcaro Banco Master"
```

### 7.2 Agendamento Periódico no Cron

Adicione ao crontab do host (`crontab -e`):

```cron
# Executa monitoramento factual a cada 6 horas
0 */6 * * * /opt/vorcarozap/scripts/cron-monitor.sh >> /opt/vorcarozap/logs/cron.log 2>&1
```

### 7.3 Pausar o Monitoramento

Para pausar temporariamente o monitoramento, comente a linha correspondente no `crontab -e`.

---

## 8. Procedimento de Atualização de Versão

Siga rigorosamente a ordem de passos abaixo para atualizar o VorcaroZAP em produção:

```bash
cd /opt/vorcarozap

# PASSO 1: Fazer backup de segurança pré-deploy
./scripts/backup.sh

# PASSO 2: Obter nova versão do código e compilar imagem
git pull origin main
make compose-build

# PASSO 3: Executar migrations pendentes de forma explícita
make migrate

# PASSO 4: Reiniciar os containers de forma controlada
make prod-up

# PASSO 5: Verificar prontidão e saúde da aplicação
curl -f http://127.0.0.1:8090/health/ready
curl -f http://127.0.0.1:8090/health/live

# PASSO 6: Executar a suíte de Smoke Check
./scripts/smoke-check.sh https://vorcarozap.exemplo.org
```

---

## 9. Procedimento de Rollback de Emergência

Caso uma atualização apresente falhas críticas ou incompatibilidades:

```bash
cd /opt/vorcarozap

# 1. Parar a aplicação
docker compose stop app

# 2. Restaurar o backup pré-deploy para o banco ativo
# Localize o arquivo gerado no PASSO 1 (ex.: /backups/vorcarozap-PRE-DEPLOY.db)
docker compose run --rm -T app vorcarozap restore \
    --backup /backups/vorcarozap-PRE-DEPLOY.db \
    --target /data/vorcarozap.db \
    --confirm

# 3. Retornar o código para a versão anterior estável
git checkout <TAG_OU_COMMIT_ANTERIOR>
make compose-build

# 4. Reiniciar a aplicação
make prod-up

# 5. Validar prontidão
curl -f http://127.0.0.1:8090/health/ready
./scripts/smoke-check.sh https://vorcarozap.exemplo.org
```

---

## 10. Observabilidade, Disco e Controle de Custos

### 10.1 Inspeção de Logs sem Vazamento de Segredos

```bash
# Logs da aplicação Go
docker compose logs -f --tail=100 app

# Logs do proxy Caddy
docker compose logs -f --tail=100 caddy

# Logs das execuções de monitoramento
tail -n 100 /opt/vorcarozap/logs/monitor.log
```

> [!NOTE]
> Os logs da aplicação e dos scripts omitem explicitamente tokens de API, hashes e dados sensíveis.

### 10.2 Manutenção de Disco e Checkpoint do WAL

O SQLite em modo WAL executa checkpoints automáticos por padrão a cada 1000 páginas. Para verificar o tamanho dos arquivos:

```bash
docker compose exec -T app ls -lh /data /backups
```

### 10.3 Controle Financeiro de LLM

O VorcaroZAP impõe limites orçamentários duplos por padrão:
- `MONITOR_MAX_COST_PER_RUN_USD=0.25`: encerra graciosamente em `partial` caso o custo de uma única execução exceda $0.25;
- `MONITOR_MAX_COST_PER_DAY_USD=1.00`: aborta antes da descoberta caso o acumulado do dia em UTC atinja $1.00;
- `MONITOR_LOCK_TTL=10m`: lease no SQLite que impede sobreposição e corrida de custos.
