# Política de segurança

## Versões suportadas

O Observaki está em pré-1.0 (Fase 0). Apenas a `main` e a release alpha mais
recente recebem correções de segurança; não há backport de alphas anteriores nem
SLA formal antes do GA.

| Versão | Suporte |
|---|---|
| `main` | Correções de segurança |
| Última release `v0.0.x-alpha` | Correções de segurança |
| Releases alpha anteriores | Sem suporte |

## Como reportar uma vulnerabilidade

**Não abra issue pública, discussão ou pull request para uma vulnerabilidade.**

Use o canal privado do GitHub:

1. Abra <https://github.com/kaikeventura/observaki/security/advisories/new>.
2. Descreva o impacto e o caminho de reprodução.
3. Se o canal não estiver disponível, envie e-mail para
   **kaike.gnr@hotmail.com** com o assunto `[SECURITY] Observaki`.

Inclua, quando possível:

- versão, commit ou imagem afetada;
- componente (gateway, query, storage, UI, agente, CI/release);
- pré-condições e passos de reprodução;
- impacto (confidencialidade, integridade, disponibilidade, tenancy);
- prova de conceito mínima, sem dados reais de terceiros;
- se a falha já é pública e onde.

**Nunca envie segredos reais, tokens, dados de clientes ou PII.** Use dados
sintéticos e remova credenciais dos logs.

## O que esperar

- **Confirmação de recebimento:** meta de 3 dias úteis.
- **Triagem inicial:** meta de 7 dias úteis, com severidade e próximo passo.
- **Divulgação coordenada:** meta de até 90 dias após a confirmação, ou antes se
  houver correção e acordo entre as partes. Prazos são metas de melhor esforço
  durante a Fase 0, não um SLA.
- **Crédito:** damos crédito público a quem reportar, se desejado; você pode
  permanecer anônimo.
- **Bug bounty:** não há programa de recompensa financeira nesta fase.

## Escopo

Dentro do escopo:

- código-fonte deste repositório e artefatos de release (binários, imagens,
  SBOMs, checksums, assinaturas);
- bypass de autenticação, autorização, isolamento de tenancy ou auditoria;
- exposição de segredos, tokens ou dados entre `tenant_id`/`project_id`;
- injeção, SSRF, path traversal e execução remota de código;
- falhas na cadeia de suprimentos (build, dependências, publicação de release);
- configurações inseguras nos manifests de `deploy/`.

Fora do escopo:

- vulnerabilidades em dependências de terceiros sem impacto demonstrável neste
  projeto (reporte ao upstream e nos avise);
- resultados de scanners sem prova de impacto ou exploit;
- negação de serviço contra infraestrutura de demonstração;
- engenharia social, acesso físico ou ataques a contas pessoais;
- más práticas sem impacto de segurança demonstrável.

## Divulgação e safe harbor

Pedimos que você não divulgue a falha publicamente antes de uma correção ou de
um prazo acordado. Em contrapartida, não tomaremos medidas legais contra
pesquisa de boa-fé que:

- respeite esta política e a privacidade de terceiros;
- use apenas contas e dados que você controla;
- evite degradar o serviço, acessar dados de outros clientes ou persistir
  acesso;
- reporte a falha de forma privada e dê tempo razoável para correção.

Esta política não autoriza testes em ambientes de terceiros nem em produção de
clientes sem permissão explícita.

## Chaves e canais

Não há chave PGP publicada nesta fase. Prefira o GitHub Security Advisories, que
mantém o relato privado entre você e os mantenedores. O e-mail é apenas fallback.
