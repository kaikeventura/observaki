## Resumo

<!-- O que muda e por quê, em 1-3 frases. -->

## Issue relacionada

<!-- Ex.: Closes #123. Toda mudança deve apontar para issue, FR, story ou AD. -->

## Tipo de mudança

- [ ] Correção de bug
- [ ] Nova funcionalidade
- [ ] Documentação
- [ ] Refactor ou performance
- [ ] Build, CI ou release
- [ ] Governança ou licença

## Como foi testado

<!-- Comandos, testes adicionados, benchmark antes/depois, evidência. -->

```text
make ci
make smoke
```

## Checklist

- [ ] Commits em Conventional Commits e com `Signed-off-by` (`git commit -s`).
- [ ] `make ci` passa (ou explico abaixo por que não).
- [ ] Adicionei/atualizei testes para o comportamento novo.
- [ ] Atualizei a documentação afetada.
- [ ] Não incluí segredos, tokens, dados de clientes ou PII.
- [ ] UI: teclado, foco visível, WCAG 2.2 AA e estado nunca só por cor.
- [ ] Caminho crítico: benchmark antes/depois anexado.
- [ ] Tenancy: testes de isolamento por `tenant_id`/`project_id` quando aplicável.

## Notas para o review

<!-- Riscos, migração, rollback, decisões em aberto. -->
