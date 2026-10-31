# Git Workflow — Observaki

**Remoto:** `git@github.com:kaikeventura/observaki.git` (SSH; HTTPS: `https://github.com/kaikeventura/observaki.git`)
**Branch principal:** `main`
**Regra:** tudo direto na `main` (preferência do dono: sem branch por story e sem PR); commits pequenos; nada de segredo; nada de `force-push` em `main`.

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

Não crie branch por story: o trabalho vai direto na `main` (preferência do
dono). Use branch apenas para um experimento descartável, com acordo explícito,
e nunca deixe de mergear de volta no fim.

**Antes de codar**, leia `STATUS.md`; a posição atual do trabalho está nele.

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

5. Push da `main` (sem branch de story e sem PR):

```bash
git push origin main
```

6. Registre a evidência no `STATUS.md`: comandos, resultados e screenshots
   quando houver UI. Sem PR, o session log e o commit são o registro.

7. Em outra máquina, atualize com `git pull --rebase origin main`.

---

## 5. Regras de push

- **Nunca** `git push --force` em `main`.
- **Nunca** commite segredo. Se acontecer, revogue a credencial e limpe o histórico antes de push.
- **Sempre** `git pull --rebase origin main` antes do push.
- Se o push falhar por auth, mantenha o commit local, registre o bloqueio em `STATUS.md` e peça ao usuário para autenticar.
- Se o push falhar por conflito, rebase; não faça merge de `main` dentro de branch sem necessidade.
- Se o repo remoto já tiver README/commit inicial, faça `git pull --rebase origin main` antes do primeiro push ou use `git push -u origin main` após reconciliar.

---

## 6. Branches

**Sem branches por story.** Todo trabalho (feature, fix, docs, benchmark, infra)
é commitado direto na `main`, conforme a preferência do dono. Branch só para
experimento descartável com acordo explícito, e o resultado volta para a `main`
no fim da sessão.

PR não faz parte do fluxo atual; a revisão é registrada no `STATUS.md`.

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
4. Commitar com Conventional Commits + `-s` na `main` e dar push.
5. Registrar o SHA no `STATUS.md`.
6. Parar e reportar.

Se não houver credencial de push, ele deve **não** tentar adivinhar; mantém o commit local, registra o bloqueio e pede autenticação.
