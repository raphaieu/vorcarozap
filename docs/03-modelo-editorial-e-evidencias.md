# Modelo editorial e evidências

## Unidades

Entidade é pessoa/organização. Relação fornece contexto. Alegação é uma proposição atribuível. Evidência descreve o suporte. Fonte identifica a publicação verificável.

## Dimensões

- **Grau A–E:** força pública da alegação; pertence à alegação.
- **Relevância 1–5:** alcance da entidade; não mede culpa ou proximidade.
- **Confiança técnica:** qualidade da extração/deduplicação; pertence ao candidato/run e não aparece como verdade pública.

## Escala

| Grau | Significado | Publicação automática |
|---|---|---|
| A | documento/registro/manifestação direta contextualizada | sim, se gates passarem |
| B | reportagem fundamentada ou confirmação independente | sim, se gates passarem |
| C | associação documentada com significado incompleto | sim, explicitando limites |
| D | alegação atribuída sem confirmação independente | sim somente com atribuição explícita e fonte pública |
| E | pista/menção indireta | sim somente se a menção estiver documentada e sem inferir ilícito |

Sem URL pública, entidade inequívoca, linguagem compatível, trecho/localizador suficiente ou data mínima, o candidato vai para quarentena.

## Estados e visibilidade

```text
quarantined  -> não público; aguarda administrador
published    -> público; entra nas métricas
rejected     -> não público; decisão evita republicação automática
archived     -> não público; preservado para histórico
```

Somente `published` aparece em páginas, busca, métricas e XLSX. Entidade sem claim/relação publicada não aparece por associação vazia. Moderar claim/fonte recalcula a visibilidade derivada.

## Gates automáticos mínimos

- URL `http/https` válida;
- título/veículo ou emissor;
- entidade resolvida sem ambiguidade relevante;
- alegação limitada ao conteúdo citado;
- trecho/localizador suficiente;
- grau válido e linguagem obrigatória para C/D/E;
- ausência de telefone/documento/endereço desnecessário;
- fingerprint não rejeitado anteriormente;
- validação local do schema e das citações retornadas.

Falha em gate gera quarentena, não descarte.

## Relevância

1 local/circunstancial; 2 setorial/regional; 3 nacional moderada; 4 alta nacional; 5 estratégica/internacional. Justificativa factual obrigatória.

## Fontes

Registrar título, publicador/autor, URL canônica, publicação/acesso, tipo, trecho/localizador e papel (`supports`, `contradicts`, `contextualizes`). Múltiplos veículos que derivam da mesma origem não contam automaticamente como confirmações independentes.

Snapshots, cadeia de custódia e licença avançada ficam P1. Canal de correção e linguagem neutra entram no MVP.

