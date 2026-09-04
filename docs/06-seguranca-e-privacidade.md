# Segurança, privacidade e responsabilidade

## Baseline do MVP

O MVP possui painel e OpenRouter, portanto aplica controles pequenos e proporcionais:

- HTTPS por Caddy;
- `/admin` protegido por uma credencial forte configurada fora do Git;
- cookie/sessão ou Basic Auth protegida, com verificação de Origin/CSRF nas mutações;
- queries parametrizadas e templates escapados;
- API key do OpenRouter somente no ambiente;
- limites de tempo, resultados, tokens e custo por monitoramento;
- saída da LLM validada por schema e regras locais;
- conteúdo web tratado como dado, não instrução;
- nenhum HTML da LLM renderizado diretamente;
- telefones, documentos e endereços desnecessários removidos;
- desaprovação retira o conteúdo da página, métricas e exportação;
- canal público de correção.

Não haverá upload público nem fetch genérico arbitrário no MVP. A pesquisa fica restrita à ferramenta do provedor e URLs são usadas como referência externa.

## Risco editorial

Fontes datadas e atribuição reduzem risco, mas não transformam alegação em fato. C/D/E exigem linguagem explícita. O administrador pode remover rapidamente um item e registrar motivo. Conteúdo sem gates suficientes vai para quarentena.

## Futuro

MFA/RBAC, dupla revisão, trilha imutável, snapshots, retenção formal, incident response, backup criptografado externo, SSRF hardening para crawler próprio, testes negativos abrangentes e validação jurídica aprofundada tornam-se prioritários com equipe, tráfego, crawler, uploads ou contestação relevante.

Esses itens ficam rastreados e não bloqueiam o MVP.

