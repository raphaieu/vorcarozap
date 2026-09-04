# Segurança, privacidade e responsabilidade

## Baseline do MVP

O MVP possui painel e OpenRouter, portanto aplica controles pequenos e proporcionais:

- HTTPS por Caddy;
- `/admin` protegido por uma credencial forte configurada fora do Git;
- cookie/sessão ou Basic Auth protegida, com verificação de Origin/CSRF nas mutações;
- queries parametrizadas e templates escapados;
- API key do OpenRouter somente no ambiente;
- limites de tempo, resultados, tokens e custo por monitoramento;
- saída da descoberta validada estruturalmente e submetida a segunda avaliação semântica estruturada; a política final é aplicada em Go;
- conteúdo web tratado como dado, não instrução;
- nenhum HTML da LLM renderizado diretamente;
- telefones, documentos e endereços desnecessários removidos;
- rejeição de claim retira a alegação da página, métricas e exportação; rejeição de `evidence_source` afeta apenas aquele uso e pode colocar o claim sem suporte em quarentena;
- canal público de correção.

Não haverá upload público nem fetch/crawler genérico arbitrário no MVP. A única requisição direta é um verificador estreito de acessibilidade: GET com timeout e limite de bytes, sem persistir corpo, no máximo poucos redirects configuráveis e validação de esquema, DNS/IP e destino a cada salto. Loopback, redes privadas, link-local, metadata e portas não permitidas são bloqueados. HEAD não é usado.

## Risco editorial

Fontes datadas e atribuição reduzem risco, mas não transformam alegação em fato. C/D/E exigem linguagem explícita. O administrador pode remover rapidamente um item e registrar motivo. Conteúdo sem gates suficientes vai para quarentena.

D/E nunca são publicados automaticamente. Qualquer grau também vai para quarentena quando envolver homônimo, source `unreachable`, suporte insuficiente, acusação criminal não confirmada, PII desnecessária, divergência entre estágios ou fingerprint semelhante previamente rejeitado. `not_checked` não equivale a rejeição e segue política configurável, conservadora por padrão.

## Futuro

MFA/RBAC, dupla revisão, trilha imutável, snapshots, retenção formal, incident response, backup criptografado externo, SSRF hardening para crawler próprio, testes negativos abrangentes e validação jurídica aprofundada tornam-se prioritários com equipe, tráfego, crawler, uploads ou contestação relevante.

Esses itens ficam rastreados e não bloqueiam o MVP.
