---
description: Continua exatamente do próximo passo registrado em STATUS.md
---

Leia `AGENTS.md`, `project-context.md` e `STATUS.md`.

Use `phase`, `epic`, `story`, `task` e `next_action` de `STATUS.md` como posição atual. **Não replaneje, não recalcule o roadmap e não pule gates.**

- Se `status: blocked`, leia `blocked_by`, explique o bloqueio e pare.
- Se `status: done`, avance para o próximo item do checklist, mas só se as dependências estiverem concluídas.
- Se estiver na branch `main`, crie a branch da story antes de editar (ex.: `feat/p0-s1.1-bootstrap`).
- Implemente apenas o `next_action` atual. Se ele for uma task de story, siga o task breakdown em `_bmad-output/planning-artifacts/stories/`.
- Rode os checks aplicáveis.
- Atualize `STATUS.md` (frontmatter + checklist + session log) e `epics.md` se a story mudou de status.
- Faça commit com Conventional Commits + DCO (`git commit -s`) e push da branch. Se o push falhar por auth, mantenha o commit local e reporte.
- Pare com um relatório curto: o que mudou, evidência, riscos, próximo passo.

Não comece a próxima story na mesma sessão.
