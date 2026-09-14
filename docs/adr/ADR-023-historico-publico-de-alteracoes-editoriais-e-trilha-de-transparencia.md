# ADR-023: Histórico público de alterações editoriais e trilha de transparência

## Status

Aprovado

## Contexto

A entrega **VZ-024** da Fase 7 (pós-MVP) do VorcaroZAP estabelece a implementação do **Histórico Público de Alterações Editoriais** e da trilha de transparência pública da plataforma, preservando integralmente a auditoria administrativa completa já existente em `moderation_decisions` ([ADR-017](ADR-017-moderacao-humana-de-claims-transacoes-e-protecao-csrf.md), [ADR-018](ADR-018-moderacao-granular-de-evidence-sources-e-quarentena-por-perda-de-suporte.md)).

Em investigações de interesse público com múltiplas fontes e checagens documentais:
1. Os leitores precisam compreender, de forma neutra, transparente e segura, quando uma informação pública foi publicada, quando teve sua sustentação probatória alterada, quando foi reavaliada ou quando foi retirada/recolhida para quarentena.
2. É imperativo manter a **segregação estrita entre a auditoria administrativa e a transparência pública**:
   - A trilha administrativa completa (`/admin`) registra operadores autenticados (`actor`), justificativas internas fundamentadas (`reason`), hashes de fingerprint de monitoramento (`candidate_fingerprint`) e integridade relacional XOR;
   - O histórico público (`/pessoas/{slug}` e `/documentos/{id}`) deve exibir apenas eventos relevantes ao conteúdo público ou que justifiquem neutralmente sua desativação, redigindo qualquer identificador de credencial/operador, segredos, raw responses ou dados sensíveis.
3. Alegações puramente privadas, em quarentena ou rejeitadas nos gates que nunca foram públicas não podem vazar nem ter suas proposições inferidas através do histórico público.

## Decisão

### 1. Reutilização Integral do Schema e Ausência de Migrações Redundantes

- Conforme avaliação técnica, o schema relacional consolidado no MVP (`moderation_decisions`, `claims`, `evidence_sources`, `evidence`, `sources`, `relationships`, `entities`) e a view canônica `public_claims_view` são **100% suficientes** para derivar a trilha de transparência pública.
- Não foram criadas novas tabelas nem aplicadas migrações destrutivas, preservando a simplicidade do monólito Go e SQLite WAL em instância única.

### 2. Modelo Público Redigido e Política Estrita de Privacidade

Define-se o tipo de domínio `domain.PublicEditorialEvent` e o ViewModel `pages.PublicEditorialEventVM` contendo apenas informações seguras:
- **Identificação do Operador (`actor`):** Redigida incondicionalmente na visualização pública. O sistema exibe o rótulo coletivo e institucional `Equipe Editorial` ou omite o autor individual, impedindo enumeração de credenciais ou nomes de moderadores.
- **Justificativa (`reason`):** A justificativa administrativa bruta (que pode conter anotações de trabalho internas ou referências investigativas) não é repassada diretamente ao leitor. No lugar dela, o sistema gera resumos neutros, objetivos e padronizados baseados na ação deliberada e no tipo de alvo.
- **Fingerprints e Payloads Técnicos:** O hash SHA-256 do candidato (`candidate_fingerprint`), payloads JSON brutos (`raw_response`, `raw_payload`), contagens de tokens e custos em micro-USD permanecem estritamente isolados na área restrita `/admin`.
- **Proteção de Conteúdo Não Público:** Se uma alegação previamente publicada for rejeitada (`action = 'reject'`) ou recolhida para quarentena, o histórico público exibe um aviso neutro ("Alegação desaprovada e retirada da visualização pública e métricas após revisão editorial"), omitindo o texto da proposição (`ClaimProposition = ""`) para evitar reexposição de dados não sustentados. Decisões sobre itens que nunca foram públicos são integralmente filtradas na consulta.

### 3. Visualização Pública SSR em Entidades e Documentos

- **Detalhe Público de Entidade (`/pessoas/{slug}`):** Adição da seção "Histórico de Alterações Editoriais", compilando os eventos de aprovação/publicação, retiradas editoriais, restaurações para quarentena e alterações em suportes de evidência vinculados à pessoa/organização.
- **Detalhe Público de Documento (`/documentos/{id}`):** Adição da seção "Histórico de Alterações Editoriais do Documento", apresentando a cadeia de custódia e registros de inclusão, desativação ou reativação dos trechos e citações daquela fonte.
- **Diferenciação no Painel Administrativo (`/admin`):** As seções de auditoria em `/admin/claims/{id}` e `/admin/evidencias/{id}` recebem destaque visual explícito como `Auditoria Administrativa Completa (Privada)`, mantendo todos os dados operacionais (actor, reason, fingerprint) visíveis aos operadores autorizados.

### 4. Ordenação Determinística e Invalidação Imediata de Cache

- Todas as consultas de histórico aplicam ordenação determinística decrescente por carimbo UTC e ID (`ORDER BY created_at DESC, id DESC`), com limite conservador padrão (`DefaultEditorialHistoryLimit = 50`).
- Todas as rotas públicas que expõem histórico mantêm os cabeçalhos `Cache-Control: no-cache, no-store, must-revalidate` ([ADR-019](ADR-019-invalidacao-imediata-de-visualizacoes-publicas-metricas-e-exportacao.md)), garantindo que qualquer ação de moderação humana reflita instantaneamente no histórico público na próxima requisição.

## Consequências

### Positivas
- Fornece aos leitores transparência editorial completa sobre o ciclo de vida das informações documentadas, fortalecendo a credibilidade do projeto.
- Garante total segurança e privacidade para operadores, impedindo vazamento de credenciais, notas internas ou dados de itens quarentenados.
- Preserva a integridade e imutabilidade da tabela `moderation_decisions` sem requerer nova infraestrutura de banco ou filas.
- Mantém o padrão SSR puro (Go + Templ) sem dependência de JavaScript.

### Limitações
- O histórico reflete eventos deliberados de moderação humana e as publicações canônicas dos registros ativos. Eventos operacionais internos de monitoramento de baixo nível (tentativas de raspagem, falhas de conectividade transitórias) pertencem ao log de `monitoring_runs` e não compõem o histórico público de transparência editorial.
