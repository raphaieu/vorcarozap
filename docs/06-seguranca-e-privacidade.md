# Segurança, privacidade e responsabilidade

Este documento é orientação de engenharia/editorial, não parecer jurídico. Antes do lançamento, responsável editorial e assessoria competente devem validar LGPD, direitos de personalidade, segredo de justiça, direito autoral, termos de fontes e resposta/correção.

## Modelo de ameaças

Ativos: painel, credenciais, banco, snapshots, material bruto, trilha, backups e reputação das pessoas. Ameaças: tomada de conta, CSRF/XSS/SQLi, SSRF e downloads maliciosos, enumeração, upload bomba/XXE, vazamento em logs/backup, prompt injection, adulteração de histórico, publicação indevida e erro de identidade.

## Controles do painel

- Senhas com Argon2id em parâmetros versionados (ou provedor externo validado), MFA para editores/admins e proteção contra credential stuffing.
- Sessão aleatória de alta entropia; somente hash no banco; rotação no login/elevação; expiração absoluta/inativa e revogação.
- Cookie `HttpOnly; Secure; SameSite=Lax` (Strict quando fluxo permitir), prefixo `__Host-`, sem domínio e path `/`.
- CSRF token ligado à sessão em toda mutação; reautenticação para usuário/papel, publicação sensível e credenciais.
- RBAC no caso de uso, não apenas no handler; deny by default e trilha para login, leitura sensível, revisão, export e configuração.
- Rate limit por rota/identidade/IP com limites confiáveis atrás do proxy; bloqueio progressivo e alerta sem permitir DoS de conta.

## Aplicação e transporte

Caddy força HTTPS/HSTS após validação; headers: CSP restritiva com nonce/hash, `frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy`, `Permissions-Policy` e COOP quando compatível. Templates escapam por padrão; Markdown/HTML permitido passa por sanitizador allowlist. Queries parametrizadas, validação de tamanho/tipo e mensagens de erro sem detalhes internos.

URLs aceitam apenas `http/https`, normalizam IDN, rejeitam credenciais e portas/esquemas não previstos. Coletor resolve DNS e bloqueia loopback, link-local, redes privadas, metadata/cloud, IPv6 especial e redirects que mudem para destino proibido; revalida a cada redirect e conecta ao IP validado. Resposta tem timeout, limite de bytes, MIME permitido e não executa conteúdo.

Uploads: assinatura mágica e extensão allowlist (`.xlsx` inicial), tamanho/quantidade limitados, nome gerado, diretório sem execução e parsing ZIP protegido contra zip bomb, path traversal, macros e entidades externas. Antivírus é decisão de implantação conforme uploads de terceiros.

## LLM como fronteira não confiável

Conteúdo web é dado, nunca instrução. O provider tem egress e ferramentas mínimos, schema estrito, limites de busca, domínios e tamanho; saída é validada novamente. Não enviar segredos, conteúdo privado desnecessário ou PII integral. Resposta bruta é acesso restrito, criptografada/retida por prazo e nunca renderizada como HTML. Toda publicação requer humano.

## Privacidade e LGPD

Manter inventário de dados/finalidades, controlador/operador e bases legais a validar. Aplicar necessidade, qualidade, transparência, segurança, prevenção e prestação de contas. Oferecer canal para acesso, correção, contestação e resposta; autenticar pedidos proporcionalmente sem coletar excesso.

Telefones, documentos, endereços, contas e dados familiares são redigidos salvo interesse público claro, necessidade e revisão reforçada. Distinguir material público, vazado, judicializado e enviado por usuário. Ausência de defesa é registrada como estado de busca, nunca como admissão. Dados de sessão e logs têm retenção curta; conteúdo rejeitado mantém fingerprint/fundamento mínimo para impedir republicação, com acesso restrito.

## Segredos, backups e incidentes

Segredos vêm do ambiente/secret store, nunca de Git, logs ou DB editorial; rotação e revogação documentadas. Backups são consistentes, criptografados antes do envio, com chave separada, acesso mínimo, retenção e restore testado. Logs têm redação e acesso controlado.

Plano de incidente define classificação, contenção, preservação, rotação, avaliação de titulares/ANPD quando aplicável, comunicação e post-mortem. Runbooks cobrem conta comprometida, publicação indevida, vazamento, fonte maliciosa, perda/corrupção do DB e chave OpenRouter exposta.

## Risco editorial/jurídico

Mitigações: atribuição e contexto próximos; revisão humana; limiar maior para alegações graves; contraditório proeminente; status processual e temporal; política de correções; snapshots com direitos avaliados; não indexar rascunhos; revisão periódica de conteúdo antigo. O risco não é eliminado por disclaimer.

