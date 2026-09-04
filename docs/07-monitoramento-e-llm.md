# Monitoramento, pesquisa e LLM

## Status

**Planejado para uma fase posterior ao primeiro lançamento.** Nenhuma integração com OpenRouter é necessária para colocar a página pública inicial no ar.

As primeiras atualizações serão produzidas manualmente e aplicadas por importação/CLI. Isso reduz custo de tokens e evita construir painel, fila e mecanismos de verificação antes de validar interesse no produto.

## Arquitetura futura preservada

```go
type ResearchProvider interface {
    Discover(ctx context.Context, input DiscoverInput) (DiscoverResult, error)
    Verify(ctx context.Context, input VerifyInput) (VerifyResult, error)
}
```

`OpenRouterProvider` será o primeiro adaptador, sem acoplamento do domínio. Modelos, URL e engine serão configuração.

## Referência técnica consultada em 03/09/2026

O plano utiliza structured outputs com JSON Schema em modelo compatível e a server tool beta `openrouter:web_search`. O plugin `web` e o sufixo `:online` estão depreciados. Revalidar a documentação oficial quando esta fase começar:

- [Web Search Server Tool](https://openrouter.ai/docs/guides/features/server-tools/web-search)
- [Structured Outputs](https://openrouter.ai/docs/guides/features/structured-outputs)
- [Quickstart](https://openrouter.ai/docs/quickstart)

## Pipeline futuro

Descoberta por janela → normalização → deduplicação → verificação dirigida → fontes/contraditório → candidato → revisão humana → publicação.

Antes de ativá-lo serão implementados apenas os controles necessários à funcionalidade: limite de custo, idempotência, validação de citações, armazenamento restrito do bruto e proteção contra prompt injection. Testes de contrato substituirão uma suíte ampla inicialmente.

