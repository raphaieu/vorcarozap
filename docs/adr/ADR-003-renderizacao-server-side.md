# ADR-003 — Renderização server-side

- **Status:** aceito
- **Data:** 2026-09-03

## Contexto

O site é majoritariamente leitura/busca, precisa funcionar bem em celular, ser acessível/indexável e manter JavaScript e operação pequenos.

## Decisão

HTML server-side com `templ`, rotas `chi`, CSS próprio/utilitário leve e HTMX como melhoria progressiva. Links e formulários funcionam sem JavaScript; fragments e páginas completas usam o mesmo view model.

## Alternativas consideradas

- SPA React/Vue/Angular: interatividade rica, porém bundle, estado duplicado, API e testes adicionais.
- Templates `html/template`: menos dependência e boa segurança, mas componentes tipados/compostos são menos ergonômicos.
- Site totalmente estático: rápido, mas painel, busca e atualização dinâmica exigem contornos.

## Consequências positivas

Primeiro carregamento leve, acessibilidade/SEO naturais, menos superfície cliente e uma linguagem principal.

## Consequências negativas

Interações complexas exigem cuidado; navegações podem gerar mais requests; templ adiciona geração ao build.

## Riscos

Fragments divergirem, foco/histórico HTMX falharem ou JS virar requisito acidental. Testes sem JS e E2E críticos mitigam.

## Gatilhos de revisão

Fluxos comprovadamente semelhantes a desktop/offline, visualização interativa impossível de manter ou métricas mostrando UX insuficiente após otimização SSR.

