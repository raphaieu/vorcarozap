# Critérios de aceite

## Fase 0

- escopo do primeiro lançamento está separado das evoluções;
- segurança e testes futuros estão documentados sem bloquear o MVP;
- Go, SQLite, SSR, importação e deploy possuem decisões claras;
- OpenRouter está explicitamente fora do primeiro lançamento;
- classificação editorial e fontes continuam obrigatórias.

## Definition of Done do MVP

Uma entrega está pronta quando compila, cumpre a jornada manual relacionada, não expõe segredo/erro interno e atualiza a documentação quando alterar comportamento.

Não há meta de cobertura automatizada nesta fase.

## Gate técnico

```bash
go fmt ./...
go vet ./...
go build ./...
```

Migrations devem funcionar em banco vazio. Importação deve ser testada primeiro com `--dry-run`.

## Smoke test manual

1. subir ambiente limpo;
2. criar/migrar SQLite;
3. executar dry-run da planilha;
4. importar e repetir sem duplicar;
5. abrir home em viewport mobile;
6. pesquisar e filtrar;
7. abrir pessoa, alegação e fonte;
8. baixar XLSX;
9. reiniciar containers e confirmar persistência;
10. confirmar que não existe endpoint público de escrita.

## Gate editorial mínimo

- nome e homônimos conferidos;
- síntese não presume culpa;
- grau e relevância não são confundidos;
- associação possui ao menos uma fonte pública;
- título/veículo/URL/data disponíveis são exibidos;
- pista ou alegação é identificada como tal;
- dados pessoais desnecessários não são exibidos;
- contato para correção está disponível.

## Não bloqueiam o primeiro lançamento

Painel, MFA/RBAC, dupla revisão, OpenRouter, scheduler, snapshots, logs/métricas avançados, testes unitários abrangentes, integração, E2E, WCAG formal, backup externo criptografado, RPO/RTO formal e parecer jurídico completo.

Esses itens permanecem no backlog e tornam-se obrigatórios quando a funcionalidade associada for criada ou o projeto demonstrar tráfego, colaboração ou risco que justifique o custo.

