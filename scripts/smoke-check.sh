#!/usr/bin/env bash
# ==============================================================================
# VorcaroZAP — Script de Validação Operacional (Smoke Check HTTP)
# ==============================================================================
# Este script executa verificações de fumaça contra a instância em execução,
# validando endpoints públicos, saúde, exportação, segurança do admin e CSRF.
#
# Uso:
#   ./scripts/smoke-check.sh [BASE_URL]
# Exemplo:
#   ./scripts/smoke-check.sh http://localhost:8090
#   ./scripts/smoke-check.sh http://localhost:8000
# ==============================================================================

set -euo pipefail

BASE_URL="${1:-http://localhost:8090}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_TEST_PASS:-}"

echo "=== INICIANDO SMOKE CHECK OPERACIONAL CONTRA ${BASE_URL} ==="

FAILED=0

check_endpoint() {
    local name="$1"
    local path="$2"
    local expected_code="$3"
    local extra_flags="${4:-}"

    local url="${BASE_URL}${path}"
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" ${extra_flags} "${url}" || echo "000")

    if [ "${code}" = "${expected_code}" ]; then
        echo "  [PASS] ${name}: ${path} retornou HTTP ${code}"
    else
        echo "  [FAIL] ${name}: ${path} esperado HTTP ${expected_code}, obtido HTTP ${code}"
        FAILED=1
    fi
}

echo "1. Validando endpoints de integridade e saúde..."
check_endpoint "Liveness probe" "/health/live" "200"
check_endpoint "Readiness probe" "/health/ready" "200"

echo "2. Validando rotas públicas principais..."
check_endpoint "Página inicial" "/" "200"
check_endpoint "Listagem de pessoas" "/pessoas" "200"
check_endpoint "Página de metodologia" "/metodologia" "200"
check_endpoint "Exportação XLSX" "/exportar/base.xlsx" "200"

echo "3. Validando proteção e segurança da área administrativa..."
# Sem credenciais deve exigir autenticação (401 ou 303 Redirect) ou 404 se desabilitado
ADMIN_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/admin" || echo "000")
if [ "${ADMIN_STATUS}" = "401" ] || [ "${ADMIN_STATUS}" = "303" ]; then
    echo "  [PASS] Área administrativa /admin exige autenticação (HTTP ${ADMIN_STATUS})"
elif [ "${ADMIN_STATUS}" = "404" ]; then
    echo "  [INFO] Área administrativa /admin desabilitada por configuração (HTTP 404)"
else
    echo "  [FAIL] Área administrativa /admin com status inesperado: HTTP ${ADMIN_STATUS}"
    FAILED=1
fi

# Se houver senha de teste configurada, valida fluxo de autenticação por sessão e CSRF
if [ -n "${ADMIN_PASS}" ] && ([ "${ADMIN_STATUS}" = "401" ] || [ "${ADMIN_STATUS}" = "303" ]); then
    echo "4. Validando autenticação por sessão e proteção CSRF com credenciais..."
    COOKIE_JAR=$(mktemp)

    LOGIN_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -c "${COOKIE_JAR}" -X POST "${BASE_URL}/admin/login" -d "username=${ADMIN_USER}&password=${ADMIN_PASS}" || echo "000")
    if [ "${LOGIN_STATUS}" = "303" ] || [ "${LOGIN_STATUS}" = "200" ]; then
        echo "  [PASS] Login administrativo autenticado (HTTP ${LOGIN_STATUS})"
        check_endpoint "Admin autenticado" "/admin" "200" "-b ${COOKIE_JAR}"
        check_endpoint "Admin observabilidade SSR" "/admin/observabilidade" "200" "-b ${COOKIE_JAR}"
        check_endpoint "Admin métricas JSON" "/admin/api/metrics" "200" "-b ${COOKIE_JAR}"

        # POST sem Origin deve ser bloqueado por CSRF (HTTP 403)
        CSRF_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -b "${COOKIE_JAR}" -X POST "${BASE_URL}/admin/claims/claim_test/moderate" -d "action=approve&reason=Teste" || echo "000")
        if [ "${CSRF_STATUS}" = "403" ]; then
            echo "  [PASS] Proteção CSRF no admin: POST sem Origin rejeitado com HTTP 403"
        else
            echo "  [FAIL] Proteção CSRF no admin: POST sem Origin retornou HTTP ${CSRF_STATUS}, esperado 403"
            FAILED=1
        fi
    else
        echo "  [WARN] Login administrativo retornou HTTP ${LOGIN_STATUS} (verifique credenciais ou desafio de MFA)"
    fi
    rm -f "${COOKIE_JAR}"
fi

echo "================================================================================"
if [ "${FAILED}" -eq 0 ]; then
    echo "=== SMOKE CHECK OPERACIONAL: TODOS OS TESTES PASSARAM COM SUCESSO ==="
    exit 0
else
    echo "=== SMOKE CHECK OPERACIONAL: UMA OU MAIS ASSERÇÕES FALHARAM ==="
    exit 1
fi
