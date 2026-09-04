# ADR-010 — Escolha do driver SQLite

- **Status:** aceito
- **Data:** 2026-09-04

## Contexto

O VorcaroZAP utiliza SQLite em modo WAL como banco de dados principal ([ADR-002](file:///home/raphael/personal/vorcarozap/docs/adr/ADR-002-sqlite-com-wal.md)), operando como monólito Go em instância única. A aplicação requer integridade referencial (`foreign_keys=ON`), tolerância a contenção transitória (`busy_timeout=5000`), durabilidade balanceada (`synchronous=NORMAL`) e execução eficiente de migrations (`pressly/goose/v3`).

Além disso, o deploy ocorre em contêineres Docker enxutos e o ambiente de desenvolvimento deve ser reproduzível e portátil sem dependência de toolchain C (gcc/musl).

## Alternativas consideradas

1. **`modernc.org/sqlite` (Pure Go via transcompilação ccgo):**
   - **CGO:** Não utiliza CGO (`CGO_ENABLED=0`).
   - **Compilação e Docker:** Compilação extremamente rápida, binários estáticos puros, imagens Docker mínimas (scratch/alpine) sem pacotes `build-base` ou `gcc`.
   - **WAL e Pragmas:** Total compatibilidade com WAL, `foreign_keys`, `busy_timeout` e `synchronous`. Suporta injeção de pragmas por conexão diretamente na DSN.
   - **Portabilidade e Manutenção:** Portável para qualquer arquitetura suportada pelo Go; mantido ativamente e atualizado com as últimas versões do SQLite upstream.
   - **Backup consistente:** Suporta `VACUUM INTO 'backup.db'` e transações para snapshot consistente.

2. **`github.com/mattn/go-sqlite3` (CGO clássico):**
   - **CGO:** Requer CGO (`CGO_ENABLED=1`) e compilador C no host e no estágio de build do Docker.
   - **Compilação e Docker:** Compilação mais lenta; exige pacotes de desenvolvimento C e complica compilação cruzada.
   - **WAL e Pragmas:** Suporte maduro e histórico ao SQLite C API.
   - **Portabilidade:** Limitada pela dependência de libc e toolchain nativo.

3. **`github.com/ncruces/go-sqlite3` (WebAssembly/wazero):**
   - **CGO:** Pure Go em tempo de execução via runtime Wasm embutido.
   - **Compilação e Docker:** Dispensa CGO, mas adiciona camada de abstração e runtime Wasm sem ganho mensurável sobre o código nativo transcompilado para a escala do MVP.

## Decisão

Adotar **`modernc.org/sqlite`** como driver SQLite padrão do projeto.

Justificativa:
- Elimina CGO, simplificando radicalmente o `Dockerfile`, o processo de build multi-stage e a execução local.
- Oferece suporte completo a WAL, transações e pragmas essenciais.
- Permite configurar pragmas por conexão via DSN (ex: `_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)`), assegurando que toda conexão aberta pelo pool receba a configuração mandatória sem depender de execução imperativa pontual.
- Para a Fase 1, adota-se uma política conservadora de pool de conexões (`SetMaxOpenConns(1)` e `SetMaxIdleConns(1)`), eliminando concorrência de escrita no SQLite e garantindo previsibilidade.
- Viabiliza backup consistente online por meio de `VACUUM INTO`.

## Consequências

- O `Dockerfile` não necessita de `gcc`, `musl-dev` ou dependências de build C.
- A aplicação compila como binário Go estático puro.
- O driver é compatível diretamente com `pressly/goose/v3` e `sqlc`.

## Gatilhos de revisão

Caso surja necessidade de extensões SQLite nativas em C não portadas para Go ou gargalo crítico comprovado de CPU em operações matemáticas intensivas do SQLite, o driver pode ser reavaliado.
