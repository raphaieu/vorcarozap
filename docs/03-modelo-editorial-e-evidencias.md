# Modelo editorial e evidências

## Unidades e dimensões

Uma **entidade** existe independentemente do caso. Uma **relação** conecta entidade a entidade ou ao caso. Uma **alegação** é uma proposição textual, limitada, atribuível e verificável. Uma **evidência** explica como uma ou mais fontes sustentam ou contestam uma alegação. Uma fonte não comprova nada sozinha: exige trecho/localizador, contexto e papel (`supports`, `contradicts`, `contextualizes`).

Três dimensões nunca se fundem:

1. **Grau A–E:** força do suporte público à alegação.
2. **Confiança técnica 0–100:** confiança do pipeline na extração/identidade/deduplicação; não é verdade, culpa nem probabilidade jurídica.
3. **Relevância 1–5:** alcance público, institucional, econômico ou político da entidade; não mede vínculo ou evidência.

## Escala de evidências

| Grau | Definição operacional | Requisito mínimo | Exibição obrigatória |
|---|---|---|---|
| A | Evidência primária ou confirmação oficial | documento/registro institucional, decisão ou manifestação direta verificável e contextualizada | natureza, data, emissor e limites |
| B | Evidência jornalística forte | investigação fundamentada ou confirmações independentes confiáveis | veículos, independência e eventuais divergências |
| C | Associação documentada, mas incompleta | contato/relação demonstrado; significado ou consequência não confirmado | dizer expressamente o que não está demonstrado |
| D | Alegação atribuída | terceiro identificado ou matéria baseada em fonte não identificada, sem confirmação independente suficiente | atribuição proeminente e contraditório |
| E | Pista não confirmada | menção indireta, coincidência ou sinal que requer apuração | rótulo de pista e proibição de conclusão |

O grau pertence à alegação/relação contextual, não à pessoa. Promoção ou rebaixamento exige revisão, justificativa e nova versão. Quantidade de fontes não converte automaticamente D em B; independência, qualidade e proximidade ao fato importam.

## Relevância

1 local/circunstancial; 2 setorial/regional; 3 nacional moderada; 4 alta nacional; 5 nacional estratégica/internacional. Registrar justificativa factual e data. Não usar relevância para ordenar por padrão nem somá-la ao grau.

## Linguagem e publicação

Preferir “foi citado por”, “aparece em”, “segundo [fonte]”, “a investigação sustenta”, “a defesa afirmou”, “não foi localizada confirmação independente” e “permanece classificada como pista”. Termos como culpado, criminoso ou integrante de esquema só cabem em citação/contexto de decisão juridicamente adequado, com situação processual e revisão editorial/jurídica.

Todo cartão/detalhe deve declarar: alegação precisa; autor da alegação quando aplicável; grau por texto/ícone/cor; fontes e excerto/localizador; limites; contraditório; estado e revisão. Contato social/profissional, presença em agenda/evento ou ausência de resposta nunca é descrito como prova de irregularidade.

## Fluxo e checklist

Antes de aprovar: resolver identidade sem inferência; limitar a proposição; verificar data/contexto; preferir primária; testar independência da segunda fonte; procurar contraditório; minimizar dados; avaliar interesse público/dano; conferir grau; revisar título e resumo; registrar responsável e motivo. Alegações graves ou grau D/E sobre pessoa identificável exigem revisão reforçada e podem permanecer apenas no painel.

Correções criam nova versão e nota pública quando materiais. Contestação move o item a `disputed`, sem apagamento automático; conteúdo pode ficar oculto cautelarmente por decisão registrada. Pedidos usam canal publicado, SLA interno e preservação de evidências.

## Fonte e snapshot

Tipos controlados: documento oficial, decisão judicial, manifestação institucional, reportagem, entrevista, mensagem/captura, rede social, audiovisual, secundária e não verificada. Registrar URL canônica e arquivada, título, autoria/publicador, publicação/acesso, disponibilidade, hash e observação. Snapshots somente quando lícitos e necessários; guardar metadados e hash mesmo quando o conteúdo não puder ser redistribuído.

Mensagens/capturas exigem procedência, cadeia de publicação, contexto, autenticidade conhecida/desconhecida e redação de telefone/endereço/documento. Marcar `public`, `leaked`, `court_record`, `user_submitted`; existência não autoriza republicação.

## Planilha de referência

Inspeção de 03/09/2026: quatro abas (`Resumo`, `Pessoas A-Z`, `Método e fontes`, `Leitura rápida`); 151 linhas na tabela principal, 13 colunas, todas com fonte principal, 44 com fonte adicional e nenhum nome exato duplicado. A escala legada é relacional (“direto confirmado”, “agenda apenas” etc.) e **não equivale** à nova escala epistemológica A–E. Importar valor bruto e mapear para campos estruturados somente após revisão linha a linha; nenhum registro será publicado pela importação.

