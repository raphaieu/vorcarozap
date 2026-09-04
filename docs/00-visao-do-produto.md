# Visão do produto

## Problema e proposta

Informações públicas sobre o caso estão fragmentadas entre reportagens, decisões, documentos, entrevistas, mensagens e redes sociais. O VorcaroZAP oferece uma base navegável e auditável para responder quem foi citado, em qual contexto, por quais fontes, com qual força documental, qual contraditório foi localizado e o que mudou.

O produto organiza o registro público; não investiga poderes que não possui, não decide culpa e não transforma proximidade em acusação. A unidade central é a **alegação verificável**, ligada a evidências e fontes, e não uma ficha acusatória sobre uma pessoa.

## Públicos e necessidades

- Cidadão: leitura rápida, linguagem clara e distinções visíveis.
- Jornalista/pesquisador: rastreabilidade por afirmação, datas, histórico e exportação.
- Pessoa mencionada: contexto, atribuição, contraditório, correção e contestação.
- Curador: fluxo especializado de descoberta, verificação, revisão, publicação e auditoria.

## Resultados esperados

Um leitor consegue localizar uma entidade, entender a natureza da associação, abrir as fontes exatas, diferenciar confirmação de pista, consultar defesa e data de revisão. Um curador consegue demonstrar quem alterou o quê, com qual fundamento e quando.

## Escopo do MVP

Página pública mobile first; busca, filtros e ordenação; páginas de entidades e fontes; linha do tempo; metodologia; painel protegido; importação da base inicial; monitoramento assistido por IA com aprovação humana; XLSX derivado do banco; correções, auditoria, backup e operação em uma VPS.

Fora do MVP: rede social, comentários públicos, denúncias anônimas automatizadas, aplicativo nativo, SPA, análise de culpabilidade, publicação autônoma por IA, microsserviços, alta disponibilidade e ingestão irrestrita da web.

## Métricas de produto e salvaguardas

- 100% das alegações publicadas com ao menos uma evidência e uma fonte acessível ou snapshot autorizado.
- 100% das mudanças publicadas com revisão e histórico.
- Tempo mediano para encontrar fontes de uma pessoa e taxa de buscas sem resultado.
- Percentual de registros com contraditório procurado, localizado e exibido.
- Correções: tempo de resposta, causa e reincidência.
- Acessibilidade WCAG 2.2 AA nas jornadas principais e orçamento operacional acompanhado.

Métricas de volume, relevância ou evidência jamais serão apresentadas como ranking de culpa.

## Premissas

- Uma única equipe editorial e uma instância escrevem no MVP.
- Conteúdo público pode ser lido sem conta; administração exige autenticação.
- Português do Brasil é o idioma inicial e UTC é armazenado, com exibição em `America/Fortaleza`.
- A planilha de 03/09/2026 é insumo a revisar, não fonte de verdade nem autorização de publicação.
- O caso deve ser uma entidade própria para permitir futuros recortes sem redesenhar o domínio.

## Riscos e decisões pendentes

Riscos centrais: dano reputacional, erro de identidade, perda de contexto, indisponibilidade/licença de fontes, dados pessoais, custo/instabilidade de LLM, SSRF e comprometimento do painel. Mitigações detalhadas constam nos documentos 03, 06 e 07.

Antes da publicação real, precisam de validação humana: responsável editorial e jurídico; política de correções/contato; base legal e períodos de retenção; licença para snapshots; identidade nominal do controlador; modelos e teto mensal do OpenRouter; provedor de e-mail/autenticação; domínio, RPO/RTO e retenção de backups.

