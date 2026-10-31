# Git Workflow — Observaki

**Remoto:** `git@github.com:kaikeventura/observaki.git` (SSH; HTTPS: `https://github.com/kaikeventura/observaki.git`)
**Branch principal:** `main`
**Regra:** uma story por branch/PR; commits pequenos; nada de segredo; nada de `force-push` em `main`.

---

## 1. Primeira subida (se o repo local ainda não existir)

```bash
cd ~/Github/observaki

# 1. Inicializa o repositório local
git init

# 2. Confere o que vai entrar
git status --short

# 3. Adiciona tudo (o .gitignore já protege build/secrets)
git add -A

# 4. Primeiro commit com DCO
git commit -s -m "chore: initialize Observaki planning workspace"

# 5. Branch principal
git branch -M main

# 6. Remoto (SSH; use HTTPS se preferir)
git remote add origin git@github.com:kaikeventura/observaki.git
# Alternativa HTTPS:
# git remote add origin https://github.com/kaikeventura/observaki.git
# Se já existir:
# git remote set-url origin git@github.com:kaikeventura/observaki.git

# 7. Envia
git push -u origin main
```

Se o GitHub pedir autenticação, use um Personal Access Token (fine-grained, escopo `contents: write`) ou o GitHub CLI/SSH configurado na máquina. **Nunca cole o token no chat, em arquivo ou no commit.**

---

## 2. Início de cada sessão

```bash
cd ~/Github/observaki
git status --short --branch
git pull --rebase origin main
```

Se for começar uma story nova:

```bash
git switch -c feat/p0-s1.1-bootstrap
# padrão: <tipo>/<fase>-<story>-<slug>
```

Tipos: `feat`, `fix`, `chore`, `docs`, `test`, `refactor`, `perf`, `ci`.

**Antes de codar**, leia `STATUS.md`. A branch deve refletir a story atual.

---

## 3. Durante o desenvolvimento

Commits pequenos, um por unidade lógica:

```bash
git add <arquivos>
git commit -s -m "feat(ingest): add OTLP HTTP receiver"
```

**Conventional Commits + DCO (`-s`).** Exemplos:

```text
feat(ingest): add OTLP gRPC receiver
feat(query): add MetricsQL query endpoint
fix(clickhouse): dedupe retried event_id
test(bench): add 1M/s workload generator
docs(adr): record ClickHouse + VictoriaMetrics storage
chore(ci): add SBOM and signed images
perf(query): cache 24h dashboard queries
```

Regras:
- Não commite `node_modules/`, `target/`, `bin/`, `web/build/`, `.env`, dados de benchmark.
- Não commite logs, dumps ou credenciais.
- Não misture refactor com feature no mesmo commit.
- Se o commit for grande demais, quebre por task (T1.1.1, T1.1.2...).

---

## 4. Fim de cada sessão

1. Rode os checks:

```bash
make fmt
make lint
make test
make build
make smoke      # quando existir
```

2. Atualize `STATUS.md` com:
   - task/story concluída;
   - evidência (comandos, teste, benchmark);
   - `next_action`;
   - bloqueios;
   - linha no session log.

3. Commit do código + status:

```bash
git add -A
git commit -s -m "feat(p0-s1.1): bootstrap monorepo and dev toolchain"
```

4. Capture o SHA e registre em `STATUS.md`:

```bash
git rev-parse --short HEAD
```

Atualize `last_commit` no frontmatter e no session log. Faça um commit pequeno de status:

```bash
git add STATUS.md
git commit -s -m "docs(status): record p0-s1.1 session"
```

5. Merge da branch na `main` (preferência do dono: a `main` é sempre o destino final da sessão) e push:

```bash
git switch main
git pull --rebase origin main
git merge --no-ff --no-commit <branch-da-story>
git commit -s -m "merge: <fase>-<story> <resumo>"
git push origin main
```

6. Abra o PR (opcional quando o merge é local direto; GitHub web, já que `gh` não está instalado):

```text
https://github.com/kaikeventura/observaki/compare/main...<branch>
```

O PR deve conter:
- story/task e link para `epics.md`;
- o que mudou;
- comandos/resultados de teste;
- riscos e próximo passo;
- screenshots se UI.

7. Depois do merge, atualize `main`:

```bash
git switch main
git pull --rebase origin main
```

---

## 5. Regras de push

- **Nunca** `git push --force` em `main`.
- **Nunca** commite segredo. Se acontecer, revogue a credencial e limpe o histórico antes de push.
- **Sempre** `git pull --rebase origin main` antes de abrir PR.
- Se o push falhar por auth, mantenha o commit local, registre o bloqueio em `STATUS.md` e peça ao usuário para autenticar.
- Se o push falhar por conflito, rebase; não faça merge de `main` dentro da branch sem necessidade.
- Se o repo remoto já tiver README/commit inicial, faça `git pull --rebase origin main` antes do primeiro push ou use `git push -u origin main` após reconciliar.

---

## 6. Branches e PRs

| Tipo | Exemplo |
|---|---|
| Feature | `feat/p0-s1.1-bootstrap` |
| Correção | `fix/p0-s3.3-dedupe-event-id` |
| Documentação | `docs/adr-storage-decision` |
| Benchmark | `bench/p0-1m-throughput` |
| Infra | `chore/ci-release-alpha` |

**Um PR = uma story.** Se a story for grande, quebre em PRs por task, mas mantenha a branch da story.

---

## 7. Tags e releases

- `v0.1.0-alpha` — fim da Fase 0.
- `v0.5.0-beta` — fim da Fase 1.
- `v0.8.0-beta` — fim da Fase 2.
- `v1.0.0` — GA (fim da Fase 3).

```bash
git tag -s v0.1.0-alpha -m "Observaki Phase 0 alpha"
git push origin v0.1.0-alpha
```

---

## 8. Recuperação

```bash
# Desfazer alterações não commitadas de um arquivo
git restore <arquivo>

# Desfazer o último commit mantendo as mudanças
git reset --soft HEAD~1

# Ver o que foi alterado
git diff
git diff --staged

# Ver histórico curto
git log --oneline --decorate -10
```

Nunca use `git reset --hard` sem confirmar que não há trabalho não commitado.

---

## 9. O que o OpenCode deve fazer

No fim de cada sessão, o agente deve:

1. Rodar os checks.
2. Atualizar `STATUS.md`.
3. Commitar com Conventional Commits + `-s`.
4. Fazer merge da branch na `main` (`--no-ff`, com `-s`) e push da `main`.
5. Registrar o SHA/PR no `STATUS.md`.
6. Parar e reportar.

Se não houver credencial de push, ele deve **não** tentar adivinhar; mantém o commit local, registra o bloqueio e pede autenticação.
