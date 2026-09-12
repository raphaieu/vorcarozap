# ADR-016 — Proteção simples do `/admin` por HTTP Basic Authentication e hash bcrypt

- **Status:** aceito
- **Data:** 2026-09-11
- **Fase:** VZ-014 (Fase 5 — Painel simples e moderação humana)

## Contexto

O VorcaroZAP opera como um monólito modular Go com SQLite, renderização SSR e instância única. Na Fase 5 do roadmap (painel simples e moderação humana), é necessária uma superfície de acesso administrativo protegida (`/admin`) para um operador responsável por pós-moderação, desaprovação de alegações/fontes e auditoria de candidatos em quarentena.

Em consonância com as diretrizes de segurança, arquitetura e economia do MVP:
1. O escopo do MVP não comporta nem requer infraestruturas complexas de autenticação (múltiplos usuários, banco relacional de credenciais, sessões em memória/Redis, cookies de sessão, JWT, fluxos de redefinição de senha, RBAC ou MFA).
2. O tráfego de produção trafega sob terminação TLS/HTTPS obrigatória fornecida pelo reverse proxy Caddy, tornando o canal seguro contra interceptação e espionagem de credenciais em trânsito.
3. A rota `/admin` precisa de proteção robusta contra exposição acidental, vazamento de segredos em logs/respostas, timing attacks de enumeração de usuário e acessos não autorizados (*fail-closed*).
4. Senhas em texto puro no ambiente (`ADMIN_PASSWORD`) trazem risco de vazamento em dumps de ambiente e erros operacionais, exigindo exclusivamente armazenamento e validação de hash criptográfico bcrypt forte (`ADMIN_PASSWORD_HASH`).

## Decisão

### 1. HTTP Basic Authentication no Monólito Go
 
Adota-se HTTP Basic Authentication nativa implementada via middleware isolado em `internal/web/auth.go`, atuando diretamente no topo da árvore de rotas montada em `/admin`:
- Toda e qualquer requisição direcionada a `/admin`, `/admin/` ou `/admin/*` (independente do método HTTP: GET, POST, PUT, PATCH, DELETE, OPTIONS) passa pelo `BasicAuthMiddleware` antes de qualquer decisão de roteamento, handler, 404 ou 405.
- Não há tabelas de usuários nem persistência de estado de autenticação no SQLite.
- O desafio emitido em caso de requisição não autenticada utiliza o cabeçalho padrão:
  `WWW-Authenticate: Basic realm="VorcaroZAP admin", charset="UTF-8"`
- Não se confia em cabeçalhos de proxy (`X-Forwarded-User`, `X-Remote-User`, `X-Admin`, etc.) para autorização de acesso.
- Não há bypass por parâmetros de query string, endereço IP de loopback ou flag de ambiente.

### 2. Configuração por Variáveis de Ambiente e Hash Bcrypt Obrigatório (Faixa 12–14)

A autenticação é parametrizada exclusivamente via:
- `ADMIN_USER`: nome do usuário administrativo. Deve ser não vazio após trim, limitado a 128 caracteres, sem o caractere delimitador `:` e sem caracteres de controle ASCII ou Unicode.
- `ADMIN_PASSWORD_HASH`: hash bcrypt válido gerado previamente a partir de uma senha forte (`$2a$`, `$2b$`, `$2y$`), com custo estritamente dentro da faixa permitida de **12 a 14** (`AdminPasswordBcryptMinCost = 12` e `AdminPasswordBcryptMaxCost = 14`).
- **Justificativa da faixa 12–14:** custos abaixo de 12 oferecem proteção insuficiente contra ataques de força bruta modernos em hashes vazados; custos acima de 14 causam latência operacional excessiva de processamento de CPU (> 1s por requisição), degradando o monólito.
- **Proibição de Senha em Texto Puro:** a aplicação não aceita, não implementa e não documenta variável `ADMIN_PASSWORD`.

### 3. Comportamento *Fail-Closed* e Validação Estrita no Startup

- **Ambos Ausentes (Desabilitado):** quando nem `ADMIN_USER` nem `ADMIN_PASSWORD_HASH` forem informados, a aplicação pública sobe normalmente, porém as rotas `/admin`, `/admin/` e subrotas respondem `404 Not Found` padrão, **sem** o cabeçalho `WWW-Authenticate`, reduzindo a superfície de descoberta.
- **Configuração Parcial ou Inválida (Bloqueio no Startup):** se apenas uma das duas variáveis for informada, ou se o usuário ou hash forem inválidos, `config.Load()` retorna erro seguro e impede o início da aplicação (*fail-fast*).
- **Sem Vazamento de Segredos:** mensagens de erro de configuração ou falhas de autenticação jamais contêm o hash, a senha ou credenciais parciais.

### 4. Mitigação de Timing Attacks e Enumeração de Usuário

No middleware de autenticação (`internal/web/auth.go`):
- A comparação do nome de usuário utiliza comparação em tempo constante (`crypto/subtle.ConstantTimeCompare`).
- A verificação `bcrypt.CompareHashAndPassword` é executada **incondicionalmente** com a senha enviada contra o hash configurado, de forma que o custo computacional de CPU seja idêntico quer o usuário exista ou não, impedindo que atacantes descubram se o nome de usuário está correto via medição de latência.
- Todas as falhas de autenticação (falta de cabeçalho, formato inválido, base64 corrompido, usuário incorreto ou senha incorreta) resultam exatamente na mesma resposta genérica `401 Unauthorized`.

### 5. Cabeçalhos de Segurança e Controle de Cache Restritivos

- Toda resposta da área administrativa (tanto 401 Unauthorized quanto 200 OK autenticado e 404 autenticado) define obrigatoriamente:
  - `Cache-Control: no-store`
  - `Vary: Authorization`
- Os cabeçalhos de segurança globais (`X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`) permanecem ativos.

### 6. Página SSR Mínima e Delimitação de Fases

- Na presente fase (VZ-014), a rota `GET /admin` renderiza uma página SSR neutra e informativa em Templ (`web/pages/admin.templ`).
- Nenhum dado factual, candidato descoberto, alegação, fonte, métrica interna, custo de LLM, nome de usuário ou botão de mutação é exibido.
- Não são criados endpoints `POST`, `PUT`, `PATCH` ou `DELETE` nesta fase. Proteções contra CSRF e validação de `Origin`/`Referer` serão implementadas obrigatoriamente nas etapas de ações de moderação (VZ-016 e VZ-021).

## Consequências

### Positivas
- **Simplicidade e robustez:** sem dependências de infraestrutura, sessões em cookies, migrations de banco ou bibliotecas externas complexas.
- **Segurança sólida:** combinando bcrypt, timing mitigation, fail-closed e HTTPS em produção via Caddy.
- **Zero impacto na área pública:** a navegação pública e métricas continuam totalmente desacopladas da autenticação administrativa.

### Limitações e Próximos Passos
- O sistema opera com uma única credencial administrativa compartilhada no MVP.
- Autenticação multiusuário com perfis (RBAC), histórico de login, trilha de auditoria e MFA constituem evolução proposta para a Fase pós-MVP (VZ-026).
- A inspeção e listagem de itens a moderar serão entregues em VZ-015; as mutações de moderação transacional XOR serão entregues em VZ-016/VZ-021.
