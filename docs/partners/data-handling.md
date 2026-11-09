# Tratamento de dados no piloto (resumo para o outreach)

Este é o resumo que aparece no primeiro contato (Story 7.1, AC2). O acordo assinado está em [`agreement.md`](agreement.md) (Story 7.2, AC2) e precisa refletir exatamente o que está aqui — se divergir, o acordo vence e este documento é corrigido antes do próximo convite. O teste `TestAgreementMatchesTheOutreachSummary` falha quando os dois deixam de concordar sobre retenção, duração ou cadência.

## Princípios

1. **O dado fica com o parceiro.** A stack é self-hosted: ClickHouse, VictoriaMetrics, NATS, PostgreSQL e os serviços Observaki rodam na infraestrutura do parceiro. Não existe telemetria subindo para nós.
2. **Sem acesso por padrão.** Suporte é feito por call, compartilhamento de tela e arquivos que o parceiro escolher enviar. Acesso remoto só se for oferecido por ele, com escopo e prazo declarados.
3. **Ambiente não crítico.** O piloto não cobre produção crítica; SLA reduzido, conforme o PRD (Fase 1).
4. **Minimização.** Pedimos apenas os sinais necessários para avaliar a stack, não dumps de banco nem dado de cliente final.

## Retenção

- Padrão do piloto: **7 dias** para logs e traces, ajustável por política de retenção.
- O parceiro pode reduzir ou aumentar dentro do suportado, e apagar dados derrubando os volumes.
- A decisão de retenção default do produto segue aberta em `DECISIONS.md` (P-02).

## Amostras e material público

- Amostras enviadas para diagnóstico são anonimizadas antes de sair do ambiente do parceiro quando contêm identificadores.
- Números publicados (eventos/s, latência p95, compressão, custo) são **agregados**; nenhum nome de empresa ou serviço aparece sem aprovação por escrito.
- Redação de campos sensíveis já acontece no gateway (chaves e padrões de cartão) antes da persistência.

## Saída

- Revogar o token de ingestão encerra a coleta.
- `docker compose down -v` (ou remoção dos PVCs no Helm) apaga os dados locais.
- Nada fica retido do nosso lado; não há cópia externa.
- O parceiro pode pedir encerramento a qualquer momento, sem aviso prévio e sem custo.

## O que não prometemos

- SLA de produção, GA ou suporte 24/7 na Fase 0.
- Certificações (SOC 2, HIPAA, FedRAMP) — fora do roadmap da Fase 1.
- Residência de dados gerenciada por nós: não somos um SaaS no piloto.
- Uso do nome do parceiro, logo ou case sem aprovação escrita.
