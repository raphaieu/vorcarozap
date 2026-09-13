#!/usr/bin/env bash
# ==============================================================================
# VorcaroZAP — Script Operacional de Backup SQLite Consistente (Modo WAL)
# ==============================================================================
# Este script executa um snapshot consistente via VACUUM INTO no container 'app',
# valida PRAGMA integrity_check e a compatibilidade de migrations do Goose, e
# aplica a política de retenção no volume persistente /backups do container.
#
# Uso:
#   ./scripts/backup.sh [--out /backups/caminho.db] [--retention <N>]
#
# Exemplos:
#   ./scripts/backup.sh
#   ./scripts/backup.sh --retention 14
#   ./scripts/backup.sh --out "/backups/vorcarozap-customizado.db" --retention 5
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

LOG_DIR="${REPO_ROOT}/logs"
mkdir -p "${LOG_DIR}"
LOG_FILE="${LOG_DIR}/backup.log"

log() {
    local timestamp
    timestamp="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo "[${timestamp}] $*" | tee -a "${LOG_FILE}"
}

log "=== INICIANDO ROTINA DE BACKUP SQLITE CONSISTENTE ==="

ARGS=()
HAS_RETENTION=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --retention)
            if [[ $# -lt 2 ]] || ! [[ "$2" =~ ^[0-9]+$ ]]; then
                log "ERRO: A opção --retention requer um número inteiro não negativo."
                exit 1
            fi
            ARGS+=("$1" "$2")
            HAS_RETENTION=1
            shift 2
            ;;
        --out)
            if [[ $# -lt 2 ]] || [[ -z "$2" ]]; then
                log "ERRO: A opção --out requer um caminho de arquivo não vazio."
                exit 1
            fi
            ARGS+=("$1" "$2")
            shift 2
            ;;
        -h|--help)
            ARGS+=("$1")
            shift
            ;;
        *)
            log "ERRO: Opção desconhecida '$1'. Opções aceitas: --out <caminho>, --retention <N>, --help."
            exit 1
            ;;
    esac
done

# Se a flag --retention não foi explicitamente fornecida via CLI, aplica BACKUP_RETENTION_COUNT do ambiente
if [ "${HAS_RETENTION}" -eq 0 ]; then
    ENV_RETENTION="${BACKUP_RETENTION_COUNT:-7}"
    if ! [[ "${ENV_RETENTION}" =~ ^[0-9]+$ ]]; then
        log "ERRO: BACKUP_RETENTION_COUNT inválido ('${ENV_RETENTION}'). Deve ser um número inteiro não negativo."
        exit 1
    fi
    ARGS+=("--retention" "${ENV_RETENTION}")
fi

# Determina se executa no container app em execução ou em container efêmero
if docker compose ps --services --filter "status=running" 2>/dev/null | grep -q "^app$"; then
    EXEC_CMD=(docker compose exec -T app vorcarozap backup "${ARGS[@]}")
else
    EXEC_CMD=(docker compose run --rm -T app vorcarozap backup "${ARGS[@]}")
fi

if "${EXEC_CMD[@]}" 2>&1 | tee -a "${LOG_FILE}"; then
    log "SUCESSO: Rotina de backup e retenção concluída com sucesso no volume de backups."
else
    EXIT_CODE=$?
    log "ERRO: Falha ao gerar, validar ou rotacionar backup do SQLite (código ${EXIT_CODE})"
    exit "${EXIT_CODE}"
fi

log "=== ROTINA DE BACKUP FINALIZADA COM SUCESSO ==="
