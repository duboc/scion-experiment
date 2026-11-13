# Handoff de Sessão (`jetski-cli` -> Jetski Hub) — `psearch` Wave 6 & `lead-cost-controller`

**Data:** 2026-11-11  
**Repositório:** `duboc/scion-experiment` (branch `main`, commit base deste handoff: `2ff8fd1`)  
**Objetivo deste arquivo:** Centralizar todas as decisões, entregas, estado operacional da VM do Hub e próximos passos dentro do próprio repositório para retomar o trabalho no **Jetski Hub** sem depender do histórico local do `jetski-cli`.

---

## 1. Resumo Executivo

Durante esta sessão foram concluídas duas frentes:

1. **Wave 6 (Backlog & Knowledge Plane):** Criação dos ADRs `ADR-012` a `ADR-016`, atualização do contrato OpenAPI v2.6.0 e abertura das issues `#39`–`#44` (subsequentemente entregues e fechadas junto com `#45`–`#54`).
2. **Diagnóstico e Mitigação de Custo do `claude-opus-5-5`:**
   - **Causa raiz identificada:** o heartbeat de 15 minutos de `psearch--lead` (96 execuções/dia) acordava uma conversa sem reset (`~755k–964k` tokens) após expiração do cache de prompt de 5 minutos (`ephemeral_5m`), gerando um `cache_write` completo (`1.25x`) a cada wake (~98% do gasto em Opus; `737M` `cache_write` + `842M` `cache_read` no `lead` vs. `3.1M`/`57.6M` no `deployer` em 13 dias).
   - **Mitigação implementada (Stage 1 em produção na VM do Hub):** controller autônomo [`lead-controller.py`](../scripts/lead-controller.py) rodando via cron a cada 5 min com `--apply-compact` (compacta `lead` e `deployer` acima de `200,000` tokens) e activity gate em shadow dry-run (`would_wake`) aguardando validação final para Stage 2 (`--apply-wake`).

---

## 2. Decisões Tomadas (e Racional)

| Tema | Decisão | Racional |
| --- | --- | --- |
| **Modelo dos agentes** | Manter `claude-opus-5-5` em `lead` e `deployer` | Qualidade de orquestração/deploy preservada; o problema era frequência de wake + tamanho de contexto/cache, não capacidade do modelo. |
| **Arquitetura do controller (Option S4)** | Script isolado [`lead-controller.py`](../scripts/lead-controller.py), separado de [`factory-watchdog.py`](../scripts/factory-watchdog.py) | Mantém escopos limpos (watchdog = saúde de infra; cost controller = compaction + activity gate). Não alterar `factory-watchdog.py` nem templates de agentes. |
| **Compactação de contexto** | `/compact` automático quando `context_tokens > 200,000` (`lead` e `deployer`) | Reduz custo por chamada em ~30× no `lead` (`764,580 -> 2,326` tokens observado no Step 0) sem perder estado essencial. |
| **Regras de wake (R1–R3, R5)** | Só acordar `lead` quando houver demanda real (issues abertas, menções/inbox PO, PRs/CI ou mudança de estado de agentes), com **backoff R5: 1h -> 4h** quando ocioso | Corta wakes ociosos mantendo responsividade quando há trabalho ativo. |
| **Regra R4 (PO reminder)** | **Descartada** (não implementada) | Evita ruído operacional e complexidade desnecessária nesta etapa. |
| **Safety-net heartbeat (Stage 2)** | Reduzir schedule `psearch-lead-heartbeat` de `15m` para `4h` (`0 */4 * * *`) ao ativar `--apply-wake` | Garante rede de segurança periódica mesmo se o gate ficar quieto, com impacto mínimo de custo. |
| **Heartbeat pausado** | `_next_heartbeat()` retorna `NO_UPCOMING_HEARTBEAT` quando `status != "active"` | Evita leituras falsas de `nextRunAt` congelado no passado quando o schedule está `paused`. |

