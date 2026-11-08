# Helm chart smoke - 20261108T030211Z

- Cluster: kind `observaki-helm` (3 nodes)
- Chart: `deploy/helm/observaki` (0.1.0-alpha.1)
- Release: `observaki` in namespace `observaki`
- Helm: v4.3.1
- Kubernetes: unknown
- Host: Linux 7.0.0-42-generic x86_64

## Install

| Metric | Value |
|---|---|
| Install wall clock | 45 s |
| Healthy limit | 600 s |
| API reachable | {"status":"ok"} |
| Gateway ready | {"status":"ok"} |
| Nodes Ready | 3 |

### Pods

```
```

### Persistent volumes

```
```

## AC1 - Install

- Workloads and bundled dependencies ready: pass
- Sample telemetry: pass
- ClickHouse logs/traces: 2 logs / 2 traces
- VictoriaMetrics sample present: pass

## AC2 - Upgrade and rollback

| Step | Revision | Chart | Image tags | Logs | Traces |
|---|---|---|---|---|---|
| Installed | 1 | observaki-0.1.0-alpha.1 | observaki:local | 2 | 2 |
| Upgraded | 2 | observaki-0.1.0-alpha.1.1 | observaki:local-v2 | 4 | 4 |
| Rolled back | 3 | observaki-0.1.0-alpha.1 | observaki:local | 4 | 4 |

- Upgrade wall clock: 25 s
- Rollback wall clock: 18 s
- Chart version bumped and restored: pass
- Data preserved across upgrade and rollback: pass
- Rollback restored the previous image: pass

## AC3 - Documentation

- Chart README sections: pass

## Checks

| Result | Check |
|---|---|
| PASS | built observaki:local and observaki:local-v2 |
| PASS | created 3-node kind cluster |
| PASS | loaded images into the kind nodes |
| PASS | helm install completed in 45s |
| PASS | install under the 600s health limit |
| PASS | deployment/observaki-api ready replicas 2/2 |
| PASS | deployment/observaki-gateway ready replicas 2/2 |
| PASS | statefulset/observaki-writer ready replicas 1/1 |
| PASS | statefulset/observaki-clickhouse ready replicas 1/1 |
| PASS | statefulset/observaki-victoriametrics ready replicas 1/1 |
| PASS | statefulset/observaki-postgres ready replicas 1/1 |
| PASS | statefulset/observaki-nats ready replicas 1/1 |
| PASS | 3 cluster nodes Ready (3) |
| PASS | persistent volume claims bound (5) |
| PASS | API answers /healthz through the Service |
| PASS | gateway answers /readyz through the Service |
| PASS | telemetry reached ClickHouse (2 logs / 2 traces) |
| PASS | metric reached VictoriaMetrics |
| PASS | helm test |
| PASS | upgrade rolled the pods to observaki:local-v2 |
| PASS | API and gateway reachable after the upgrade |
| PASS | telemetry after the upgrade (4 logs / 4 traces) |
| PASS | chart version moved to 0.1.0-alpha.1.1 and rolled back to 0.1.0-alpha.1 |
| PASS | rollback restored the previous image (observaki:local) |
| PASS | API and gateway reachable after the rollback |
| PASS | data survived the upgrade and rollback (4 logs / 4 traces) |
| PASS | chart README documents prerequisites, values, sizing, upgrade and troubleshooting |

**Summary:** 27 passed, 0 failed.
