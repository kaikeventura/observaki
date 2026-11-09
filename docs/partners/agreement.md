# Acordo de design partner (piloto Fase 0)

**Story 7.2, AC2.** Este é o texto base que o parceiro assina antes do onboarding. Ele espelha exatamente [`data-handling.md`](data-handling.md) — se os dois divergirem, o acordo vence e o resumo do outreach é corrigido antes do próximo convite. A versão assinada é um documento local (fora do repositório).

Preencha os campos entre `{ }` e envie em PDF ou e-mail de aceite. Nada aqui promete SLA, GA ou paridade com ferramenta comercial.

## 1. Escopo

**Parceiro:** {empresa}, CNPJ {cnpj}, representada por {nome} ({cargo}).

**Piloto:** Observaki alpha, plataforma de observabilidade open source (Apache 2.0), self-hosted e OTel-native, executada na infraestrutura do parceiro. Escopo: ingestão de métricas, logs e traces de **um ambiente não crítico**; dashboards, alertas e correlação; feedback sobre a experiência.

**Fora do escopo:** produção crítica, dados de cliente final, SSO/SCIM, certificações (SOC 2, HIPAA, FedRAMP), residência de dados gerenciada por nós e suporte 24/7.

## 2. Duração e compromisso

- **8 semanas** a partir da data de assinatura, alinhadas à Fase 0.
- **1 call de 30 min por semana**, com quem opera a stack no dia a dia (ou feedback assíncrono por escrito, se a semana não couber).
- Telemetria de um ambiente não crítico via OTel Collector, no volume que o parceiro escolher.
- O parceiro pode encerrar antes do fim, sem custo e sem aviso prévio (cláusula 7).

## 3. Tratamento de dados

O dado fica com o parceiro. A stack (ClickHouse, VictoriaMetrics, NATS, PostgreSQL e serviços Observaki) roda no cluster dele; **não existe telemetria subindo para nós**.

| Item | Regra |
|---|---|
| **Retenção** | Padrão de **7 dias** para logs e traces, ajustável pela política de retenção do parceiro dentro do suportado. Métricas seguem a retenção configurada no VictoriaMetrics. |
| **Acesso** | Nenhum acesso remoto por padrão. Suporte por call, compartilhamento de tela e arquivos que o parceiro escolher enviar. Acesso só se ele oferecer, com escopo e prazo declarados por escrito. |
| **Minimização** | Apenas os sinais necessários para avaliar a stack: sem dump de banco e sem dado de cliente final. |
| **Redação** | O gateway redige campos sensíveis (chaves e padrões de cartão) antes da persistência; o parceiro pode estender a lista de campos. |
| **Anonimização** | Amostras enviadas para diagnóstico saem do ambiente do parceiro já anonimizadas quando contêm identificadores. |
| **Material público** | Só números agregados (eventos/s, p95, compressão, custo). Nome, logo, serviço ou case exigem aprovação escrita, registrada neste acordo ou em e-mail posterior. |

## 4. Cadência de feedback

- **Semanal:** call de 30 min ou resumo escrito — o que funcionou, o que quebrou, o que faltou.
- **Bloqueios:** reportados pelo canal combinado e triados em **1 dia útil** (evidência da Story 7.3).
- **Registro:** cada ciclo gera uma nota curta (dor, impacto, sugestão) usada para priorizar a Fase 1; nenhuma nota é publicada com identificação sem aprovação.

## 5. Baseline do ambiente (AC3)

No início do onboarding, com o parceiro, registramos o retrato do ambiente atual conforme [`baseline.md`](baseline.md): ferramentas em uso, volume de eventos/s, custo mensal e as **3 principais dores**. O baseline fica local (`partners/baselines/{id}.md`, fora do git); só os números agregados entram no benchmark público.

## 6. Propriedade e licença

O núcleo é Apache 2.0: o parceiro pode usar, modificar e distribuir. Não há lock-in, não há licença paga no piloto e nenhuma contribuição do parceiro é obrigatória. Feedback e sugestões podem ser usados para melhorar o produto sem contrapartida financeira.

## 7. Saída

- **Revogar o token** de ingestão encerra a coleta imediatamente.
- **`docker compose down -v`** (ou remoção dos PVCs no Helm) apaga os dados locais.
- Nada fica retido do nosso lado: não há cópia externa para apagar.
- O encerramento pode ser pedido a qualquer momento, sem custo e sem prazo de aviso; encerramos o acesso, apagamos as notas de feedback identificáveis e confirmamos por escrito em até 5 dias úteis.

## 8. Garantias e limitações

Software alpha, fornecido **como está**, sem SLA de disponibilidade, sem garantia de adequação a produção crítica e sem suporte 24/7. Bugs e indisponibilidades são tratados como parte do piloto e viram item de backlog. Nenhuma das partes assume obrigação de compra futura.

## 9. Assinatura

| Campo | Parceiro | Observaki |
|---|---|---|
| Nome | {nome} | Kaike Ventura |
| Cargo | {cargo} | Fundador |
| Empresa | {empresa} | Observaki (projeto open source) |
| Data | {data} | {data} |
| Aceite | assinatura ou e-mail de aceite | — |

Ao assinar, o parceiro confirma que leu as cláusulas 1–8, em especial a retenção de 7 dias, a anonimização de amostras e a saída sem custo.
