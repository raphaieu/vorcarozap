# Visão do produto

## Problema e proposta

As informações do caso estão fragmentadas em mídia, documentos, decisões judiciais, relatórios periciais, entrevistas, mensagens e redes sociais. O VorcaroZAP organiza, analisa e publica essas associações com fontes rastreáveis, datas, classificação editorial e métricas públicas.

A proposta central do produto é permitir ao leitor percorrer de ponta a ponta uma trilha documental clara e auditável:
**pessoas/organizações &rarr; alegações &rarr; evidências/trechos literais &rarr; fontes originais e contrapontos de defesa**, acompanhados de contexto rigoroso e métricas consolidadas por estado ativo.

O produto não decide culpa nem atua como tribunal de exceção. Ele responde objetivamente:
1. Por que um nome ou organização aparece relacionado;
2. Quem publicou ou produziu a informação de sustentação;
3. Qual a força probatória documental da alegação (Graus A a E) e o alcance institucional da entidade (1 a 5);
4. Quais são os limites contextuais e eventuais manifestações de contraditório;
5. Quando houve a última atualização ou verificação.

### Inspiração conceitual e navegação documental

Iniciativas externas como o [MasterWhats](https://masterwhats.recomendeme.com.br/) ([repositório](https://github.com/rafaelbressan/masterzap)) demonstram o valor de apresentar acervos investigativos complexos — a exemplo da transcrição manual e cenários do laudo IPJ-A nº 3298613/2026 ([documentação](https://github.com/rafaelbressan/masterzap/blob/main/data/ipj-3298613/README.md)) — sob uma interface navegável e localizada por página e figura. No VorcaroZAP, adota-se essa diretriz conceitual de rastreabilidade profunda, mantendo-se rigorosamente as premissas canônicas:

- **Recursos declarados vs. testados:** Trata-se qualquer capacidade documental descrita em projetos externos como funcionalidade declarada em documentação, e não como recurso testado ou integrado ao VorcaroZAP.
- **Licenciamento e conformidade:** Não foi identificada licença de software ou de redistribuição de dados na raiz do repositório de referência consultado (`masterzap`). É expressamente vedado qualquer reuso de código-fonte, dados brutos ou artefatos sem confirmação formal de permissão ou licença compatível.
- **Representação secundária:** Transcrições, visualizadores de mensagens ou resumos são sempre representações secundárias. O documento original, laudo oficial, página, figura e limitações metodológicas devem permanecer claramente distinguíveis pelo leitor.
- **Menção não é conluio nem culpa:** A mera citação de um nome em mensagem, anotação ou relatório policial não implica interlocução direta, anuência, vínculo substantivo ou juízo condenatório.
- **Proibição de fabricação ou elevação automática:** O sistema jamais preenche lacunas com dados inventados, não resolve homônimos ou identidades por mera semelhança e não eleva o grau documental (A–E) por inferência de modelos de linguagem.
- **Não independência da mesma fonte:** Múltiplos trechos, capturas ou notas derivados do mesmo laudo, documento ou veículo de imprensa constituem a mesma fonte primária e não configuram corroboração independente.

## Estrutura de Entregas (MVP e Evolução)

O ciclo de produto distingue rigorosamente três horizontes:

1. **Implementado:** Fundação Go, SQLite WAL, renderização SSR com Templ, importador da base `curated_seed` com mapping versionado, listagem, filtros multifacetados, busca, detalhes de entidades com fontes e defesas, metodologia pública e engine de métricas em tempo real na Home.
2. **Planejado no MVP:**
   - VZ-009: Exportação XLSX derivada diretamente do banco;
   - VZ-010 a VZ-013: Esteira de monitoramento autônomo via OpenRouter com gates estrutural e semântico, idempotência e limites de custo;
   - VZ-020: Verificador seguro HTTP GET para integridade e acessibilidade de fontes externas (dependência técnica dos gates de publicação automática);
   - VZ-014 a VZ-017 e VZ-021: Painel simples de moderação humana com autenticação básica, decisões atômicas por XOR (claim ou evidence_source), quarentena automática ao perder último suporte ativo e invalidação instantânea de visualizações;
   - VZ-018 e VZ-019: Deploy VPS econômico e suíte de testes table-driven obrigatórios.
3. **Evolução Proposta (Pós-MVP):**
   - VZ-022: Referenciação aprofundada de laudos e peças externas (uso de locators precisos de página e figura em `evidence_sources` sem inflar o schema do MVP);
   - VZ-023: Viewer SSR de documentos e sequências contextuais (avaliação de viabilidade técnica, respeitando limites de direitos, licenças e sem introduzir SPAs ou frameworks pesados).

## Fora do MVP

RBAC multiusuário, MFA, dupla revisão, CMS completo, snapshots integrais, cadeia de custódia forense, alertas sofisticados, HA, múltiplas instâncias, API pública e suíte ampla de testes.

## Métricas públicas essenciais

- pessoas/organizações ativas;
- fontes públicas ativas;
- alegações por grau A–E;
- entidades por categoria e relevância;
- registros adicionados/atualizados recentemente;
- data/hora do último monitoramento concluído.

Confiança técnica da IA não é exibida como probabilidade de verdade ou culpa.

Uma correção ou contexto pode permanecer público sem aumentar totais de contatos/vínculos. Métricas de rede exigem claim `published` e `metric_eligible = true`; métricas operacionais, como último monitoramento, seguem sua própria consulta.

## Premissas

- uma instância e um administrador no início;
- volume e tráfego baixos/moderados;
- fontes são públicas e registradas por URL/data;
- atualizações automáticas têm limites de custo;
- conteúdo desaprovado deixa imediatamente a página e os números públicos;
- português do Brasil, timestamps em UTC e exibição em `America/Fortaleza`.

## Riscos registrados

Erro de identidade, fontes circulares, perda de contexto, links indisponíveis, prompt injection, custo de LLM, contestação e dados pessoais. São tratados inicialmente por gates, quarentena, fonte visível, pós-moderação e canal de correção; controles avançados evoluem com uso real.
