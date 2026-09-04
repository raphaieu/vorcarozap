# Modelo editorial e evidências

## Princípio

O VorcaroZAP organiza associações já publicadas. Uma pessoa estar na base não significa culpa, participação em irregularidade ou proximidade pessoal com Daniel Vorcaro.

Uma **entidade** é uma pessoa ou organização. Uma **relação** descreve o contexto em que aparece. Uma **alegação** registra uma proposição atribuível. Uma **fonte** permite ao leitor conferir de onde veio a informação.

## Dimensões independentes

1. **Evidência A–E:** força do suporte público à alegação.
2. **Relevância 1–5:** alcance público, institucional, econômico ou político da pessoa.
3. **Confiança técnica:** futuramente, confiança do pipeline em extração/deduplicação; nunca mede culpa ou verdade.

O grau pertence à alegação. Relevância pertence à entidade. Confiança técnica pertence à execução automática/candidato.

## Escala A–E

| Grau | Significado | Como exibir |
|---|---|---|
| A | documento, decisão, registro institucional ou manifestação direta verificável | explicar natureza, data e limites |
| B | reportagem fundamentada ou confirmações jornalísticas independentes | identificar veículos e contexto |
| C | associação/contato documentado, com significado ou consequência incompletos | dizer claramente o que não está demonstrado |
| D | alegação atribuída sem confirmação independente suficiente | destacar quem alegou e apresentar fonte |
| E | pista, menção indireta ou coincidência que exige apuração | rotular como pista e não produzir conclusão |

Quantidade de links não fortalece automaticamente o grau; vários veículos podem reproduzir a mesma origem.

## Regra de publicação do MVP

O MVP não terá workflow multiusuário. A revisão acontece manualmente antes da importação.

- A–C podem ser publicados com síntese fiel, fonte e limites.
- D pode ser publicado como alegação atribuída, nunca como fato confirmado.
- E pode ser publicado somente quando a própria menção/associação estiver documentada por fonte pública e o texto não inferir crime, benefício ou intenção.
- Item sem fonte pública identificável não entra na página.
- Alegações graves que ultrapassem o que a fonte demonstra permanecem fora do texto público.

Essa regra permite mostrar “onde há fumaça” sem vender fumaça como incêndio confirmado.

## Relevância

- 1: local ou circunstancial;
- 2: setorial ou regional;
- 3: relevância nacional moderada;
- 4: alta relevância nacional;
- 5: relevância nacional estratégica ou internacional.

Relevância não altera o grau e não representa intensidade da relação.

## Linguagem

Preferir: “foi citado por”, “aparece em”, “segundo a fonte”, “a reportagem afirma”, “a defesa declarou”, “não foi localizada confirmação independente” e “permanece como pista”.

Contato social/profissional, presença em agenda/evento ou ausência de resposta nunca é apresentado como prova de irregularidade.

## Fonte mínima

Registrar, quando disponível: título, veículo/autor, URL, data de publicação, data de acesso, tipo e trecho/localizador. No MVP, não é obrigatório armazenar cópia integral ou snapshot. Preservação, hash, licença, dependência entre fontes e snapshots permanecem evolução futura.

Mensagens e capturas exigem origem pública identificada e contexto. Telefones, documentos, endereços e dados pessoais sem interesse para a associação devem ser ocultados.

## Correções

O lançamento deve oferecer um canal de correção. Alterações iniciais podem ser registradas pelo histórico Git, data da importação e versão da planilha. Workflow imutável e histórico público detalhado entram com o painel editorial.

## Planilha inicial

O arquivo de 03/09/2026 possui quatro abas (`Resumo`, `Pessoas A-Z`, `Método e fontes`, `Leitura rápida`), 151 linhas na tabela principal e fontes registradas. Ele está disponível no repositório público como artefato de pesquisa, mas suas linhas não são automaticamente aprovadas para exibição na aplicação. A escala legada deve ser preservada e não convertida silenciosamente para A–E.

