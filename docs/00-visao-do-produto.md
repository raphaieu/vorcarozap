# Visão do produto

## Problema

Informações públicas sobre o caso Vorcaro/Banco Master estão fragmentadas em reportagens, documentos, decisões, entrevistas, mensagens e redes sociais. O VorcaroZAP organiza esse material numa página navegável, com nomes, contexto, relevância, classificação e links para as fontes.

O produto não decide culpa. Ele demonstra por que um nome aparece associado ao caso e qual fonte pública sustenta essa associação.

## Objetivo do primeiro lançamento

Publicar rapidamente uma experiência mobile first que permita:

- visualizar pessoas em ordem alfabética;
- pesquisar e filtrar registros;
- compreender cargo, relevância e tipo de relação;
- abrir as fontes datadas;
- distinguir confirmação, associação, alegação e pista;
- baixar a base disponível.

## Escopo do MVP rápido

- monólito Go;
- HTML server-side com `templ` e HTMX apenas onde agregar valor;
- SQLite;
- importação da planilha por comando local;
- página pública de leitura;
- resumo editorial simples;
- listagem, busca, filtros e detalhes;
- metodologia e disclaimer;
- download de XLSX;
- Docker/Caddy e deploy em uma VPS.

## Fora do primeiro lançamento

- painel administrativo;
- autenticação, MFA e RBAC;
- monitoramento automático e OpenRouter;
- uploads públicos;
- captura ou armazenamento integral de páginas;
- workflows editoriais multiusuário;
- auditoria imutável avançada;
- observabilidade e alertas sofisticados;
- suíte unitária, integração ou E2E abrangente;
- alta disponibilidade e múltiplas instâncias.

Esses itens permanecem documentados para evolução e não bloqueiam o início do desenvolvimento.

## Premissas

- poucos acessos e uma única instância no início;
- atualização manual por uma pessoa responsável;
- conteúdo baseado em material já publicado e com URLs registradas;
- nenhum endpoint público de escrita no MVP;
- português do Brasil, datas persistidas em UTC e exibidas em `America/Fortaleza`;
- a planilha atual é insumo de pesquisa e precisa de conferência editorial antes de virar página pública.

## Medida de sucesso inicial

O MVP está validado quando pode ser publicado na VPS, carrega a base, funciona bem no celular e permite chegar de um nome à fonte correspondente. Métricas avançadas serão adicionadas após existir tráfego real.

## Riscos registrados, não bloqueantes

Erro de identidade, perda de contexto, link indisponível, privacidade, direito de resposta, segurança do painel futuro, custo de LLM e escala. No MVP, a mitigação principal é conteúdo revisado manualmente, fonte visível, linguagem neutra, página somente de leitura e canal de correção.

