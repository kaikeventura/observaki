---
description: Continua a Fase 0 na ordem, sem pular gates
---

Leia `AGENTS.md`, `STATUS.md`, `_bmad-output/planning-artifacts/epics.md` e `_bmad-output/planning-artifacts/PROGRAM-PLAN.md`.

Use `STATUS.md` como posição atual. Continue a Fase 0 na ordem das stories, uma por vez, a partir do `next_action`. Para cada story:
1. Confirme dependências e critérios de aceitação.
2. Implemente apenas a story atual.
3. Rode os checks aplicáveis.
4. Atualize o status e registre evidência.
5. Pare ao final da story e reporte.

Não avance para a Fase 1. Não pule a Story 1.1. Não altere o escopo do PRD. Se um gate falhar, pare e reporte o bloqueio com evidência.
