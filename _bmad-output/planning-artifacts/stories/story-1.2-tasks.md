# Story 1.2 — CI de qualidade e release alpha assinada

**Epic:** 1 — Fundação do repositório e governança
**Tamanho:** M
**Status:** in-progress
**PRD:** FR-53 (base), NFR-8, NFR-15; AR-8.
**Depende de:** Story 1.1 (`done`).

## Objetivo

Todo push/PR na `main` roda lint, testes, build, SAST, scan de dependências e
SBOM; toda tag `v*` produz binários e imagens multi-arquitetura assinados com
checksums e SBOM anexados. Falha crítica bloqueia o merge.

## Decisões desta story

- **CI:** GitHub Actions (`.github/workflows/`).
- **SAST:** CodeQL (Go e TypeScript) + `govulncheck`.
- **Dependências:** `pnpm audit --audit-level critical` e `cargo audit`.
- **SBOM:** Syft (`anchore/sbom-action`), CycloneDX JSON.
- **Assinatura:** cosign **keyless** via OIDC (`id-token: write`), sem chave no repo.
- **Imagem:** Dockerfile mínimo de runtime (`distroless/static`, nonroot),
  provisório até a Story 5.1 empacotar Compose/Helm de verdade.
- **Gate de merge:** branch protection configurada no GitHub (documentada em
  `docs/ci.md`); o repositório não consegue versionar essa configuração.

## Tarefas

### T1.2.1 — Workflow de qualidade (0,5 dia)

**Arquivos:** `.github/workflows/ci.yml`, `scripts/smoke.sh`.

- Job `quality` em push/PR/`workflow_dispatch`: Go 1.27, Node 26, pnpm 12,
  Rust stable, `make tools`, `make ci`, `make smoke`.
- `SMOKE_SKIP_CI=1` no smoke para não repetir o `make ci`.
- Upload do `web/build` como artefato.

**Concluído quando:** workflow válido (`actionlint`), jobs nomeados e com timeout.

### T1.2.2 — Segurança e SBOM (0,5 dia)

**Arquivos:** `.github/workflows/ci.yml` (job `security`), `.github/workflows/codeql.yml`.

- `govulncheck ./...`, `pnpm audit --audit-level critical`, `cargo audit`.
- SBOM CycloneDX com Syft, artefato `sbom`.
- CodeQL para `go` e `javascript-typescript` em push/PR/schedule.

**Concluído quando:** jobs rodam em PR e falham em achado crítico; SBOM publicado.

### T1.2.3 — Release alpha assinada (1 dia)

**Arquivos:** `.github/workflows/release.yml`, `Dockerfile`, `.dockerignore`.

- Tag `v*.*.*` dispara build de `observaki` (linux amd64/arm64) e
  `observaki-agent` (amd64 via cargo, arm64 via `cross`).
- Imagem multi-arch em `ghcr.io/<repo>:<tag>` com `buildx`, assinada por digest.
- SBOM por binário, `sha256sum`, bundles de assinatura cosign.
- GitHub Release com binários, SBOMs, checksums e bundles.

**Concluído quando:** workflow válido; imagem amd64 construída e executada
localmente; passos de assinatura revisados.

### T1.2.4 — Documentação do CI (0,25 dia)

**Arquivo:** `docs/ci.md`.

- Jobs, checks obrigatórios (branch protection), como reproduzir localmente,
  como verificar assinaturas (`cosign verify-blob`/`verify`), rotação de nada
  (keyless) e limitações.

### T1.2.5 — Validação e evidência (0,25 dia)

- `actionlint` nos workflows; `docker build` + `docker run --rm ... version`;
- `make ci` e `make smoke` verdes;
- registrar evidência no `STATUS.md` (não há runner GitHub local).

## Fora de escopo

- Licença/DCO/templates (Story 1.3), ADRs (1.4).
- Compose/Helm reais (5.1/5.2); a imagem aqui é só de release.
- Assinatura de pacotes de SO e SLSA nível 3 (avaliar na 1.2 futura/2.x).

## Riscos

| Risco | Mitigação |
|---|---|
| Ações de terceiros mudarem | versões major pinadas; ações oficiais quando possível |
| CodeQL não bloquear sozinho | branch protection + docs; govulncheck/audits falham no job |
| keyless exigir repo público/OIDC | documentado; fallback com chave gerenciada se necessário |
| build arm64 do agente | `cross` no runner Linux; testar na primeira tag |
| Sem runner local | evidência por `actionlint` + build local da imagem |
