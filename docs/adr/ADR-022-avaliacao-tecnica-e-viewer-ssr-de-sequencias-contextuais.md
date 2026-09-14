# ADR-022: Avaliação técnica e visualizador SSR de sequências contextuais documentais

## Status

Aprovado

## Contexto

A entrega **VZ-023** da Fase 7 (pós-MVP) do VorcaroZAP propõe avaliar a viabilidade técnica, jurídica e editorial de visualização de documentos e sequências contextuais, mantendo o VorcaroZAP como um monólito Go, SQLite e SSR (Templ) em infraestrutura econômica.

Com a expansão de documentos primários referenciados (peças judiciais, laudos policiais como o IPJ-A nº 3298613/2026, relatórios periciais e reportagens apuradas), surgiu a necessidade de:
1. Avaliar se o VorcaroZAP deve atuar como proxy reverso de documentos remotos ou oferecer visualização direta de arquivos PDF/imagens externos.
2. Definir os limites de segurança contra Server-Side Request Forgery (SSRF), DNS rebinding, negação de serviço (DoS), exaustão de recursos em VPS econômica e injeção de scripts (XSS/CSP bypass).
3. Avaliar a conformidade jurídica com direitos autorais e licenças de código/dados de projetos de terceiros (como o repositório sem licença pública formal do MasterWhats/masterzap, conforme identificado no [ADR-012](ADR-012-navegacao-documental-e-referencias-a-acervos-externos.md)).
4. Definir a política de integridade documental e representação determinística de sequências contextuais sem inflar o schema relacional com tabelas desnecessárias.

## Avaliação de Viabilidade Técnica e Segurança

### 1. Inviabilidade e Riscos de Proxy Remoto Aberto (Fetch on-the-fly)

A realização de download e retransmissão síncrona (*proxying*) de arquivos PDF, imagens e páginas HTML de servidores remotos no momento da requisição do leitor foi avaliada como **insegura e inviável** pelos seguintes motivos:
- **Esgotamento de Recursos e DoS:** Servidores de tribunais e repositórios externos frequentemente hospedam arquivos PDF volumosos (50 MB a 500 MB). Baixar e processar esses arquivos em tempo real sob demanda na VPS de 1 vCPU e 1–2 GB de RAM levaria a esgotamento imediato de memória e banda.
- **Risco de SSRF e DNS Rebinding (TOCTOU):** Qualquer rotina de proxy sob URL controlada por usuário ou banco exigiria dupla validação de IP e resolução DNS em cada conexão, correndo risco de Time-of-Check to Time-of-Use (TOCTOU) se não isolada estritamente pelo transporte do `internal/sourcecheck`.
- **Execução de Conteúdo Ativo e Bypass de CSP:** Arquivos PDF remotos e páginas HTML externas podem conter JavaScript embutido, formulários maliciosos ou exploits para visualizadores de PDF embutidos em navegadores, violando a Content Security Policy e as garantias de segurança do VorcaroZAP.
- **Latência e Indisponibilidade:** A dependência síncrona de portais externos (sujeitos a instabilidades, paywalls, Cloudflare 403 e limites de taxa 429) degradaria a experiência SSR do leitor.

### 2. Conformidade de Direitos Autorais e Propriedade Intelectual

- Conforme registrado na [ADR-012](ADR-012-navegacao-documental-e-referencias-a-acervos-externos.md), o repositório público `rafaelbressan/masterzap` e outras fontes de terceiros não possuem licença explícita de código aberto. O VorcaroZAP não incorpora código, bibliotecas, datasets JSON/JSONL ou arquivos brutos desses repositórios.
- O VorcaroZAP opera como um **índice documental investigativo e cadeia de custódia editorial**, fundamentado no direito de citação e citação jornalística/acadêmica (Lei nº 9.610/1998, art. 46), organizando metadados, links externos canônicos e trechos literais pontuais estritamente necessários (`excerpt` e `locator`).
- Nenhum PDF integral ou cópia não autorizada de obra de terceiros é persistido ou redistribuído na aplicação.