Referência completa de design e critérios de aceite: [`docs/scion/specs/2026-11-06-lead-cost-controller-design.md`](../specs/2026-11-06-lead-cost-controller-design.md).

---

## 3. Arquivos Criados/Modificados e Histórico de Commits

### Wave 6 & Knowledge Plane (`d926319` -> `575b3fc`)
- [`knowledge-plane/adr/ADR-012-scann-ann-and-pagination.md`](../../../knowledge-plane/adr/ADR-012-scann-ann-and-pagination.md)
- [`knowledge-plane/adr/ADR-013-autocomplete-suggestions.md`](../../../knowledge-plane/adr/ADR-013-autocomplete-suggestions.md)
- [`knowledge-plane/adr/ADR-014-typo-tolerant-fuzzy-fallback.md`](../../../knowledge-plane/adr/ADR-014-typo-tolerant-fuzzy-fallback.md)
- [`knowledge-plane/adr/ADR-015-image-search-embedding-adapter.md`](../../../knowledge-plane/adr/ADR-015-image-search-embedding-adapter.md)
- [`knowledge-plane/adr/ADR-016-similar-items-and-substitutes.md`](../../../knowledge-plane/adr/ADR-016-similar-items-and-substitutes.md)
- [`knowledge-plane/INDEX.md`](../../../knowledge-plane/INDEX.md)
- [`knowledge-plane/contracts/biggy-search-openapi.yaml`](../../../knowledge-plane/contracts/biggy-search-openapi.yaml) (v2.6.0)
- Commit Wave 6 docs: `d926319` (issues `#39`–`#54` já fechadas até `575b3fc`).

### Lead Cost Controller (`295401d` .. `2ff8fd1`, já em `origin/main`)
- [`docs/scion/specs/2026-11-06-lead-cost-controller-design.md`](../specs/2026-11-06-lead-cost-controller-design.md) (`295401d`) — Spec aprovada (Opções S0–S6, regras R1–R3/R5, teto 200k, rollout §6, métricas §7).
- [`docs/plans/2026-11-09-lead-cost-controller.md`](../../plans/2026-11-09-lead-cost-controller.md) (`622842e`) — Plano TDD de 8 tarefas (Tasks 1–7 concluídas; **Task 8 pendente**).
- [`docs/scion/scripts/lead-controller.py`](../scripts/lead-controller.py) — Controller CLI (`--apply-compact`, `--apply-wake`, `--install-cron`, `flock`, checagem de `last_event_at` para deployer, `NO_UPCOMING_HEARTBEAT`).
- [`docs/scion/scripts/tests/test_lead_controller.py`](../scripts/tests/test_lead_controller.py) — Suíte unitária com **58 testes** (`python3 -m unittest discover -s docs/scion/scripts/tests` passando em `0.068s` no `main`).
- [`docs/scion/scripts/query-token-metrics.py`](../scripts/query-token-metrics.py) — Utilitário read-only local para consultar métricas diárias do Cloud Monitoring (`gen_ai.api.calls`, `scion.usage.tokens`) por agente/modelo na etapa de verificação de custo da Task 8.
- [`docs/scion/operations-runbook.md`](../operations-runbook.md), [`docs/scion/scripts/README.md`](../scripts/README.md), [`docs/scion/README.md`](../README.md) (`2ff8fd1`) — Documentação operacional de Stage 1/2, kill-switch e rollback.

---

## 4. Estado Operacional Atual (Verificado em 2026-11-10 / 2026-11-11)

- **GitHub (`duboc/scion-experiment`):** 0 issues abertas (`#39`–`#54` fechadas); branch `main` limpa e sincronizada com `origin/main` (`2ff8fd1` antes deste handoff).
- **Catálogo (`psearch`):** `20,535` produtos no Spanner (`psearch-db`).
- **Compaction inicial:**
  - `psearch--lead`: `/compact` manual em `2026-11-06T21:54Z` (`764,580 -> 2,326` tokens); estava em `188,283` tokens em `2026-11-10T16:35Z` e permanecia em `188,283` em `2026-11-11T09:35Z`.
  - `psearch--deployer`: `/compact` automático pelo primeiro tick do cron em `2026-11-10T16:35Z` (`377,168 -> 2,496` tokens, evento `LEAD_CONTROLLER_COMPACT` no Cloud Logging).
