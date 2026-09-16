# ADR-024: Módulo de contraditório estruturado e submissão pública de manifestações de defesa

## Status

Aprovado

## Contexto

A entrega **VZ-025** do VorcaroZAP estabelece a criação de um fluxo público, estruturado e auditável para que pessoas mencionadas, seus representantes legais ou interessados apresentem manifestações de defesa, retificação, esclarecimento ou contexto adicional relacionadas a alegações documentadas (`claims`).

Em plataformas investigativas e documentais:
1. É fundamental garantir o direito ao contraditório e à ampla defesa, permitindo o envio de contrapontos e retificações;
2. **Nenhuma submissão pública pode ser publicada automaticamente** nem alterar diretamente o status editorial do claim, entidades, relações, fontes (`sources`), usos de evidência (`evidence_sources`) ou a visão canônica `public_claims_view` e métricas de rede;
3. É imperativo assegurar proteção rigorosa contra abusos (injeção de scripts, payloads excessivos, scraping e spam), adotando normalização Unicode, limites de tamanho e taxa (*rate limiting*);
4. **Privacidade e sigilo de dados de contato:** dados pessoais de contato fornecidos (como e-mail, telefone ou nome do declarante) são estritamente confidenciais e exclusivos do painel administrativo, sendo terminantemente proibida sua exposição nas páginas públicas;
5. Decisões editoriais sobre manifestações devem ser tomadas exclusivamente por operadores humanos autenticados no `/admin`, registradas em histórico imutável com controle de concorrência otimista (OCC), proteção CSRF e padrão Post/Redirect/Get (303 See Other).

## Decisão

### 1. Modelo de Dados e Ciclo de Vida da Manifestação

Cria-se a tabela relacional `defense_statements` e a tabela imutável de deliberações `defense_statement_decisions`:

- **`defense_statements`**:
  - `id TEXT PRIMARY KEY`: identificador unívoco (UUID v4);
  - `claim_id TEXT NOT NULL REFERENCES claims(id) ON DELETE RESTRICT`: vínculo relacional obrigatório com uma alegação existente;
  - `statement_type TEXT NOT NULL`: categorização estruturada (`correction`, `rebuttal`, `clarification`, `additional_context`);
  - `title TEXT NOT NULL`: título suscinto (1 a 200 caracteres);
  - `content TEXT NOT NULL`: texto detalhado da manifestação (10 a 5000 caracteres);
  - `source_url TEXT NOT NULL DEFAULT ''`: link opcional para documento comprobatório ou fonte oficial externa (esquemas `http`/`https` validados, máx. 1000 caracteres);
  - `contact_info TEXT NOT NULL DEFAULT ''`: dado privado de contato (máx. 255 caracteres), tratado como confidencial;
  - `status TEXT NOT NULL DEFAULT 'quarantined'`: estado operacional/editorial (`quarantined`, `under_review`, `accepted`, `rejected`, `archived`);
  - `created_at` e `updated_at`: carimbos temporais UTC ISO-8601 RFC3339Nano.

- **`defense_statement_decisions`**:
  - `id TEXT PRIMARY KEY`: identificador da decisão (UUID v4);
  - `statement_id TEXT NOT NULL REFERENCES defense_statements(id) ON DELETE RESTRICT`: manifestação deliberada;
  - `action TEXT NOT NULL`: ação executada (`accept`, `reject`, `review`, `archive`);
  - `reason TEXT NOT NULL`: justificativa editorial fundamentada (1 a 1000 caracteres, sem HTML);
  - `actor TEXT NOT NULL`: operador administrativo autenticado (1 a 128 caracteres);
  - `created_at`: data/hora UTC da decisão.

### 2. Quarentena Obrigatória e Isolamento da Fronteira Pública Canônica

