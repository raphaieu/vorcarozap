# Visão do produto

## Problema e proposta

As informações do caso estão fragmentadas em mídia, documentos, decisões, entrevistas, mensagens e redes. O VorcaroZAP monitora, organiza e publica essas associações com fontes, datas, classificação e métricas.

O produto não decide culpa. Ele responde por que um nome aparece relacionado, quem publicou a informação, qual a força documental e quando houve atualização.

## Resultado do MVP

- página mobile first com resumo, métricas, busca, filtros, nomes e detalhes;
- fontes e evidências acessíveis;
- planilha inicial importada;
- novas fontes encontradas automaticamente via OpenRouter;
- conteúdo A/B válido publicado automaticamente e C publicado apenas com linguagem e limites explícitos;
- conteúdo D/E colocado em quarentena por padrão;
- conteúdo duvidoso colocado em quarentena;
- painel simples para localizar, desaprovar, restaurar e revisar;
- métricas recalculadas após ingestão ou moderação.

## Modelo operacional

O MVP usa pós-moderação. Primeiro, Go aplica um gate estrutural mecânico. Depois, uma segunda avaliação da LLM verifica identidade, suporte, atribuição, grau, extrapolação e inferência ilícita. A LLM recomenda; a política Go decide. A/B podem seguir automaticamente, C exige limites explícitos e D/E vão para quarentena por padrão. O administrador pode remover, restaurar ou aprovar itens.

## Fora do MVP

RBAC multiusuário, MFA, dupla revisão, CMS completo, snapshots integrais, cadeia de custódia, alertas sofisticados, HA, múltiplas instâncias, API pública e suíte ampla de testes.

## Métricas públicas essenciais

- pessoas/organizações ativas;
- fontes públicas ativas;
- alegações por grau A–E;
- entidades por categoria e relevância;
- registros adicionados/atualizados recentemente;
- data/hora do último monitoramento concluído.

Confiança técnica da IA não é exibida como probabilidade de verdade ou culpa.

## Premissas

- uma instância e um administrador no início;
- volume e tráfego baixos/moderados;
- fontes são públicas e registradas por URL/data;
- atualizações automáticas têm limites de custo;
- conteúdo desaprovado deixa imediatamente a página e os números públicos;
- português do Brasil, timestamps em UTC e exibição em `America/Fortaleza`.

## Riscos registrados

Erro de identidade, fontes circulares, perda de contexto, links indisponíveis, prompt injection, custo de LLM, contestação e dados pessoais. São tratados inicialmente por gates, quarentena, fonte visível, pós-moderação e canal de correção; controles avançados evoluem com uso real.
