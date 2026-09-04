# Critérios de aceite

## Definition of Ready e Done

Uma história está pronta quando objetivo, dados, autorização, estados, erros e impacto editorial/privacidade estão claros. Está concluída quando código e migration são revisados, testes proporcionais passam, acessibilidade/segurança são verificadas, docs/runbook mudam, métricas existem e nenhuma fixture contém pessoa real.

## Aceite da fundação documental (Fase 0)

- Todos os arquivos solicitados existem e se referenciam sem contradições materiais.
- Diagramas Mermaid renderizam e mostram revisão humana, fonte por evidência e banco único.
- Evidência A–E, relevância 1–5 e confiança técnica estão separadas.
- Estados/transições e atores estão definidos; automação não alcança `approved`/`published`.
- Modelo inclui tabelas mínimas, ligações, constraints, índices, exclusão, slugs, dedupe e importação.
- ADRs têm contexto, decisão, alternativas, prós/contras, riscos, gatilhos e data.
- Inspeção da planilha informa estrutura/contagens sem transformar seus dados em fatos publicados.
- OpenRouter tem fonte oficial/data e não usa plugin web depreciado como decisão nova.
- Riscos e decisões pendentes estão explícitos; aplicação substancial não foi iniciada.

## Cenários críticos

1. **Importar:** dado XLSX válido, dry-run retorna mapeamento/151 linhas/avisos e zero registros editoriais; aplicação repetida não duplica; erro fatal faz rollback.
2. **Consultar:** leitor encontra nome/alias, filtra e abre detalhe em mobile/teclado/sem JS; somente `published` aparece.
3. **Abrir fontes:** cada alegação mostra papel, trecho/localizador e metadata; link indisponível é marcado, não removido.
4. **Monitorar:** duas execuções simultâneas resultam em uma lease; falha parcial retoma sem duplicar; orçamento interrompe com estado seguro.
5. **Revisar:** reviewer compara extraído/atual/origem, registra motivo; conflito de versão exige recarregar.
6. **Publicar:** serviço rejeita candidato não aprovado e ator sem papel; sucesso cria change log e invalida derivados.
7. **Exportar:** XLSX abre, contém sete áreas mínimas, totais conciliam com snapshot e hash publicado confere.
8. **Restaurar:** backup de DB em atividade é consistente, `integrity_check` passa e aplicação restaurada atende smoke test dentro do RTO.

## Gates editoriais por publicação

- Identidade e homônimos tratados; proposição limitada, datada e atribuída.
- Ao menos uma evidência ligada a fonte, excerto/localizador e papel.
- Grau justificado; relevância não influenciou o grau; confiança não foi tratada como verdade.
- Contraditório procurado e estado registrado; dados pessoais minimizados.
- Linguagem não presume culpa; situação jurídica e limites têm contexto.
- Revisão, versão e histórico registrados; item disputado segue política cautelar.

## Aceite técnico transversal

- Respostas públicas não contêm conteúdo administrativo, PII redigida, segredos ou stack trace.
- CSRF, XSS, SSRF, URL/upload, sessão e RBAC têm testes negativos.
- PRAGMAs e volume local são verificados; nenhuma chamada externa ocorre em transação longa.
- Logs estruturados correlacionam request/run sem texto sensível; alertas principais são exercitados.
- WCAG 2.2 AA automatizada + revisão manual de teclado/contraste; p95 comparado ao orçamento definido.
- Dependências estão fixadas, justificadas e licenciadas; artefato é reproduzível.

## Bloqueadores de go-live

Responsável editorial/jurídico, política pública de correção/contato, termos e privacidade, base legal/retenção, licença de snapshots, domínio, MFA/recuperação, teto de custos/modelos, RPO/RTO/retenção e restore real devem estar aprovados. Nenhum deles bloqueia o esqueleto técnico, mas bloqueiam publicação de dados reais.

