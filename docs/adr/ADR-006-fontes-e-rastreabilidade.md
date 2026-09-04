# ADR-006 — Fontes e rastreabilidade por alegação

- **Status:** aceito
- **Data:** 2026-09-03; modelo ajustado em 2026-09-04

## Contexto

Uma mesma source pode sustentar claims diferentes, ser válida em um contexto e insuficiente em outro. Rejeitá-la globalmente apagaria utilizações legítimas. Ao mesmo tempo, URL citada pelo provedor não garante que esteja acessível por verificação direta.

## Decisão

Modelar `claim -> evidence -> evidence_source -> source`. Grau existe somente no claim. `evidence_source` possui ID próprio e aceita múltiplos trechos da mesma fonte. Toda publicação automática requer citação/URL e trecho/localizador suficiente.

No MVP, `evidence_source.role = contradicts` representa também defesa ou contestação e alimenta a seção pública “Defesa, contestação ou contexto”. Não duplicar o texto em tabela específica; `defense_statements` permanece P1.

Moderação atua exclusivamente sobre claim ou `evidence_source`, com constraint XOR. A source representa o documento e não recebe rejeição editorial global. Rejeitar um evidence_source remove apenas aquele uso; se o claim ficar sem `supports` ativo, ele vai para quarentena na mesma transação.

`sources.source_access_status` aceita `cited_by_provider`, `reachable`, `unreachable` e `not_checked`. Citação do OpenRouter começa em `cited_by_provider`. Um adaptador estreito faz GET seguro e limitado, sem persistir corpo e sem usar HEAD: sucesso promove para `reachable`; erro definitivo vira `unreachable`; falha temporária ou inconclusiva vira `not_checked`. `unreachable` impede publicação automática e `not_checked` segue política configurável, conservadora por padrão.

Dependência entre fontes, snapshots e cadeia de custódia ficam P1, mas o schema pode evoluir sem alterar a identidade dos registros atuais.

## Alternativas consideradas

- Rejeitar source globalmente: simples, mas remove usos independentes.
- Moderar somente claim: não permite corrigir uma citação específica preservando a alegação.
- Usar apenas a citação do provedor: barato, sem confirmação mínima de acessibilidade.
- Usar HEAD: leve, porém produz falsos negativos em muitos sites.
- Crawler/fetch completo: mais informação, superfície e custo desnecessários ao MVP.

## Consequências positivas

Permite auditoria pública suficiente, moderação granular e evita colocar URLs soltas na pessoa. A preservação integral de páginas não está garantida no MVP.

## Consequências negativas

Rejeição de uso exige recalcular invariantes do claim. O GET adiciona latência, falhas transitórias e cuidados de SSRF.

## Riscos

Redirect para rede privada, resposta grande, bloqueio anti-bot e fonte temporariamente fora do ar. Mitigar com validação por salto, timeout/limite de bytes, classificação `not_checked` e fail closed configurável.

## Gatilhos de revisão

Crawler próprio, snapshots, uploads, autenticação de fontes, incidentes SSRF ou necessidade de moderação global de documento exigem nova decisão.
