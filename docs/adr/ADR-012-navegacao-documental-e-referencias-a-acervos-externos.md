# ADR-012: Navegação documental e referências a acervos externos

## Status

Proposta

## Contexto

O VorcaroZAP organiza informações públicas, jornalísticas e periciais relativas ao caso Daniel Vorcaro e Banco Master sob uma rígida cadeia de custódia editorial: pessoas/organizações &rarr; alegações &rarr; evidências/trechos &rarr; fontes citadas e contrapontos.

Projetos independentes de terceiros, como o [MasterWhats](https://masterwhats.recomendeme.com.br/) ([repositório](https://github.com/rafaelbressan/masterzap)), propõem visualizações interativas no estilo mensageiro instantâneo a partir de vazamentos e de relatórios periciais da Polícia Federal, com destaque para a extração manual de mensagens em imagens do relatório IPJ-A nº 3298613/2026 ([documentação](https://github.com/rafaelbressan/masterzap/blob/main/data/ipj-3298613/README.md)).

Surgiu a oportunidade de avaliar como o VorcaroZAP deve absorver a riqueza documental dessas fontes sem desviar do foco econômico do MVP, sem violar as diretrizes de integridade editorial e sem incorrer em riscos jurídicos de conformidade.

Foram identificados os seguintes fatores condicionantes:
1. **Ausência de licença:** A consulta à raiz do repositório público `rafaelbressan/masterzap` não identificou licença explícita de código aberto (ex: MIT, Apache, GPL) ou cessão de uso dos arquivos de dados gerados.
2. **Natureza declarada dos recursos:** As funcionalidades e volumes descritos no README de projetos de terceiros devem ser tratados como declarações de seus autores, e não como capacidades técnicas validadas no ambiente do VorcaroZAP.
3. **Fronteira arquitetural:** O VorcaroZAP é um monólito Go com SSR (Templ) e SQLite. Não utiliza SPA, Vite, Node em produção ou banco de dados documental separado.
4. **Distinção probatória:** Transcrições de mensagens e laudos policiais constituem evidências que devem ser submetidas à mesma taxonomia rigorosa de graus (A–E) e disposições editoriais, sem presunções de culpa.

## Decisão proposta

1. **Integração inicial estritamente por referências documentais e links precisos:**
   A conexão do VorcaroZAP com acervos documentais e laudos periciais (incluindo o IPJ-A nº 3298613/2026 e reportagens associadas) dar-se-á inicialmente pela referenciação canônica de fontes (`sources`), citações literais textuais (`evidence_sources.excerpt`) e localizadores pontuais (`evidence_sources.locator`), tais como `Pág. 42, Fig. 12` ou URL pública do documento oficial.
2. **Aproveitamento das estruturas existentes sem alteração de schema no MVP:**
   O schema relacional consolidado no MVP (`00002_editorial_core.sql` e a visão `public_claims_view`) já contempla suporte completo a `sources`, `evidence_sources`, `locator`, `excerpt` e `role` (`supports`, `contradicts`, `contextualizes`). Não haverá criação de novas tabelas, alteração de schema ou novas migrações de banco de dados neste ciclo.
3. **Diferimento de importador dedicado de mensagens e de visualizador tipo chat:**
   O desenvolvimento de um importador específico de mensagens estruturadas (ex: conversas JSON/JSONL) e de um componente SSR para reproduzir interfaces de conversação (viewer) fica expressamente relegado a incrementos posteriores de maturidade (Fase 7 / pós-MVP), condicionado à validação de demanda e viabilidade técnica.
4. **Natureza secundária das transcrições e visualizadores:**
   Toda transcrição textual, leitura óptica ou visualização reproduzida em tela é classificada como representação secundária. O VorcaroZAP manterá visível e distinguível o documento oficial de origem, o número do laudo/processo, a página/figura original e as eventuais limitações de legibilidade relatadas pelos peritos ou curadores.
5. **Menção não implica interlocução, anuência ou culpa:**
   A presença de um nome em anotação, lista de contatos, captura de tela ou relatório policial indica exclusivamente uma menção documental. Não autoriza inferir conluio, interlocução bilateral direta, concordância ou responsabilidade penal/civil.
6. **Vedação à fabricação de conteúdo e elevação automática:**
   É expressamente proibido a modelos de linguagem ou curadores preencher lacunas de mensagens omitidas com conteúdo hipotético, inferir identidades de pessoas por homônimos sem confirmação documental ou elevar o grau probatório (A–E) sem evidência formal correspondente.
7. **Origem da informação vs. veículo publicador na corroboração:**
   A avaliação de independência probatória considera a origem documental e investigativa da informação, e não apenas o veículo que a publicou. Múltiplos veículos que reproduzem a mesma peça, laudo pericial ou vazamento derivam de uma fonte primária comum e não configuram corroboração independente para fins de elevação de grau probatório; reciprocamente, apurações com origens documentais distintas mantêm sua independência mesmo quando veiculadas pelo mesmo órgão de imprensa.
8. **Conformidade de direitos autorais e licenças:**
   Nenhum código-fonte ou arquivo de dados de repositórios terceiros será incorporado à base de código do VorcaroZAP sem confirmação formal de licença compatível ou autorização expressa de seus titulares.
9. **Manutenção estrita da stack e do cronograma:**
   Nenhuma dependência externa, serviço em tempo real ou biblioteca SPA será adicionada. O próximo passo funcional imediato permanece inalterado: VZ-009 (Exportação XLSX).

## Alternativas consideradas

- **Incorporação direta do frontend e base do MasterWhats:** Rejeitada por violar a ausência de licença identificada, introduzir dependência de Node/Vite no runtime e desviar recursos do núcleo editorial do MVP.
- **Criação imediata de tabelas relacionais para mensagens (`messages`, `chats`):** Rejeitada por introduzir complexidade precoce de schema antes do término das entregas prioritárias do MVP (exportação, monitoramento e painel de moderação). O campo `locator` existente atende perfeitamente à necessidade de localização pontual.
- **Omissão total de referências a conversas e laudos:** Rejeitada por subtrair da sociedade o acesso contextualizado a documentos públicos de grande repercussão que já integram o debate público.

## Consequências

### Positivas
- Preserva a integridade e o cronograma do MVP focado em publicação controlada, métricas e moderação.
- Garante conformidade jurídica e ética ao respeitar direitos autorais e ausência de licença no código de terceiros.
- Mantém o princípio jornalístico fundamental de diferenciar documento primário de transcrições intermediárias.
- Viabiliza a referenciação cirúrgica de peças periciais através do schema relacional atual (`locator` e `excerpt`).

### Negativas e Limitações
- Não entrega, neste ciclo, uma interface de navegação fluida em balões de diálogo estilo mensageiro instantâneo.
- O leitor dependerá de links para laudos em PDF e trechos literais em cartões documentais padrão.

## Limites e dependências

- Esta proposta não altera o roadmap imediato nem autoriza o início de tarefas pós-MVP antes da conclusão de VZ-009 a VZ-021.
- Eventual implementação futura de visualizador SSR (VZ-023) dependerá de aprovação formal de design, validação prévia de licença e manutenção da arquitetura SSR pura em Go + Templ.
