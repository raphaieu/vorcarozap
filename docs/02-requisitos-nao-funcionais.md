# Requisitos não funcionais

## Qualidade de serviço

- **RNF-01 Desempenho:** em VPS de referência a definir, p95 server-side menor que 500 ms para páginas cacheáveis e 800 ms para busca, excluindo rede; nenhuma chamada a LLM no caminho público.
- **RNF-02 Capacidade:** baseline inicial de 10 mil entidades/100 mil alegações e 20 requisições/s; medir antes de otimizar.
- **RNF-03 Disponibilidade:** alvo inicial 99,5% mensal, sem promessa de HA; degradação de monitor/exportação não derruba consulta.
- **RNF-04 Acessibilidade:** WCAG 2.2 AA, teclado, foco visível, landmarks, mensagens textuais, contraste e zoom a 200%.
- **RNF-05 Responsividade:** funcional a partir de 320 px, conteúdo essencial sem rolagem horizontal; progressive enhancement sem exigir JavaScript para leitura.
- **RNF-06 Segurança:** requisitos de `06-seguranca-e-privacidade.md`, princípio do menor privilégio e falha fechada no painel.
- **RNF-07 Privacidade:** minimização, finalidade, acesso, retenção e redação; observabilidade sem conteúdo sensível por padrão.
- **RNF-08 Auditabilidade:** alterações e decisões ligadas a ator, tempo, versão, fundamento e fontes; relógio em UTC.
- **RNF-09 Integridade:** foreign keys, transações curtas, checks, hashes e idempotency keys; nenhuma exportação parcial publicada.
- **RNF-10 Operação:** uma imagem imutável, configuração por ambiente, shutdown gracioso, migrations explícitas e processo escritor agendado único.
- **RNF-11 Portabilidade:** execução local/Compose; domínio não importa pacotes de HTTP, SQLite, Excelize ou OpenRouter.
- **RNF-12 Manutenibilidade:** módulos por capacidade, interfaces nos consumidores, dependências justificadas e ADR para mudança estrutural.

## SQLite e continuidade

Conexões executam `foreign_keys=ON`, `busy_timeout=5000`, `journal_mode=WAL` e `synchronous=NORMAL`, verificando o resultado. Arquivo e WAL ficam em volume local, nunca NFS/object storage. Escritas são pequenas; monitor usa lock no banco e lease com expiração.

RPO/RTO finais dependem de validação operacional. Proposta MVP: RPO 24 h e RTO 4 h, backup diário, retenção 7 diários + 4 semanais + 6 mensais, criptografia antes de cópia externa, `integrity_check` e restauração mensal automatizada em diretório temporário.

## Observabilidade e custos

Logs JSON contêm timestamp, nível, componente, request/run ID e código de erro, mas não segredo, cookie, texto integral de mensagem ou prompt sensível. Métricas mínimas: latência/erros HTTP, conexões e lock/busy SQLite, duração/estado do monitor, candidatos, tokens/custo estimado, falhas de fonte, idade do último backup/export e falhas de login. Alertar em monitor consecutivamente falho, backup vencido, disco baixo, orçamento e indisponibilidade.

Timeouts são explícitos por operação; retries somente para falhas transitórias, com backoff, jitter, limite e idempotência. `SIGTERM` para aceitar tráfego, concluir transação corrente dentro do prazo e liberar lease.

## Testes e gates

- Unitários: classificação, transições, linguagem, slugs, deduplicação e confiança.
- Integração: repositórios SQLite reais, concorrência, migrations ida e validação, importador e exportador.
- Contrato: respostas simuladas do ResearchProvider, schemas e proveniência.
- Segurança: autorização, CSRF, sessão, URL/SSRF, upload, XSS e headers.
- Renderização: páginas principais, estados vazios/erro e acessibilidade automatizada.
- E2E mínimos: importar; consultar/abrir fonte; monitorar/revisar/publicar; exportar; restaurar.

Gate de merge: formatação, geração sem diff, `go test ./...`, análise estática, migrations em banco vazio e atualizado, e teste de unidade relevante. E2E roda em mudanças críticas e release.

