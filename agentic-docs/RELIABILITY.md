# Reliability & Observability

## Service Level Objectives (SLOs)

### Availability

**Target**: 99.9% uptime for pod placement operand components

**Measurement**: Component readiness probes and leader election stability

**Impact of Downtime**:
- Operator down: No new deployments of pod placement operand (existing operands continue working)
- Controller down: New pods remain gated indefinitely (existing scheduled pods unaffected)
- Webhook down: New pods created without gates, may be scheduled to incompatible nodes

**Mitigation**: Leader election for controllers, multiple webhook replicas

### Latency

**Target**: P95 pod ungating latency < 10s (from pod creation to gate removal)

**Components**:
- Webhook response time: < 500ms (P95)
- Image inspection time: < 5s (P95, includes registry network call)
- Controller reconciliation: < 1s (P95, cached images)

**Measurement**: Prometheus metrics `mto_ppo_ctrl_time_to_process_gated_pod_seconds`

**Degradation Scenarios**:
- Registry latency spike: Inspection timeout (10s default), fallback architecture used if configured
- High pod creation rate: Reconciliation queue depth grows, autoscaling kicks in (horizontal pod autoscaler)

## Metrics

All components expose Prometheus metrics at `:8080/metrics`.

### Pod Placement Controller Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `mto_ppo_ctrl_time_to_process_pod_seconds` | Histogram | Time to process any pod |
| `mto_ppo_ctrl_time_to_process_gated_pod_seconds` | Histogram | Time to process gated pods (includes inspection) |
| `mto_ppo_ctrl_time_to_inspect_image_seconds` | Histogram | Image inspection latency |
| `mto_ppo_ctrl_processed_pods_total` | Counter | Total gated pods processed |
| `mto_ppo_ctrl_failed_image_inspection_total` | Counter | Failed image inspections |

### Webhook Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `mto_ppo_wh_pods_processed_total` | Counter | Total pods processed by webhook |
| `mto_ppo_wh_pods_gated_total` | Counter | Total pods gated |
| `mto_ppo_wh_response_time_seconds` | Histogram | Webhook response time |

### Shared Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `mto_ppo_pods_gated` | Gauge | Current number of gated pods (should converge to 0) |

**Monitoring Setup**: See `docs/metrics.md` for Grafana dashboard and example PromQL queries.

## Alerting

### Critical Alerts

**PodPlacementControllerDown**: No controller instances running
- **Severity**: Critical
- **Impact**: New pods remain gated indefinitely
- **Runbook**: Check deployment status, leader election logs

**PodPlacementWebhookDown**: Webhook not responding
- **Severity**: Critical
- **Impact**: New pods created without gates, may fail on incompatible nodes
- **Runbook**: Check webhook deployment, certificate expiration

**HighGatedPodCount**: `mto_ppo_pods_gated` > 100 for 5 minutes
- **Severity**: Warning
- **Impact**: Pods waiting for placement, possible controller degradation
- **Runbook**: Check image inspection failures, registry connectivity

### Warning Alerts

**HighImageInspectionFailureRate**: `mto_ppo_ctrl_failed_image_inspection_total` increasing rapidly
- **Severity**: Warning
- **Impact**: Pods may use fallback architecture or remain gated
- **Runbook**: Check registry connectivity, pull secret validity

**ENoExecEventDetected**: Exec format error detected on node
- **Severity**: Warning
- **Impact**: Workload scheduled to incompatible architecture (operator failed to prevent)
- **Runbook**: Investigate pod placement logic, check for webhook bypass

## Failure Modes

### Image Inspection Failure

**Cause**: Registry unreachable, authentication failure, manifest not found

**Behavior**:
- If `fallbackArchitecture` configured: Use fallback, remove gate
- If not configured: Retry with exponential backoff (max 5 attempts), then leave pod gated

**Recovery**: Pod must be deleted and recreated, or manually ungated

### Leader Election Loss

**Cause**: Network partition, API server unreachable

**Behavior**: Current leader stops reconciling, new leader elected within 15s (default)

**Impact**: Temporary pause in pod ungating (no data loss, queued pods eventually processed)

### Webhook Certificate Expiration

**Cause**: Cert-manager not rotating webhook TLS certificate

**Behavior**: Webhook fails, API server rejects pod creations

**Recovery**: Automatic (cert-manager renews certificates 30 days before expiration)

## Concurrency & Scaling

**Pod Placement Controller**:
- `MaxConcurrentReconciles = NumCPU * 4` (I/O bound workload due to image inspection)
- Horizontal scaling: Multiple controller replicas with leader election (only one active)
- Vertical scaling: Increase CPU/memory limits for more concurrent workers

**Webhook**:
- Stateless, horizontally scalable
- Uses worker pool (ants library, 16 workers) for event publishing

## Related Documents

- [→ Component: Pod Placement Controller](design-docs/components/pod-placement-controller.md) - Reconciliation internals
- [→ Concept: Image Inspection](domain/concepts/image-inspection.md) - Inspection failure handling
- [→ Workflow: Error Recovery](domain/workflows/error-recovery.md) - Failure scenarios