### 3. Integridade do Conteúdo Observado vs. Autenticidade Jurídica

- A integridade documental registrada na plataforma reflete exclusivamente o **conteúdo técnico observado na data e hora UTC da última verificação** (`source_access_checked_at`, status HTTP e código técnico de verificação do `internal/sourcecheck`).
- A aplicação **não emite declarações de autenticidade jurídica** nem certificação cartorial ou pericial: o status indica apenas se o documento na URL canônica estava acessível (`reachable`), inacessível (`unreachable`) ou não verificado (`not_checked`) na data da checagem.

### 4. Representação de Sequências Contextuais no Schema Atual

- O schema relacional consolidado no MVP (`sources`, `evidence_sources`, `evidence`, `claims`, `relationships`, `entities`) é **100% suficiente** para representar sequências contextuais de documentos sem necessidade de migrações ou novas tabelas.
- Cada fonte (`sources`) pode possuir múltiplos usos de evidência (`evidence_sources`), cada um com seu localizador cirúrgico (`locator`), trecho literal (`excerpt`), papel probatório (`supports`, `contradicts`, `contextualizes`) e vínculo a alegações (`claims`).

## Decisão

1. **Não implementação de proxy de arquivos remotos:**
   O VorcaroZAP não faz download sob demanda de PDFs/HTML remotos para repassar aos navegadores dos leitores.

2. **Implementação de Visualizador SSR Seguro de Documentos e Sequências Contextuais:**
   - **Rota pública `/documentos/{id}`:** Renderizada em Server-Side Rendering puro (Go + Templ), exibindo:
     - Metadados canônicos do documento: título, veículo/autor, data de publicação, tipo canônico (`court_document`, `police_report`, `article`, etc.), classificação primária/secundária e link canônico externo seguro (`target="_blank" rel="noopener noreferrer"`).
     - Sequência contextual cronológica/documental ordenada de todos os trechos (`evidence_sources`) e localizadores vinculados a alegações públicas ativas (`claims.status = 'published'` AND `evidence_sources.status = 'active'`).
     - Quadro de integridade técnica observada (status de acessibilidade, código HTTP, carimbo de verificação UTC).
     - Aviso editorial explícito: menção em documento primário não implica culpa, interlocução direta ou responsabilidade penal/civil.
   - **Rota administrativa `/admin/fontes/{id}`:** Inspeção profunda protegida por Basic Auth, exibindo a fonte global, diagnósticos técnicos e a lista integral de trechos ativos e desaprovados com status dos claims associados e links diretos para deliberação de moderação.

3. **Ordenação Determinística de Sequências:**
   - As sequências contextuais são ordenadas deterministicamente no pacote puro `internal/normalize` (`CompareLocators`), aplicando ordenação numérica natural sobre localizadores (páginas, folhas, figuras, itens, anexos, artigos), com desempate determinístico por data de criação e ID.

4. **Isolamento da Fronteira Pública:**
   - Registros em quarentena, rejeitados ou arquivados são rigorosamente omitidos da visualização pública `/documentos/{id}` e devolvem HTTP 404 seguro se a fonte não possuir nenhuma alegação pública ativa.
   - Cabeçalhos de cache dinâmicos `Cache-Control: no-cache, no-store, must-revalidate` garantem invalidação imediata após deliberações de moderação.

## Consequências

### Positivas
- Fornece visualização contextual rica e cronológica de laudos e peças documentais sem violar direitos autorais nem criar dependência de JavaScript/SPAs.
- Protege a infraestrutura contra DoS, esgotamento de memória e vulnerabilidades SSRF ao evitar proxy reverso de arquivos remotos.
- Mantém o schema do SQLite limpo, sem migrações ou tabelas redundantes.
- Preserva a regra de ouro editorial: menção não implica conluio, interlocução ou culpa.

### Negativas e Limitações
- O leitor consulta o arquivo binário integral diretamente no servidor do publicador original através de link canônico sanitizado.
- O VorcaroZAP não faz parsing óptico (OCR) automático de imagens de mensagens; a extração de trechos depende da esteira de curadoria e monitoramento estruturado.
