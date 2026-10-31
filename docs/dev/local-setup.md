# Setup local — Observaki

> **A preencher na T1.1.7.** Enquanto o scaffolding não estiver completo, este
> arquivo só registra o contrato: pré-requisitos, comandos `make`, portas e
> troubleshooting do desenvolvimento local.

## Pré-requisitos (alvo)

| Ferramenta | Versão mínima |
|---|---|
| Go | 1.27 (resolvido automaticamente pelo `go.mod`) |
| Node.js | 26 LTS |
| pnpm | 10 |
| Rust | 1.85 (stable, com `rustfmt` e `clippy`) |
| Docker Compose | v2 (opcional nesta fase) |
| make, curl | qualquer versão recente |

`scripts/check-toolchain.sh` valida as versões (Story 1.1, T1.1.5).

## Comandos

Os comandos definitivos entram no `Makefile` na T1.1.5:

```bash
make bootstrap   # valida a toolchain e instala dependências
make test        # go test + vitest + cargo test
make lint        # golangci-lint + svelte-check + clippy
make build       # binário Go + build do SvelteKit + agente
make dev         # Go (:8080) + SvelteKit (:5173)
make smoke       # valida o binário e o build do web
```

## Portas

| Serviço | Porta | Variável |
|---|---|---|
| API Go | 8080 | `OBSERVAKI_HTTP_ADDR` |
| SvelteKit dev | 5173 | `WEB_PORT` |

## Troubleshooting

A preencher na T1.1.7 (porta ocupada, versão de toolchain incorreta,
pnpm/cargo ausente).

## Validado por

| Data | Sistema | Autor | Resultado |
|---|---|---|---|
| — | — | — | pendente (T1.1.8) |
