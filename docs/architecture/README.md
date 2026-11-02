# Arquitetura — diagramas

Diagramas de arquitetura do Observaki, renderizados a partir da spine binding
(`_bmad-output/planning-artifacts/architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md`).
**Isto é renderização, não decisão:** nenhum AD novo é criado aqui. Onde um diagrama
parece contradizer a spine, o AD vence e a divergência está listada abaixo.

| Arquivo | O que é |
|---|---|
| `observaki-architecture.drawio` | Fonte editável — 6 páginas em um único arquivo |
| `diagrams/*.png` | Export para leitura rápida e revisão em PR (escala 1,5×) |

## Como abrir

- **draw.io desktop** ou [app.diagrams.net](https://app.diagrams.net) → *Open* → `observaki-architecture.drawio`.
- **VS Code**: extensão *Draw.io Integration* (abre o `.drawio` direto no editor).
- As páginas aparecem como abas na parte inferior. Cada uma é independente.

## Páginas

| # | Página | Pergunta que responde | ADs que ela codifica |
|---|---|---|---|
| 1 | Contexto (C4 L1) | Quem usa, o que entra e o que sai do produto | AD-1, AD-4, AD-16, AD-24, AD-35 |
| 2 | Containers (C4 L2) | Quais processos existem e como os dois planos se falam | AD-1, AD-5, AD-6, AD-7, AD-8, AD-18, AD-19, AD-27, AD-32 |
| 3 | Deploy AWS multi-AZ | Onde cada coisa roda na AWS, com HA por AZ | AD-5, AD-7, AD-12, AD-13, AD-24, AD-25, AD-31, AD-35 |
| 4 | Ingestão (pipeline) | Em que ordem os filtros rodam e onde o ack é permitido | AD-2, AD-3, AD-4, AD-13, AD-15, AD-16, AD-18, AD-19, AD-21, AD-26, AD-33 |
| 5 | Consulta (read path) | Por que UI/alertas nunca falam com ClickHouse/VM | AD-3, AD-5, AD-6, AD-17, AD-23, AD-32 |
| 6 | Control plane (ERD) | Qual é o modelo de dados do system of record | AD-2, AD-7, AD-19, AD-20, AD-28, AD-30, AD-31, AD-35 |

Cada página carrega o número do AD no próprio rótulo do nó ou da aresta, para que a
revisão de uma mudança de diagrama seja também uma revisão do invariante.

## Convenções visuais

| Cor | Significado |
|---|---|
| Azul | Control plane (API, UI, auth, alerting, reconciliação) |
| Verde | Data plane (gateway, writers, query) |
| Roxo (cilindro) | Estado durável (PostgreSQL, ClickHouse, VictoriaMetrics, NATS) |
| Laranja | Serviço gerenciado da AWS (hospedagem, não stack) |
| Cinza | Ator ou sistema externo |
| Vermelho | Fronteira de durabilidade, segurança ou proibição explícita |

Aresta sólida = síncrona · tracejada = assíncrona/eventual · tracejada vermelha =
fronteira de durabilidade ou de autorização.

## Deploy na AWS: o que é decisão e o que é referência

A AWS entra como **hospedagem**, não como substituta da stack. As escolhas de
armazenamento e mensageria continuam sendo as dos ADs:

| Componente | Onde roda | Por quê |
|---|---|---|
| Gateway, writers, query, API, alerting, promscrape | EKS (node groups por AZ) | Stateless, escala horizontal, mesmas imagens do single-node (AD-12) |
| ClickHouse, VictoriaMetrics, NATS JetStream | EKS + EBS gp3, uma réplica por AZ | AD-5 e AD-18 fixam esses componentes; não há equivalente gerenciado com o mesmo contrato |
| PostgreSQL | Amazon RDS for PostgreSQL 18.6, Multi-AZ | Mesmo engine e mesmo schema lógico (AD-7); o que muda é só a HA |
| Auditoria e backup | S3 com Object Lock (WORM) | AD-35 exige export contínuo imutável |
| Segredos e chaves | Secrets Manager + KMS | AD-13, AD-25, AD-31 (sem criptografia caseira) |
| Meta-observabilidade | CloudWatch + Amazon Managed Service for Prometheus | AD-24 exige caminho e store separados do pipeline de clientes |
| Ingress | NLB (OTLP) e ALB + WAF (UI/API), Route 53, ACM | Borda TLS; o gateway continua sendo o ponto de autenticação de tenant |

**O que este desenho deliberadamente não decide:**

- Trocar ClickHouse/VictoriaMetrics/NATS por MSK, Timestream, OpenSearch ou similares.
  Isso contradiz AD-5 e AD-18 (semântica de ack/dedupe, PromQL/MetricsQL, retenção por
  `ingested_at`) e exigiria **AD-36+ e um novo benchmark**, não uma edição de diagrama.
- Multi-região, federação, cold storage exato e retenção por tenant de métricas — todos
  estão em `Deferred` na spine.
- Região, tamanho de nó e fator de replicação exatos: dependem do baseline T1 (P-07) e
  do run de 1M/s por 1 h.

## Divergências encontradas na spine

1. **`PANEL` no ERD da spine × AD-28.** O ERD da spine tem `DASHBOARD ||--o{ PANEL`;
   AD-28 diz explicitamente "dashboard definition JSONB sem entidade `PANEL` separada".
   A página 6 segue o AD (dashboard com `definition : jsonb`) e marca a divergência no
   próprio nó. A spine não foi editada: corrigir o artefato binding é uma decisão sua.
2. **Roteamento de canal de alerta.** A spine liga `ALERT_RULE` direto a
   `NOTIFICATION_CHANNEL`. A página 6 usa a tabela de junção `alert_rule_channel`, que é
   o que AD-28 ("canais normalizados por projeto/org") implica para muitos-para-muitos.
   É refinamento de modelagem, não conflito — mas vale alinhar o texto da spine.

## Regenerar os exports

Requer o draw.io desktop com CLI (`drawio` no PATH).

```bash
i=0
for n in 1-contexto 2-containers 3-deploy-aws 4-ingestao 5-consulta 6-control-plane; do
  i=$((i+1))
  drawio -x -f png -s 1.5 --page-index "$i" \
    -o "docs/architecture/diagrams/$n.png" \
    docs/architecture/observaki-architecture.drawio
done
```

Para SVG (vetor, melhor para documentação publicada), troque `-f png -s 1.5` por `-f svg`.

## Manutenção

- Mudou um AD? Atualize a página correspondente e o rótulo do AD no nó.
- Novo componente? Ele precisa de um AD que o governe antes de aparecer no desenho.
- Não adicione serviço gerenciado que troque a stack sem AD novo.
