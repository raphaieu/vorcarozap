#!/usr/bin/env bash
# ==============================================================================
# VorcaroZAP — Procedimento Não-Destrutivo de Teste de Restauração de Backup
# ==============================================================================
# Este script restaura um snapshot de backup em um banco temporário isolado de teste,
# sem sobrescrever ou afetar o banco SQLite ativo em produção (/data/vorcarozap.db).
#
# Uso:
#   ./scripts/restore-test.sh <caminho_do_backup.db>
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

if [ "$#" -lt 1 ]; then
    echo "Uso: $0 <caminho_do_backup.db>"
    echo "Exemplo: $0 /backups/vorcarozap-20260912_180000Z.db"
    exit 1
fi

BACKUP_PATH="$1"

echo "=== INICIANDO TESTE NÃO-DESTRUTIVO DE RESTAURAÇÃO ==="
echo "Arquivo de backup: ${BACKUP_PATH}"

# Gera nome de arquivo de teste temporário
TIMESTAMP="$(date -u +"%Y%m%d_%H%M%SZ")"
TEST_TARGET="/tmp/vorcarozap_restore_test_${TIMESTAMP}.db"

echo "1. Validando integridade prévia do backup com 'verify-backup'..."
docker compose run --rm -T app vorcarozap verify-backup --file "${BACKUP_PATH}"

echo "2. Restaurando snapshot para banco de teste isolado: ${TEST_TARGET}..."
docker compose run --rm -T app vorcarozap restore --backup "${BACKUP_PATH}" --target "${TEST_TARGET}" --confirm

echo "3. Verificando integridade e migrations do banco restaurado..."
docker compose run --rm -T app vorcarozap verify-backup --file "${TEST_TARGET}"

echo "4. Limpando arquivo temporário de teste..."
docker compose run --rm -T app sh -c "rm -f ${TEST_TARGET} ${TEST_TARGET}-wal ${TEST_TARGET}-shm"

echo "=== TESTE DE RESTAURAÇÃO CONCLUÍDO COM SUCESSO ==="
echo "O arquivo de backup é 100% íntegro e passível de restauração operacional."
