# Contribuindo com o Observaki

Obrigado pelo interesse em contribuir. O Observaki está na **Fase 0**
(fundação e benchmarks), então feedback sobre escopo, benchmarks e experiência
de operação é tão valioso quanto código.

Ao participar, você concorda em seguir o [Código de Conduta](CODE_OF_CONDUCT.md).
**Não abra issue pública para vulnerabilidades** — use o processo do
[SECURITY.md](SECURITY.md).

## Formas de contribuir

- **Bugs:** reproduza com a versão e o ambiente, usando o template de bug.
- **Features:** descreva o problema antes da solução, usando o template de feature.
- **Documentação:** correções, exemplos, guias de instrumentação e tradução.
- **Benchmarks:** workloads, cenários e análise de resultados reproduzíveis.
- **Design partners:** dados reais e feedback semanal (Fase 0, Epic 7).
- **Código:** comece por issues marcadas como `good first issue`.

## Antes de começar

1. Leia o [README](README.md), o [guia de setup local](docs/dev/local-setup.md)
   e o [AGENTS.md](AGENTS.md).
2. Procure uma issue existente antes de abrir outra.
3. Mudanças pequenas (typo, teste, documentação) podem ir direto para um PR.
4. Mudanças grandes — novo sinal, storage, API pública, tenancy, retenção,
   licenciamento ou dependência relevante — precisam de uma issue/RFC e de
   acordo dos mantenedores **antes** do código. O [PRD](_bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/prd.md)
   define non-goals vinculantes: não invente escopo.
5. Toda mudança deve apontar para uma issue, FR, story ou AD. Requisitos de
   arquitetura estão em [ARCHITECTURE-SPINE.md](_bmad-output/planning-artifacts/architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md).

## Ambiente local

Pré-requisitos e troubleshooting: [docs/dev/local-setup.md](docs/dev/local-setup.md).

```bash
make bootstrap   # valida toolchain e instala dependências
make ci          # bootstrap + lint + test + build
make smoke       # valida o binário e o build web
```

O CI roda os mesmos alvos, além de análise de segurança, CodeQL e DCO.

## Fluxo de pull request

1. Faça um fork e crie uma branch curta a partir de `main`.
2. Faça commits pequenos, em inglês, no formato
   [Conventional Commits](https://www.conventionalcommits.org/) e **sempre com
   sign-off** (veja [DCO](#developer-certificate-of-origin-dco)).
3. Atualize sua branch com `git rebase origin/main`; evite merges de `main`
   dentro da branch.
4. Abra o PR preenchendo o template, com evidência de teste e links para a issue.
5. Espere o CI ficar verde: `quality`, `security`, `CodeQL` e `DCO sign-off`.
6. O mantenedor pode pedir mudanças, rebase ou squash. Não faça force-push
   depois que o review começou sem avisar.

Exemplos de mensagem:

```text
feat(ingest): add OTLP gRPC receiver
fix(clickhouse): dedupe retried event_id
test(bench): add 1M/s workload generator
docs(governance): document open-core boundary
chore(ci): add DCO sign-off check
```

Regras:

- Um commit por unidade lógica; não misture refactor com feature.
- Não commite `node_modules/`, `target/`, `bin/`, `web/build/`, `.env`, logs,
  dados de benchmark ou credenciais.
- Mudanças no caminho crítico exigem benchmark antes/depois; regressão é bug P1.
- UI segue [DESIGN.md](_bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/DESIGN.md)
  e [EXPERIENCE.md](_bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/EXPERIENCE.md):
  WCAG 2.2 AA, teclado, foco visível, estado nunca só por cor.
- Testes de isolamento de tenancy são obrigatórios quando a mudança toca
  `tenant_id`/`project_id`.

## Developer Certificate of Origin (DCO)

O projeto usa **DCO, não CLA**: você mantém o copyright da sua contribuição e
certifica o direito de submetê-la sob a licença do projeto.

Todo commit precisa de uma linha de sign-off com o seu nome e e-mail:

```text
Signed-off-by: Seu Nome <voce@example.com>
```

O jeito mais simples é:

```bash
git commit -s -m "feat(scope): describe the change"
```

O e-mail do sign-off deve corresponder ao autor **ou** ao committer do commit.
Se o CI acusar falta de sign-off:

```bash
# assinar todos os commits da sua branch
git rebase --signoff origin/main

# ou corrigir apenas o último commit
git commit --amend --signoff --no-edit
```

Depois, atualize a branch com `git push --force-with-lease`. Commits feitos pela
interface web do GitHub não recebem sign-off automaticamente; use a CLI ou
inclua o trailer manualmente na mensagem.

O check `DCO sign-off` falha com a lista de commits e o comando de correção.
O texto completo do DCO está em <https://developercertificate.org/>.

## Processo de RFC

Mudanças grandes seguem um RFC curto antes da implementação. Abra uma issue
usando o template de feature com:

- problema e usuário afetado;
- proposta e alternativas consideradas;
- impacto em tenancy, storage, API pública, retenção, performance e licença;
- plano de migração e rollback, quando aplicável.

Os mantenedores podem pedir que o RFC vire um documento em `docs/rfcs/`. Um RFC
aceito não substitui um AD: decisões de arquitetura são registradas em
[`docs/adr/`](docs/adr/) conforme a Story 1.4.

## Revisão e expectativas

- Revisão é feita por mantenedores, em regime de melhor esforço; não há SLA
  antes do GA.
- PRs pequenos e com teste são revisados mais rápido.
- Um PR pode ser fechado se sair do escopo da fase, conflitar com um AD ou ficar
  sem resposta por tempo prolongado.
- O fluxo interno do projeto é *main-only* (sem branch por story). Contribuições
  externas continuam entrando por PR.

## Licença e open-core

O núcleo é licenciado sob [Apache 2.0](LICENSE). Contribuições são recebidas
sob a mesma licença (inbound = outbound), sem CLA.

O modelo open-core e a fronteira entre OSS e enterprise estão descritos em
[docs/governance/open-core.md](docs/governance/open-core.md). A lista de
candidatos a enterprise é uma **decisão de negócio pendente** (P-03); ela não
cria compromisso e nenhum caminho de telemetria será paywalled.

## Segurança

Não abra issue pública. Siga o [SECURITY.md](SECURITY.md) para reportar
vulnerabilidades de forma privada e coordenada.

## Comunidade

Por enquanto, o canal oficial é o [issue tracker](https://github.com/kaikeventura/observaki/issues).
Canais de comunidade (chat, office hours) serão anunciados no README quando
existirem. Seja direto, respeitoso e assuma boa-fé.
