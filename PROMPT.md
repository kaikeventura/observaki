Leia AGENTS.md, project-context.md e STATUS.md.

Continue exatamente do next_action em STATUS.md.
Não replaneje, não recalcule o roadmap, não pule gates e não invente escopo.

Se STATUS.md estiver blocked, explique o bloqueio e pare.
Se a task atual não estiver clara, pare e pergunte.
Se estiver na branch main, crie a branch da story antes de editar (ex.: feat/p0-s1.1-bootstrap).

Implemente apenas o next_action atual. Rode os checks aplicáveis (make fmt, make lint, make test, make build, make smoke ou equivalentes).
Ao terminar, atualize STATUS.md (frontmatter + checklist da Fase 0 + session log), faça commit com Conventional Commits + DCO (`git commit -s`) e push da branch. Se o push falhar por autenticação, mantenha o commit local e reporte.
Pare e me mostre um resumo curto: o que mudou, evidência, riscos e próximo passo.
