# ADR-005 — Publicação automática controlada e pós-moderação

- **Status:** aceito; substitui revisão prévia obrigatória no MVP
- **Data:** 2026-09-04

## Contexto

Aprovar manualmente cada item elimina a velocidade do monitoramento e exige painel complexo. Publicar tudo que uma LLM sugere cria risco editorial e reputacional. Parte das verificações é mecânica; identidade, suporte e extrapolação exigem avaliação semântica, mas a decisão final precisa permanecer em política de aplicação auditável.

## Decisão

Adotar dois gates:

1. gate estrutural determinístico em Go para schema, URL/metadados, trecho/localizador, datas, grau, entidade vinculada, PII, tamanho, deduplicação, fingerprint, acessibilidade conforme política e orçamento;
2. gate semântico por segunda avaliação estruturada da LLM para identidade, suporte, extrapolação, atribuição, compatibilidade do grau, inferência ilícita e ambiguidades.

A LLM somente recomenda. A política Go toma e registra a decisão final:

- A/B: publicação automática quando ambos os gates passam;
- C: publicação automática somente com linguagem de associação e limites explícitos;
- D/E: quarentena por padrão, com possível aprovação no painel;
- qualquer grau: quarentena diante de homônimo, source `unreachable`, suporte insuficiente, acusação criminal não confirmada, PII desnecessária, divergência entre estágios ou rejeição semelhante. `not_checked` segue configuração conservadora por padrão, sem equivaler a rejeição.

Um administrador pode desaprovar, restaurar ou aprovar quarentena. Fingerprint rejeitado bloqueia republicação automática semelhante. A moderação altera imediatamente consulta, métricas e exportação.

## Alternativas consideradas

- Aprovação humana prévia de todos os itens: menor janela de exposição, maior custo e perda do caráter automático.
- Uma única chamada de LLM: menor custo, sem separação entre descoberta e crítica semântica.
- Publicar D/E com linguagem atribuída: maior cobertura, risco desproporcional de associação frágil automática.
- Usar a recomendação da LLM diretamente: simples, mas transfere a política editorial ao modelo.

## Consequências positivas

Atualização rápida para evidências fortes, contenção conservadora de associações frágeis, decisão reproduzível em Go e painel pequeno.

## Consequências negativas

Duas avaliações aumentam custo e latência. Continua existindo janela entre publicação e eventual desaprovação. A qualidade depende de schema, fontes e política bem testados.

## Riscos

Concordância falsa entre modelos, fonte inacessível depois da avaliação, erro de identidade e bypass de política. Mitigar com modelos/configuração registrados, fail closed, testes table-driven, fingerprints e pós-moderação.

## Gatilhos de revisão

Contestação relevante, incidente, aumento de sensibilidade, equipe maior, baixa precisão dos gates ou mudança de política automática exige novo ADR e pode restabelecer aprovação prévia/dupla revisão.
