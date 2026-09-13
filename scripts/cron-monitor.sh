#!/usr/bin/env bash
# ==============================================================================
# VorcaroZAP — Script de Execução Operacional de Monitoramento (Cron / Agendado)
# ==============================================================================
# Este script executa o ciclo seguro de monitoramento no container 'app'.
# O lock distribuído com lease em SQLite (monitoring_locks) impede sobreposição
# de execuções concorrentes e cancela a execução com segurança em caso de colisão.
#
# Uso:
#   ./scripts/cron-monitor.sh ["Consulta 1"] ["Consulta 2"] ...
#
# Se nenhuma consulta for passada via argumento, utiliza as consultas
# definidas na variável MONITOR_QUERIES ou a lista padrão de monitoramento.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

LOG_DIR="${REPO_ROOT}/logs"
mkdir -p "${LOG_DIR}"
LOG_FILE="${LOG_DIR}/monitor.log"

# Define consultas a executar
QUERIES=()
if [ "$#" -gt 0 ]; then
    QUERIES=("$@")
elif [ -n "${MONITOR_QUERIES:-}" ]; then
    IFS=';' read -ra QUERIES <<< "${MONITOR_QUERIES}"
else
    QUERIES=(
        "Daniel Vorcaro Banco Master"
        "Daniel Vorcaro investigacao"
    )
fi

log() {
    local timestamp
    timestamp="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo "[${timestamp}] $*" | tee -a "${LOG_FILE}"
}

log "=== INICIANDO CICLO OPERACIONAL DE MONITORAMENTO ==="
log "Consultas a executar: ${#QUERIES[@]}"

OVERALL_STATUS=0

for query in "${QUERIES[@]}"; do
    query_trimmed="$(echo "${query}" | xargs)"
    if [ -z "${query_trimmed}" ]; then
        continue
    fi

    log "--- Executando monitoramento para: \"${query_trimmed}\" ---"
    START_TIME="$(date +%s)"

    # Executa dentro do container em execução ou inicia container temporário
    if docker compose ps --services --filter "status=running" 2>/dev/null | grep -q "^app$"; then
        EXEC_CMD=(docker compose exec -T app vorcarozap monitor --query "${query_trimmed}")
    else
        EXEC_CMD=(docker compose run --rm -T app vorcarozap monitor --query "${query_trimmed}")
    fi

    if "${EXEC_CMD[@]}" 2>&1 | tee -a "${LOG_FILE}"; then
        END_TIME="$(date +%s)"
        DURATION=$((END_TIME - START_TIME))
        log "SUCESSO: Monitoramento concluído para \"${query_trimmed}\" (${DURATION}s)"
    else
        EXIT_CODE=$?
        END_TIME="$(date +%s)"
        DURATION=$((END_TIME - START_TIME))
        log "ERRO: Monitoramento falhou ou foi bloqueado para \"${query_trimmed}\" (código ${EXIT_CODE}, ${DURATION}s)"
        OVERALL_STATUS=1
    fi
done

log "=== CICLO DE MONITORAMENTO FINALIZADO (Status geral: ${OVERALL_STATUS}) ==="
exit "${OVERALL_STATUS}"
