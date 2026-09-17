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
    echo "Uso: $0 <caminho_do_backup.db | --remote-key chave_s3>"
    echo "Exemplos:"
    echo "  $0 /backups/vorcarozap-20260912_180000Z.db"
    echo "  $0 --remote-key backups/vorcarozap-20260912_180000Z.db"
    exit 1
fi

echo "=== INICIANDO TESTE NÃO-DESTRUTIVO DE RESTAURAÇÃO (SANDBOX) ==="

ARGS=()
if [ "$1" = "--remote-key" ]; then
    if [ "$#" -lt 2 ]; then
        echo "ERRO: --remote-key requer o nome da chave remota."
        exit 1
    fi
    ARGS=("--remote-key" "$2")
    echo "Origem: Object Storage ($2)"
else
    ARGS=("--file" "$1")
    echo "Origem: Arquivo Local ($1)"
fi

echo "Executando restore-sandbox isolado no container..."
docker compose run --rm -T app vorcarozap restore-sandbox "${ARGS[@]}"

echo "=== TESTE DE RESTAURAÇÃO EM SANDBOX CONCLUÍDO COM SUCESSO ==="
echo "O arquivo de backup é 100% íntegro e passível de restauração operacional."
