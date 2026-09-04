# Segurança, privacidade e responsabilidade

Este documento registra evolução técnica e editorial. No MVP, a estratégia de redução de risco é manter a aplicação pública somente para leitura, sem painel, contas, uploads ou coleta web automática.

## Baseline obrigatório e barato

- HTTPS por Caddy;
- aplicação sem endpoints públicos de escrita;
- templates com escape por padrão;
- queries parametrizadas;
- `.env`, chaves e banco fora do Git;
- nenhum segredo ou stack trace em resposta pública;
- somente URLs `http/https` exibidas como links;
- conteúdo da planilha tratado como texto, nunca HTML executável;
- telefones, documentos, endereços e dados pessoais desnecessários não são publicados;
- fonte, data e linguagem de atribuição ficam próximas da alegação;
- canal simples de contato/correção deve aparecer no site.

Esses controles acompanham o desenvolvimento porque custam pouco e evitam retrabalho óbvio.

## Modelo editorial inicial

O conteúdo vem de material público, mas a existência de fonte não transforma automaticamente uma alegação em fato. A publicação inicial deve:

- descrever exatamente o que a fonte afirma;
- evitar inferência de culpa;
- registrar data e URL;
- marcar pistas e alegações;
- corrigir material quando surgir informação melhor;
- oferecer contraditório quando disponível.

Validação jurídica formal, política completa de LGPD, licenciamento de snapshots e workflow de dupla revisão ficam registradas para amadurecimento. Não bloqueiam o protótipo nem o lançamento inicial, desde que o produto permaneça dentro do escopo público, referenciado e somente de leitura.

## Hardening futuro por gatilho

### Ao criar painel

Sessões seguras, Argon2id/OAuth, CSRF, RBAC, MFA, rate limit, trilha de ações e recuperação de conta.

### Ao aceitar uploads

Allowlist, assinatura do arquivo, tamanho máximo, isolamento, proteção contra ZIP bomb/path traversal e antivírus conforme exposição.

### Ao coletar URLs

Proteção SSRF, DNS/rede privada, redirects, timeout, limite de bytes e MIME.

### Ao ativar LLM

Defesa contra prompt injection, schema validado, limites de custo, minimização de dados, retenção do bruto e revisão humana.

### Ao crescer audiência/equipe

Política formal de privacidade, retenção, incidentes, backups criptografados, logs/alertas, revisão jurídica/editorial e testes negativos.

Riscos continuam rastreados. A ausência dessas implementações no MVP é uma decisão consciente de sequenciamento, não uma afirmação de que os riscos não existem.

