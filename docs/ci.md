# CI e release — Observaki

Fluxo no GitHub Actions. Tudo roda na `main` (o projeto não usa PRs por story),
mas os workflows também respondem a `pull_request` para contribuições externas.

## Workflows

| Arquivo | Quando roda | Jobs |
|---|---|---|
| `.github/workflows/ci.yml` | push na `main`, PR, manual | `quality` (lint, test, build, smoke) e `security` (govulncheck, audits, SBOM) |
| `.github/workflows/codeql.yml` | push na `main`, PR, semanal, manual | `Analyze go` e `Analyze javascript-typescript` |
| `.github/workflows/release.yml` | tag `v*.*.*` | `binaries`, `image`, `release` |

### `quality`

Toolchain: Go 1.27.2, Node 26, pnpm 12.16.1, Rust stable (rustfmt/clippy) e
golangci-lint instalado por `make tools`. Sequência:

```bash
make ci      # bootstrap + lint + test + build
make smoke   # com SMOKE_SKIP_CI=1, só valida o binário e o web/build
```

O `web/build` fica disponível como artefato `web-build`.

### `security`

- `govulncheck ./...` (Go) — falha quando há vulnerabilidade conhecida no código.
- `pnpm audit --audit-level critical` e `cargo audit` — falham em achado crítico.
- Syft gera `observaki-sbom.cdx.json` (CycloneDX) e publica o artefato `observaki-sbom`.

### `CodeQL`

SAST para Go e JavaScript/TypeScript em push, PR e semanalmente. Os resultados
aparecem em **Security → Code scanning**. Para bloquear o merge, marque o check
`Analyze <linguagem>` como obrigatório na branch protection.

## Branch protection (necessário para o gate)

A configuração não é versionável no repositório; faça uma vez em
**Settings → Branches → Add branch ruleset** para `main`:

1. Exija status checks: `Lint, test, build and smoke`, `Security and SBOM`,
   `Analyze go`, `Analyze javascript-typescript`.
2. Bloqueie force-push e exigir resolução de conversas (se houver PRs).
3. (Opcional) Exija o GitHub Actions como único caminho de merge.

Sem esse passo os jobs rodam, mas não bloqueiam o merge (AC1 da Story 1.2).

## Rodando localmente

Os mesmos comandos do CI, sem GitHub:

```bash
./scripts/check-toolchain.sh
make ci
make smoke
# equivalentes pontuais
"$(go env GOPATH)/bin/govulncheck" ./...
pnpm --dir web audit --audit-level critical
(cd agent && cargo audit)
```

SBOM local (requer Syft):

```bash
syft dir:. -o cyclonedx-json > observaki-sbom.cdx.json
```

## Release alpha

```bash
git tag -s v0.0.1-alpha -m "Observaki Phase 0 alpha"
git push origin v0.0.1-alpha
```

A tag dispara `release.yml`, que publica:

- `observaki-linux-amd64` e `observaki-linux-arm64`;
- `observaki-agent-linux-amd64` e `observaki-agent-linux-arm64`;
- `*.cdx.json` (SBOM por binário) e `SHA256SUMS`;
- bundles de assinatura `*.sigstore.json` (cosign keyless);
- imagem multi-arch `ghcr.io/kaikeventura/observaki:<tag>` e `:latest`, assinada
  por digest.

### Verificando uma release

```bash
# binário
cosign verify-blob \
  --bundle observaki-linux-amd64.sigstore.json \
  --certificate-identity-regexp '^https://github.com/kaikeventura/observaki/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  observaki-linux-amd64

# checksums
sha256sum -c SHA256SUMS

# imagem (assinada por digest no workflow)
cosign verify \
  --certificate-identity-regexp '^https://github.com/kaikeventura/observaki/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/kaikeventura/observaki@<digest>
```

## Limitações conhecidas

- **Keyless** exige repositório público e OIDC do GitHub; se o repositório virar
  privado, trocar para chave gerenciada (`cosign generate-key-pair` + secret).
- O build arm64 do agente usa `cross` e foi exercitado com sucesso na
  `v0.0.1-alpha`.
- CodeQL não falha o job por si só; o bloqueio depende da branch protection.
- Sem chave GPG no ambiente, a tag foi anotada (`git tag -a`) e não assinada;
  os artefatos são assinados pelo cosign.
- Não há runner GitHub local: a validação de PR é feita por `actionlint`, pelos
  testes locais e pela execução na `main`.

## Estado

- `v0.0.1-alpha` (2026-10-31): CI e CodeQL verdes; release com 4 binários,
  SBOMs, `SHA256SUMS`, bundles cosign e imagem multi-arch assinada.
