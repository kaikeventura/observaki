---
description: Roda o benchmark e atualiza o relatório
---

Leia `AGENTS.md`, o addendum (`_bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/addendum.md`, §6) e `_bmad-output/planning-artifacts/epics.md` (Epic 2/3).

Rode o harness de benchmark existente com o workload de referência (60% métricas/30% logs/10% traces, ~134 B comprimidos) e os cenários aplicáveis (throughput, query, cardinalidade, caos). **Não altere código de produto** para “fazer o número passar”.

Atualize o relatório em `BENCHMARKS.md` (ou no caminho definido pelo harness) com:
- commit, hardware, versões e config;
- eventos/s, bytes/s, lag p50/p95/p99;
- CPU/RAM/disco, compressão;
- query p50/p95/p99, rows/bytes lidos;
- falhas, perdas e duplicatas;
- conclusão go/no-go.

Se o benchmark falhar, reporte o gargalo e a hipótese; não maquie o resultado.
