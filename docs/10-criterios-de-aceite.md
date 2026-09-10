# Critérios de aceite

## Gate técnico econômico

```bash
go fmt ./...
go vet ./...
go build ./...
```

Sem percentual de cobertura ou E2E obrigatório no MVP.

São obrigatórios testes unitários table-driven para decisão `published`/`quarantined`, bloqueio de republicação por fingerprint rejeitado e exclusão de estados não públicos nas métricas.

## Smoke funcional

1. subir ambiente/banco vazio;
2. migrar e importar planilha sem duplicação, respeitando estado/disposição/elegibilidade do mapping e colocando exceções em quarentena;
3. consultar home, métricas, busca, filtros e detalhe;
4. executar monitor limitado e registrar custo;
5. candidato A/B válido vira público, C só com linguagem/limites explícitos e D/E vai para quarentena;
6. candidato incompleto vai para quarentena;
7. rejeitar claim remove a alegação; rejeitar evidence_source remove somente aquele uso e põe o claim sem suporte em quarentena;
8. restaurar/autorizar reverte a visibilidade;
9. execuções simultâneas do monitor não se sobrepõem;
10. reiniciar containers preservando SQLite;
11. acessar `/admin` sem credencial falha;
12. baixar XLSX apenas com conteúdo público.

## Gates automáticos de publicação

- O verificador HTTP GET seguro (VZ-020) atua como dependência técnica do gate automático: valida a acessibilidade da URL sem transformar o sistema em crawler invasivo, bloqueia destinos privados/redirecionamentos inseguros e classifica status (`reachable`, `unreachable`, `not_checked`).
- Gate estrutural Go aprova schema, URL, metadados, trecho/localizador, datas, grau, vínculo, PII, tamanho, dedupe, fingerprint e orçamento.
- Gate semântico estruturado aprova identidade, suporte, não extrapolação, atribuição, grau, ausência de inferência ilícita e ambiguidades vazias.
- Política Go publica A/B aprovados e C aprovado com linguagem/limites explícitos; D/E ficam em quarentena.
- Homônimo, source `unreachable`, acusação criminal não confirmada, divergência entre estágios ou rejeição semelhante sempre levam a quarentena; `not_checked` respeita a política por origem do [ADR-006](adr/ADR-006-fontes-e-rastreabilidade.md) (quarentena para OpenRouter; preserva initial_state do mapeamento para curated_seed).

## Métricas e disposição

- `E — fraco/corrigido` importa como `published`, `context_only` e não elegível a métricas de rede.
- `E — fraco/ambíguo` importa como `quarantined`, `possible_link` e não elegível a métricas.
- Métricas de contato/vínculo exigem claim público e `metric_eligible = true`; conteúdo público não elegível continua em página e XLSX.

## Moderação e integridade transacional (VZ-021)

- Decisão de moderação possui integridade XOR estrita: tem como alvo ou um `claim_id` ou um `evidence_source_id`, nunca ambos ou nenhum.
- Uma source referenciada por dois claims continua ativa no segundo caso o primeiro uso seja desaprovado.
- Rejeitar o último `evidence_source` ativo com papel `supports` move o claim correspondente para `quarantined` na mesma transação.
- O verificador GET limitado não persiste corpo e nunca depende apenas de HEAD.
- `unreachable` leva a quarentena; timeout/429/5xx vira `not_checked`, sem rejeição, e segue a política do [ADR-006](adr/ADR-006-fontes-e-rastreabilidade.md). As variáveis `SOURCE_NOT_CHECKED_POLICY_*` foram incorporadas ao carregamento de configuração em VZ-020 (com defaults `quarantine` para openrouter e `allow` para curated_seed).

## Rastreabilidade e navegação documental ([ADR-012](adr/ADR-012-navegacao-documental-e-referencias-a-acervos-externos.md))

- Rastreabilidade pontual: referências a laudos e peças utilizam `evidence_sources.locator` (ex: número do laudo, página, figura) e `evidence_sources.excerpt` (trecho literal), mantendo o documento oficial primário sempre distinguível de qualquer transcrição secundária.
- Menção não é conluio nem culpa: a identificação de um nome em anotação ou laudo documental não autoriza inferir contato direto ou ilícito.
- Proibição de fabricação: nenhum modelo ou operador pode preencher lacunas contextuais ou atribuir identidades por mera semelhança nominal.
- Independência da origem documental: a avaliação de corroboração considera a origem factual da informação e não apenas o veículo publicador; múltiplos veículos reproduzindo a mesma peça ou vazamento derivam de fonte primária comum e não constituem confirmação externa independente, assim como múltiplos trechos do mesmo laudo pertencem à mesma origem.
- Sem violação de licenças: nenhuma importação ou reuso de bases/código de terceiros sem licença expressa identificada.

## Não bloqueiam o MVP

MFA, RBAC multiusuário, dupla revisão, snapshots, trilha imutável, viewer SSR de conversas, métricas históricas, cobertura abrangente, E2E, observabilidade completa, HA, PostgreSQL e política jurídica operacional extensa.
