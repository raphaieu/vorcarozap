# Backlog inicial priorizado

Critérios são resumidos aqui e complementados por `10-criterios-de-aceite.md`.

| ID | Pri. | História e aceite verificável | Dep. | Observações técnicas |
|---|---|---|---|---|
| VZ-001 | P0 | Como mantenedor, quero CLI/config validados; todos os subcomandos exibem help, segredos não logam e config inválida falha antes de servir. | F0 | stdlib primeiro; exit codes documentados |
| VZ-002 | P0 | Como operador, quero Compose/Caddy/volume local; serviço sobe limpo, HTTPS é configurável e dados sobrevivem restart. | 001 | uma réplica; non-root |
| VZ-003 | P0 | Como desenvolvedor, quero migrations/sqlc; banco vazio e versão anterior migram, FK/PRAGMAs são verificados. | 001 | goose + database/sql |
| VZ-004 | P0 | Como operador, quero live/ready e logs; falhas de DB aparecem sem segredo e shutdown libera recursos. | 002–003 | JSON/request ID |
| VZ-005 | P0 | Como editor, quero entidades/aliases; homônimos coexistem, slug é estável e merge preserva referências. | 003 | concorrência otimista |
| VZ-006 | P0 | Como editor, quero relações/claims separados; não posso publicar claim sem evidência/fonte e grau válido. | 005 | invariantes no domínio |
| VZ-007 | P0 | Como editor, quero fontes/evidence_sources; cada fonte informa exatamente o trecho/localizador e seu papel. | 006 | URL canônica; SSRF no fetch |
| VZ-008 | P0 | Como pessoa citada/editor, quero contraditório; estado da busca e manifestação aparecem junto à alegação sem tratar silêncio como admissão. | 006–007 | escopo entity/claim |
| VZ-009 | P0 | Como auditor, quero histórico append-only; decisão mostra ator, antes/depois, motivo e horário, e rejeitado não é apagado. | 006 | acesso restrito ao bruto |
| VZ-010 | P0 | Como curador, quero dry-run do XLSX; cabeçalhos são mapeados, 151 linhas reportadas, originais preservados, erros por linha e zero publicação. | 003,005–009 | não presumir layout; zip limits |
| VZ-011 | P0 | Como curador, quero aplicar importação; hash+mapeamento tornam a operação idempotente, falha faz rollback e cria candidatos pending_review. | 010 | escala legada sem automap |
| VZ-012 | P0 | Como leitor, quero home acessível; vejo independência, resumo, totais, metodologia e atualização em 320 px e sem JS. | 006 | templ/SSR |
| VZ-013 | P0 | Como leitor, quero busca/filtros/ordenação; combinações retornam resultado correto, URL compartilhável e desempate determinístico. | 012 | medir antes de FTS5 |
| VZ-014 | P0 | Como leitor, quero detalhe; vejo relação, alegação, grau textual, limites, fontes, contraditório e revisão, nunca rascunhos. | 007–009,012 | HTML e HTMX equivalentes |
| VZ-015 | P0 | Como administrador, quero login/sessão; cookies seguros, CSRF, rate limit, logout/revogação e testes de bypass funcionam. | 002–004 | Argon2id/MFA a fechar |
| VZ-016 | P0 | Como administrador, quero RBAC; reviewer/editor/admin só executam permissões da matriz e toda negativa falha fechada. | 015 | autorização no use case |
| VZ-017 | P0 | Como reviewer, quero fila comparativa; vejo origem por campo/bruto restrito e aprovo/rejeito com motivo. | 009,016 | version check |
| VZ-018 | P0 | Como editor, quero publicar; só approved vira published, ação é auditada e invalida resumo/export. | 017 | automação nunca publica |
| VZ-019 | P0 | Como operador, quero ResearchProvider; fixtures exercitam schema estrito, citações, erros e troca de provedor. | 004,006 | interface no consumidor |
| VZ-020 | P0 | Como operador, quero monitor exclusivo; lease impede paralelo, checkpoints retomam e efeitos são idempotentes. | 019 | transações fora da rede |
| VZ-021 | P0 | Como reviewer, quero verificação; primária/segunda fonte/defesa são procuradas, confiança explicada e saída fica pending_review. | 017,020 | budget hard-stop |
| VZ-022 | P0 | Como leitor, quero XLSX; abas exigidas refletem um snapshot, contêm metodologia/versão/hash e não vazam campos internos. | 018 | excelize; rename atômico |
| VZ-023 | P0 | Como operador, quero backup/restore; backup online passa integridade, é criptografado externamente e restauração ensaiada atende RPO/RTO. | 003 | não copiar DB vivo |
| VZ-024 | P0 | Como equipe, quero gates de qualidade; unitários, integração, auth, import/export, render e E2E críticos passam com fixtures fictícias. | contínua | Playwright mínimo |
| VZ-025 | P1 | Como editor, quero merge assistido; prévia mostra impactos e undo lógico/histórico preserva aliases e referências. | 005,009,016 | nunca automerge pessoa |
| VZ-026 | P1 | Como leitor, quero histórico/correções público; mudanças materiais têm data, motivo e versões sem expor auditoria privada. | 009,018 | política editorial |
| VZ-027 | P1 | Como operador, quero painel de custos/falhas; tokens, custo, limites, runs parciais e alertas são consultáveis. | 020–021 | sem prompts sensíveis |
| VZ-028 | P1 | Como editor, quero dupla revisão configurável para alto risco; autor não pode completar ambas e bypass é testado. | 016–018 | regra a validar |
| VZ-029 | P1 | Como leitor, quero sitemap/metadata; apenas conteúdo publicado é indexado e páginas disputadas seguem política. | 014 | robots não é controle acesso |
| VZ-030 | P2 | Como pesquisador, quero API/JSON versionado; dados públicos têm paginação, rate limit e mesma semântica do XLSX. | 022 | avaliar demanda/licença |

