---
description: Encerra a sessão atual e deixa o handoff em STATUS.md
---

Não comece trabalho novo. Apenas consolide a sessão atual.

1. Leia `STATUS.md`, `epics.md` e a story/task atual.
2. Verifique o que foi realmente concluído e qual é a evidência (comando, teste, commit/PR, benchmark).
3. Atualize o frontmatter de `STATUS.md`:
   - `task`, `status`, `updated`, `next_action`, `blocked_by`, `last_session`;
   - se a story terminou, avance para a próxima story pronta.
4. Atualize a linha da story na tabela da Fase 0.
5. Adicione uma linha no **Session log** com: data, sessão, fase/story/task, resultado, commit/PR, próximo.
6. Se houver mudanças, faça commit com Conventional Commits + DCO (`git commit -s`) e push da branch; registre o SHA em `last_commit`.
7. Se houver bloqueio, registre a pergunta exata em `blocked_by` e pare.
8. Responda com um handoff curto: o que foi feito, evidência, o que não foi feito, próximo passo.

Não marque `done` sem evidência. Não apague histórico. Não implemente a próxima story.