- **Crontab na VM do Hub (`scion`, observado em `2026-11-11T09:49Z`):**
  - `backup-hub.sh`: ativo (`15 4 * * *`).
  - `factory-watchdog.py`: **comentado/pausado** desde `2026-11-09T19:07:17Z` (`# PAUSED-20261109T190717Z ...`).
  - `sync_factory_agents.py`: ativo (`* * * * *`).
  - `lead-controller.py --apply-compact`: ativo (`*/5 * * * *`, sem `--apply-wake` ainda).
  - Schedule `psearch-lead-heartbeat`: `status=paused` desde `2026-11-09T19:01:24Z`.
- **Anomalias observadas nos logs do cron em `2026-11-11T08:25Z–09:40Z` (INVESTIGAR ANTES DO STAGE 2):**
  1. Os ticks pares (`08:40`, `08:50`, `09:00`, `09:10`, `09:20`, `09:30`, `09:40Z`) registraram `{"result": "locked"}` porque os ticks ímpares (`08:35`, `08:45`, `08:55`, `09:05`, `09:15`, `09:25`, `09:35Z`) levaram **mais de 5 minutos** para terminar — verificar qual subprocesso (`gcloud`, `gh`, `scion`, HTTP) está sem timeout ou lento na VM do Hub.
  2. Apesar do `lead-controller.log` reportar `"deployer_context_tokens": 376457` (acima de `200,000`), `compacted` e `would_compact` apareceram vazios (`[]`) nesses logs — verificar cooldown de compaction ou divergência entre `context_tokens` reportado vs. janela do transcript do `deployer`.

---

## 5. Pendências e Próximos Passos (**Task 8** do Plano)

Referência: [`docs/plans/2026-11-09-lead-cost-controller.md`](../../plans/2026-11-09-lead-cost-controller.md) (Task 8) e [`docs/scion/operations-runbook.md`](../operations-runbook.md).

### Checklist de conclusão
1. **Diagnosticar as 2 anomalias de Stage 1 (antes de habilitar `--apply-wake`):**
   - Medir tempo de cada checagem do `lead-controller.py` na VM do Hub para descobrir por que a execução passa de 5 minutos e causa `locked` no tick seguinte.
   - Inspecionar por que `deployer_context_tokens: 376457` não disparou `compacted` / `would_compact` em `2026-11-11T08:25Z..09:35Z` (e conferir `lead-controller.log` completo desde `2026-11-10T16:30Z`).
2. **Revisar 24h de dry-run (`would_wake`) e estado pausado:**
   - Decidir com o PO se `factory-watchdog.py` (crontab) e `psearch-lead-heartbeat` (schedule) devem permanecer pausados ou ser reativados ao promover para Stage 2.
3. **Promover Stage 2 (quando `lead-controller.py` estiver rodando em `< 5 min` e validado):**
   - Atualizar cron na VM do Hub para `--install-cron --apply-compact --apply-wake`.
   - Atualizar schedule `psearch-lead-heartbeat` para `0 */4 * * *` (safety net de 4h) usando [`scion-admin.py`](../scripts/scion-admin.py).
4. **Verificar metas de custo (Spec §7) em janela de 24h ociosa:**
   - Rodar `python3 docs/scion/scripts/query-token-metrics.py --days 3` e confirmar no Cloud Monitoring / Cloud Logging: `<= 20` chamadas Opus no dia ocioso e `< 1M` tokens de `cache_write` (comparado a ~96 chamadas/dia e dezenas de milhões de tokens antes do controller).
5. **Registrar lições aprendidas e fechar Task 8:**
   - Adicionar entrada final em `docs/scion/lessons-learned.md` (se aplicável ao runbook) e atualizar status no [`docs/scion/README.md`](../README.md).

