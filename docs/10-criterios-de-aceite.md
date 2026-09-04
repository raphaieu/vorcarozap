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

- Gate estrutural Go aprova schema, URL, metadados, trecho/localizador, datas, grau, vínculo, PII, tamanho, dedupe, fingerprint e orçamento.
- Gate semântico estruturado aprova identidade, suporte, não extrapolação, atribuição, grau, ausência de inferência ilícita e ambiguidades vazias.
- Política Go publica A/B aprovados e C aprovado com linguagem/limites explícitos; D/E ficam em quarentena.
- Homônimo, source `unreachable`, acusação criminal não confirmada, divergência entre estágios ou rejeição semelhante sempre levam a quarentena; `not_checked` respeita a configuração conservadora vigente.

## Métricas e disposição

- `E — fraco/corrigido` importa como `published`, `context_only` e não elegível a métricas de rede.
- `E — fraco/ambíguo` importa como `quarantined`, `possible_link` e não elegível a métricas.
- Métricas de contato/vínculo exigem claim público e `metric_eligible = true`; conteúdo público não elegível continua em página e XLSX.

## Moderação e acessibilidade

- `moderation_decisions` rejeita migrations que preencham nenhum ou ambos os alvos.
- Uma source usada por dois claims continua válida no segundo quando o primeiro uso é rejeitado.
- Rejeitar o último `supports` ativo põe o claim em quarentena na mesma transação.
- O verificador usa GET limitado, bloqueia destino privado em redirects, não persiste corpo e nunca depende apenas de HEAD.
- `unreachable` leva a quarentena; timeout/429/5xx vira `not_checked`, sem rejeição, e segue a configuração vigente.

## Não bloqueiam o MVP

MFA, RBAC multiusuário, dupla revisão, snapshots, trilha imutável, métricas históricas, cobertura abrangente, E2E, observabilidade completa, HA, PostgreSQL e política jurídica operacional extensa.
