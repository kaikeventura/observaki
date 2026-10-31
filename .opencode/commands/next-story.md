---
description: Implementa apenas a próxima story pronta da Fase 0
---

Leia `AGENTS.md`, `STATUS.md`, `_bmad-output/planning-artifacts/epics.md` e `_bmad-output/planning-artifacts/PROGRAM-PLAN.md`.

Use `STATUS.md` como posição atual. Se houver uma story/task em andamento, continue dela. Se não, encontre a primeira story da Fase 0 cujo status ainda não é `done` e cujas dependências estão concluídas. Implemente **apenas** essa story.

Regras:
- Se a story tiver um task breakdown em `_bmad-output/planning-artifacts/stories/`, siga-o.
- Se não tiver, crie o breakdown antes de codar.
- Respeite os ADs relevantes de `ARCHITECTURE-SPINE.md` e o UX de `DESIGN.md`/`EXPERIENCE.md`.
- Não implemente nada de fases posteriores.
- Rode os checks aplicáveis (`make fmt`, `make lint`, `make test`, `make build`, `make smoke`) ou os comandos equivalentes da story.
- Atualize a documentação/memlog se uma decisão mudou.
- Atualize `STATUS.md` e faça commit com Conventional Commits + DCO (`git commit -s`) e push da branch. Se o push falhar, mantenha o commit local e reporte.
- Ao terminar, atualize o status da story no backlog e pare com um relatório: o que mudou, comandos/resultados, riscos e próximo passo.

Se a story não estiver pronta, ambígua ou conflitar com um AD, pare e pergunte antes de editar código.
