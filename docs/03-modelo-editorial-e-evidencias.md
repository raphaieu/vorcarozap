# Modelo editorial e evidências

## Unidades

Entidade é pessoa/organização. Relação fornece contexto. Alegação é uma proposição atribuível. Evidência descreve o suporte. Fonte identifica a publicação verificável.

## Dimensões

- **Grau A–E:** força pública da alegação; pertence à alegação.
- **Relevância 1–5:** alcance da entidade; não mede culpa ou proximidade.
- **Confiança técnica:** qualidade da extração/deduplicação; pertence ao candidato/run e não aparece como verdade pública.
- **Disposição:** papel editorial do claim em relação ao vínculo; não se confunde com grau.

Valores iniciais de `disposition`: `supports_link`, `possible_link`, `contradicts_link`, `context_only` e `correction`. `metric_eligible` controla participação nas métricas de rede. Claims públicos de contexto/correção continuam visíveis, mas não aumentam contagens de contatos ou vínculos.

## Escala

| Grau | Significado | Publicação automática |
|---|---|---|
| A | documento/registro/manifestação direta contextualizada | sim, se gates passarem |
| B | reportagem fundamentada ou confirmação independente | sim, se gates passarem |
| C | associação documentada com significado incompleto | sim, explicitando limites |
| D | alegação atribuída sem confirmação independente | quarentena por padrão; painel pode aprovar |
| E | pista/menção indireta | quarentena por padrão; painel pode aprovar |

Sem URL pública, entidade inequívoca, linguagem compatível, trecho/localizador suficiente ou data mínima, o candidato vai para quarentena.

## Estados e visibilidade

```text
quarantined  -> não público; aguarda administrador
published    -> público; entra nas métricas
rejected     -> não público; decisão evita republicação automática
archived     -> não público; preservado para histórico
```

Somente `published` aparece em páginas, busca e XLSX. Métricas de rede exigem também `metric_eligible = true`. Entidade sem claim/relação publicada não aparece por associação vazia. Moderar claim ou `evidence_source` recalcula a visibilidade derivada.

## Gate estrutural determinístico

Executado em Go, sem julgamento semântico:

- URL `http/https` válida;
- título/veículo ou emissor;
- trecho/localizador suficiente;
- datas normalizadas e grau A–E válido;
- entidade vinculada;
- ausência de telefone/documento/endereço desnecessário;
- fingerprint não rejeitado anteriormente;
- limites de tamanho, deduplicação e custo dentro do orçamento;
- validação local do schema e das citações retornadas.

A acessibilidade da fonte usa `cited_by_provider`, `reachable`, `unreachable` ou `not_checked`. Conforme [ADR-006](file:///home/raphael/personal/vorcarozap/docs/adr/ADR-006-fontes-e-rastreabilidade.md), `unreachable` sempre leva a quarentena; `not_checked` não equivale a rejeição e sua política diferencia a origem: `openrouter` + `not_checked` vai para quarentena, enquanto `curated_seed` + `not_checked` preserva o `initial_state` do mapeamento versionado.

## Gate semântico

Uma segunda avaliação da LLM retorna JSON estruturado sobre: correspondência de identidade; suporte direto do trecho; extrapolação da síntese; atribuição explícita; compatibilidade do grau; inferência de culpa/ilícito; lista de ambiguidades; e ação recomendada. Essa avaliação não publica: a política Go consome o resultado e toma a decisão.

```json
{
  "identity_match": true,
  "claim_supported": true,
  "claim_overstates_source": false,
  "attribution_explicit": true,
  "grade_compatible": true,
  "contains_illicit_inference": false,
  "uncertainties": [],
  "recommended_action": "publish"
}
```

A/B são publicáveis quando os dois gates passam. C também exige linguagem de associação e limites explícitos. D/E entram em quarentena por padrão. Qualquer grau entra em quarentena diante de homônimo, source `unreachable`, trecho insuficiente, acusação criminal não confirmada, PII desnecessária, divergência entre estágios ou rejeição semelhante; `not_checked` segue a política diferenciada por origem definida no [ADR-006](file:///home/raphael/personal/vorcarozap/docs/adr/ADR-006-fontes-e-rastreabilidade.md).

Falha em gate gera quarentena, não descarte.

## Importação curada

A planilha inicial usa `origin = curated_seed`. Ela passou por curadoria prévia, portanto segue política própria: identidade, contexto/cargo, explicação, fonte principal, classificação legada reconhecida e texto não extrapolado permitem aplicar o `initial_state` do mapeamento; caso contrário, `quarantined`. O mapeamento legado define `grade`, `initial_state`, `disposition` e `metric_eligible`, é versionado fora do importador e tem sua versão registrada na execução.

No mapeamento v1, A/B/C sustentam vínculo e são elegíveis às métricas; D é publicado como vínculo possível e elegível; E ambíguo fica em quarentena e não é elegível; E corrigido é contexto público e não entra nas métricas de rede.

## Relevância

1 local/circunstancial; 2 setorial/regional; 3 nacional moderada; 4 alta nacional; 5 estratégica/internacional. Justificativa factual obrigatória.

## Fontes

Registrar título, publicador/autor, URL canônica, publicação/acesso, tipo, trecho/localizador e papel (`supports`, `contradicts`, `contextualizes`). Múltiplos veículos que derivam da mesma origem não contam automaticamente como confirmações independentes.

No MVP, `contradicts` também representa defesa ou contestação e é exibido em “Defesa, contestação ou contexto”. Não há duplicação do texto em tabela própria. `defense_statements` e pedidos de resposta estruturados ficam P1.

## Unidade de moderação

A `source` representa o documento e não é rejeitada globalmente. A decisão de moderação aponta exatamente para um `claim` ou um `evidence_source`:

- rejeitar o claim remove integralmente a alegação da área pública;
- rejeitar um `evidence_source` remove somente aquela utilização da fonte;
- a mesma source continua disponível para outros claims;
- se um claim publicado ficar sem `evidence_source` ativo com papel `supports`, ele muda atomicamente para `quarantined`;
- restauração reavalia os mesmos invariantes antes de tornar o claim público.

Snapshots, cadeia de custódia e licença avançada ficam P1. Canal de correção e linguagem neutra entram no MVP.
