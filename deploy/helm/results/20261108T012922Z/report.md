# Helm chart smoke - 20261108T012922Z

- Cluster: kind `observaki-helm` (3 nodes)
- Chart: `deploy/helm/observaki` (0.1.0-alpha.1)
- Release: `observaki` in namespace `observaki`
- Helm: v4.3.1
- Kubernetes: v1.37.0
- Host: Linux 7.0.0-42-generic x86_64

## Install

| Metric | Value |
|---|---|
| Install wall clock | 47 s |
| Healthy limit | 600 s |
| API reachable | {"status":"ok"} |
| Gateway ready | {"status":"ok"} |
| Nodes Ready | 3 |

### Pods

```
NAME                                 READY   STATUS      RESTARTS      AGE   IP            NODE                     NOMINATED NODE   READINESS GATES
observaki-api-7877d8cd45-4sctt       1/1     Running     0             20s   10.244.2.22   observaki-helm-worker2   <none>           <none>
observaki-api-7877d8cd45-ftnhm       1/1     Running     0             16s   10.244.1.9    observaki-helm-worker    <none>           <none>
observaki-clickhouse-0               1/1     Running     0             15s   10.244.1.10   observaki-helm-worker    <none>           <none>
observaki-gateway-75ccb99bd-22mx2    1/1     Running     0             7s    10.244.1.11   observaki-helm-worker    <none>           <none>
observaki-gateway-75ccb99bd-8482f    1/1     Running     2 (19s ago)   20s   10.244.2.23   observaki-helm-worker2   <none>           <none>
observaki-migrate-161220efd3-tdfjt   0/1     Completed   0             92s   10.244.2.12   observaki-helm-worker2   <none>           <none>
observaki-migrate-eab039da14-jb57r   0/1     Completed   0             30s   10.244.2.21   observaki-helm-worker2   <none>           <none>
observaki-nats-0                     1/1     Running     0             19s   10.244.2.25   observaki-helm-worker2   <none>           <none>
observaki-postgres-0                 1/1     Running     0             19s   10.244.2.26   observaki-helm-worker2   <none>           <none>
observaki-test-api                   0/1     Completed   0             52s   10.244.2.13   observaki-helm-worker2   <none>           <none>
observaki-test-gateway               0/1     Completed   0             50s   10.244.2.14   observaki-helm-worker2   <none>           <none>
observaki-victoriametrics-0          1/1     Running     0             19s   10.244.2.24   observaki-helm-worker2   <none>           <none>
observaki-writer-0                   1/1     Running     0             17s   10.244.2.27   observaki-helm-worker2   <none>           <none>
```

### Persistent volumes

```
NAME                               STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   VOLUMEATTRIBUTESCLASS   AGE
data-observaki-clickhouse-0        Bound    pvc-81c2f593-45e0-40cc-8a89-7d8e7f48852b   20Gi       RWO            standard       <unset>                 2m16s
data-observaki-nats-0              Bound    pvc-3ac38492-2856-4c3f-bb69-83f3c2d5e180   20Gi       RWO            standard       <unset>                 2m16s
data-observaki-postgres-0          Bound    pvc-463a4ca8-0bf0-42fa-82c7-598a9e660a9e   2Gi        RWO            standard       <unset>                 2m16s
data-observaki-victoriametrics-0   Bound    pvc-f64d6114-9abc-4831-858c-b5c3c6366d78   10Gi       RWO            standard       <unset>                 2m16s
spool-observaki-writer-0           Bound    pvc-162d2bb1-4acc-42f0-a86e-6dcce4954826   10Gi       RWO            standard       <unset>                 2m16s
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

- Upgrade wall clock: 22 s
- Rollback wall clock: 17 s
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
| PASS | helm install completed in 47s |
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
