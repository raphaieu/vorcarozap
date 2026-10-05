# ADR-031: Prevenção de Exposição de Credenciais em URLs Externas (VZ-032)

## Status

Aceito

## Contexto

O VorcaroZAP armazena referências a fontes documentais, notícias e pareceres jurídicos que sustentam as alegações catalogadas. Essas URLs externas são exibidas em diversas superfícies de saída:
1. **Páginas Públicas SSR** (detalhes de entidades, catálogo documental `/documentos`, visualizador de documentos `/documentos/{id}` e manifestações de defesa).
2. **API Pública JSON** (`/api/v1/pessoas`, `/api/v1/pessoas/{slug}`, `/api/v1/documentos`, `/api/v1/documentos/{id}`).
3. **Exportação Pública em Planilha** (`/exportar/base.xlsx`).
4. **Telas Administrativas de Pós-Moderação** (`/admin/candidatos`, `/admin/fontes`, `/admin/evidencias`, `/admin/manifestacoes`).

Até a VZ-031, a função `normalize.SafeURL` validava o esquema (`http`/`https`), o host e a presença de `userinfo` (ex.: `http://user:pass@host`), rejeitando esquemas perigosos e credenciais embutidas na autoridade da URL, além de remover fragmentos (`#...`). No entanto, a query string completa (`?param=...`) era preservada sem inspeção de parâmetros.

Conforme reconhecido no ADR-030, URLs contendo credenciais, tokens privados de sessão, senhas, chaves de API ou parâmetros de URLs assinadas de provedores de nuvem (ex.: `?token=...`, `?auth_token=...`, `?X-Amz-Signature=...`, `?x-goog-signature=...`, `?sig=...`) poderiam ser inadvertidamente expostas se tais links chegassem à plataforma por ingestão ou submissão.

## Decisão

Implementar uma política explícita, centralizada e conservadora para URLs apresentadas ao usuário em todas as superfícies de saída, sem alterar o armazenamento persistido nem as identidades/fingerprints de candidatos na ingestão.

### 1. Detecção Centralizada de Parâmetros Sensíveis em `internal/normalize`

Na camada de normalização, adicionam-se as funções `IsSensitiveQueryParamName`, `HasSensitiveQueryParams` e aprimora-se `SafeURL`:

- **Insensibilidade a maiúsculas/minúsculas e separadores**: Nomes de parâmetros são avaliados em minúsculas, com normalização de separadores (`-` e `_`).
- **Suporte a chaves codificadas na query**: A validação decodifica chaves via `url.QueryUnescape` antes da comparação.
- **Fail-Closed em consultas malformadas**: Se a query string não puder ser parseada ou contiver percent-encoding inválido na chave, a URL é tratada como insegura.
- **Cobertura de parâmetros sensíveis**:
  - Tokens e autorizações: `token`, `access_token`, `auth_token`, `api_token`, `jwt`, `bearer`, `bearer_token`, `auth`, `authorization`, `session`, `session_token`, `session_id`, `otp`, `passcode`, `assertion`, `code`, `cf_token`, `token_hash`, `secure_token`.
  - Senhas, segredos e chaves: `key`, `apikey`, `api_key`, `access_key`, `auth_key`, `secret_key`, `secret`, `api_secret`, `client_secret`, `shared_secret`, `password`, `pass`, `pwd`, `credential`, `credentials`.
  - Assinaturas criptográficas: `sig`, `signature`, `sign`, `signed_url`, `hmac`, `sas_token`.
  - Provedores de nuvem (URLs assinadas AWS S3, GCP Cloud Storage, Azure Blob SAS):
    - AWS: `x-amz-signature`, `x-amz-credential`, `x-amz-security-token`, `x-amz-algorithm`, `x-amz-date`, etc.
    - GCP: `x-goog-signature`, `x-goog-credential`, `x-goog-security-token`, `x-goog-algorithm`, `x-goog-date`, etc.
    - Azure SAS: `sig`, `sas_token`, `se`, `sp`, `sv`, `sr`, `skoid`, `sktid`.
- **Preservação de parâmetros legítimos**: Parâmetros comuns como `?id=123`, `?page=2`, `?q=busca`, `?sort=title`, `?categoria=advocacia` permanecem plenamente funcionais.

### 2. Omissão Completa do Link Externo Apresentado

Quando uma URL contiver qualquer parâmetro sensível (ou esquema inválido/userinfo):
- `normalize.SafeURL(raw)` retorna string vazia (`""`).
- **API Pública JSON (`/api/v1`)**: O campo `canonical_url` (ou `source_url` em manifestações) é retornado como string vazia `""` ou omitido via `omitempty`.
- **Páginas Públicas SSR**: Os templates verificam a presença da URL sanitizada antes de renderizar o elemento `<a>`. Links externos com parâmetros sensíveis são suprimidos da renderização HTML sem quebrar o layout nem gerar links quebrados.
- **Exportação XLSX**: A coluna "URL Canônica" na aba "Fontes e Documentos" recebe string vazia quando sanitizada.
- **Área Administrativa (`ExternalLinkVM`)**: A função `SanitizeExternalLink` zera o campo `RawURL` (`RawURL: ""`, `IsValid: false`) quando parâmetros sensíveis ou credenciais embutidas forem detectados, impedindo que o token seja renderizado em texto puro como fallback na interface administrativa.

### 3. Preservação de Armazenamento e Ingestão

- As tabelas `sources` (`canonical_url`, `original_url`) e `defense_statements` (`source_url`) no SQLite mantêm o registro conforme recebido/curado.
- Os fingerprints de candidatos e monitoramento (`FingerprintV1` em `internal/monitoring/rules.go`) permanecem inalterados, preservando a idempotência da esteira de ingestão.

### 4. Proteção em Logs e Telemetria

- O logger estruturado (`internal/observability/logger.go`) estende o mascaramento de atributos sensíveis e padrões de query string em URLs registradas nos logs, redigindo assinaturas (`sig`, `signature`), credenciais de nuvem (`x-amz-*`, `x-goog-*`) e tokens.

### 5. Proteção em Mensagens de Erro Operacionais e Monitoramento

- Falhas de normalização de URLs em candidatos de monitoramento (`internal/monitoring/service.go` e `internal/monitoring/runner.go`) registram apenas motivos de erro genéricos e o erro estruturado de normalização, sem interpolar a URL bruta recebida do provedor. Isso impede que URLs malformadas contendo tokens ou senhas sejam persistidas no campo `error_message` da tabela `monitoring_runs` ou no resumo técnico da execução (`technical_summary`).

---

## Consequências

### Positivas

- **Eliminação de vazamento de credenciais**: Impede que links com tokens temporários, senhas ou assinaturas de storage vazem para o público, indexadores ou usuários da API.
- **Transparência e conformidade**: Mantém links externos legítimos operacionais enquanto oculta silenciosa e seguramente acessos com credenciais embutidas.
- **Consistência em todas as saídas**: Comportamento idêntico no SSR público, API JSON, XLSX e painel administrativo.
- **Segurança Fail-Closed**: Queries truncadas ou com codificação malformada são rejeitadas por padrão.

### Negativas / Mitigações

- Links de fontes que dependem exclusivamente de URLs pré-assinadas temporárias não exibirão âncora externa no frontend público; tais fontes continuam citadas pelo título, veículo e metadados arquivísticos no catálogo público.