- **Quarentena Inicial:** Toda submissão pública inicia compulsoriamente com `status = 'quarantined'`.
- **Fronteira Canônica para Submissão e Formulário:** Tanto o formulário `GET /manifestar` quanto o endpoint `POST /manifestar` e o serviço de submissão consultam exclusivamente a visão canônica `public_claims_view` (requerendo `status = 'published'` e ao menos uma evidência ativa com papel `supports`). Claims inexistentes, rejeitados ou em quarentena recebem tratamento neutro sem vazar proposições ou entidades privadas.
- **Invisibilidade Pública de Estados Não-Aceitos:** Manifestações em `quarantined`, `under_review`, `rejected` ou `archived` são 100% invisíveis na área pública.
- **Exibição Condicional de Aceitas:** Somente manifestações com `status = 'accepted'` vinculadas a claims presentes na `public_claims_view` são renderizadas publicamente sob a seção de contraditório da alegação (`/pessoas/{slug}`). A consulta pública realiza `JOIN public_claims_view`, garantindo que manifestações aceitas fiquem ocultas caso a alegação perca seu suporte probatório ativo.
- **Isolamento de Métricas e Rede:** Manifestações não criam nem alteram claims, nós de entidade, arestas de relacionamento ou fontes documentais globais, preservando a fidelidade da `public_claims_view` e das métricas de rede.

### 3. Proteção Contra Abuso, Sanitização e Validação Rigorosa

- **Limitação de Payload:** O endpoint de submissão `POST /manifestar` impõe limite rígido de 64 KiB no corpo da requisição (`http.MaxBytesReader`).
- **Sanitização Estrita:** Rejeição de qualquer caractere ou tag HTML/script (`<`, `>`, `javascript:`, etc.) e normalização pura Unicode NFC.
- **Validação Rigorosa de URLs:** Links fornecidos em `source_url` são validados via `net/url.ParseRequestURI`, rejeitando esquemas não-HTTP (ex.: `javascript:`, `data:`, `ftp:`), credenciais embutidas (`user:pass@`), hosts vazios, hostnames inválidos, caracteres de controle ASCII (< 32, == 127) e comprimentos excessivos (> 1000 caracteres).
- **Rate Limiting Anti-Spoofing:** Implementação de controle de taxa em memória thread-safe por IP do cliente (máx. 5 submissões por janela de 10 minutos por IP) no monólito Go. O IP é extraído estritamente de `r.RemoteAddr` via `net.SplitHostPort`, sem confiar em cabeçalhos de proxy forjáveis (`X-Forwarded-For`, `X-Real-IP`), retornando `HTTP 429 Too Many Requests` em caso de excesso.
- **Mensagens Neutras:** Respostas ao usuário público utilizam mensagens genéricas e seguras, evitando vazamento de erros de banco de dados, probing ou enumeração interna.

### 4. Gestão Administrativa, Concorrência Atômica e Auditoria

- **Inspeção no `/admin`:** Operadores autenticados podem listar (`/admin/manifestacoes`) e inspecionar (`/admin/manifestacoes/{id}`) o conteúdo integral das manifestações, incluindo o dado privado `contact_info` para fins de verificação de autenticidade.
- **Controle Otimista de Concorrência Atômico (OCC):** Moderações executam a instrução atômica `UPDATE defense_statements SET status = ?, updated_at = ? WHERE id = ? AND updated_at = ?` (:execrows). Se zero linhas forem afetadas por conflito de versão, a transação aborta e retorna `HTTP 409 Conflict`.
- **Proteção CSRF e PRG:** As ações mutantes exigem correspondência exata do cabeçalho `Origin` com `ADMIN_ALLOWED_ORIGIN` e aplicam redirecionamento Post/Redirect/Get (303 See Other).
- **Trilha de Auditoria Imutável:** Todas as transições são registradas atomicamente em `defense_statement_decisions`.

## Consequências

### Positivas
- Cria um canal institucional, seguro e auditável para exercício de contraditório e retificações;
- Protege a integridade editorial da plataforma contra injeção e publicação não autorizada;
- Garante conformidade com princípios de minimização de dados e privacidade;
- Mantém o monólito Go puro, sem acoplamento a serviços externos ou filas complexas.

### Limitações
- O rate limiter padrão atua na memória da instância única do monólito Go;
- A verificação de identidade dos manifestantes depende de contato humano ou análise documental pelos operadores no painel administrativo.