### Pré-requisitos ao abrir num clone limpo do Jetski Hub

- **`docs/scion/config/factory.env` é git-ignored (`*.env`)** e não virá no clone do Jetski Hub. Antes de executar `run-on-hub.sh`, `query-token-metrics.py` ou comandos SSH, recrie `docs/scion/config/factory.env` a partir de [`docs/scion/config/factory.env.example`](../config/factory.env.example) (ou defina `FACTORY_ENV=/caminho/para/factory.env`) preenchendo ao menos: `HUB_PROJECT`, `HUB_ZONE`, `HUB_VM`, `HUB_OS_USER`, `GITHUB_REPO` e `PO_EMAIL`.
- **Autenticação no ambiente do Jetski Hub:** `gcloud auth login` (ou ADC) com permissão de IAP SSH para `${HUB_VM}` e leitura no Cloud Monitoring/Logging de `${HUB_PROJECT}`.

### Comandos úteis (sem segredos; lê variáveis de `docs/scion/config/factory.env` local)

```bash
# 1. Testes locais obrigatórios após qualquer edição
python3 -m unittest discover -s docs/scion/scripts/tests -q

# Validar também o guard de IDs/emails proibidos nos arquivos versionados:
# (construa FORBIDDEN_ID_PATTERN a partir dos valores reais de docs/scion/config/factory.env)
(cd src/psearch/serving && go test ./...)

# 2. Inspecionar logs completos e duração do lead-controller na VM do Hub
set -a && source docs/scion/config/factory.env && set +a
gcloud compute ssh "${HUB_OS_USER}@${HUB_VM}" \
  --project "${HUB_PROJECT}" --zone "${HUB_ZONE}" \
  --command 'sudo -u scion bash -lc "wc -l ~/.scion/lead-controller.log; grep -E \"compacted|would_compact|would_wake|failures|locked\" ~/.scion/lead-controller.log | tail -n 40; tail -n 30 ~/.scion/lead-controller.cron.log"'

# 3. Ver schedules do Scion na VM do Hub e métricas diárias no Cloud Monitoring
docs/scion/scripts/run-on-hub.sh docs/scion/scripts/scion-admin.py schedule list --project psearch
python3 docs/scion/scripts/query-token-metrics.py --days 3
```

---

## 6. Guardrails do Repositório

1. **Nunca commitar IDs/e-mails reais:** [`TestTrackedSourceContainsNoForbiddenIDPattern`](../../../src/psearch/serving/internal/config/config_test.go) varre **todos os arquivos versionados no Git**. Use sempre placeholders (`${HUB_PROJECT}`, `${SANDBOX_PROJECT}`, `${HUB_VM}`, `${PO_EMAIL}`, `${DEPLOYER_SA}`). O arquivo local `docs/scion/config/factory.env` é git-ignored.
2. **Commits limpos:** nunca incluir `Co-Authored-By`, `Claude-Session` ou trailers de sessão de agente.
3. **Escopo:** não modificar [`docs/scion/scripts/factory-watchdog.py`](../scripts/factory-watchdog.py) nem prompts/templates dos agentes sem pedido explícito.
4. **Higiene de testes:** rodar sempre `python3 -m unittest discover -s docs/scion/scripts/tests` e `go test ./...` em `src/psearch/serving/` antes de declarar conclusão.

---

## 7. Prompt Pronta para Retomar no Jetski Hub

> Leia `docs/scion/handoffs/2026-11-11-jetski-session-handoff.md`, `docs/plans/2026-11-09-lead-cost-controller.md` (Task 8) e `docs/scion/operations-runbook.md`.
> Comece investigando as duas anomalias observadas nos logs de Stage 1 da VM do Hub (`lead-controller.py` demorando >5 min nos ticks ímpares causando `locked` nos pares, e `deployer_context_tokens: 376457` com `compacted: []`), apresente o diagnóstico e proponha os próximos passos antes de promover para Stage 2 (`--apply-wake` + heartbeat `0 */4 * * *`).
