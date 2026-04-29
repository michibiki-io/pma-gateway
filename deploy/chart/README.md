# pma-gateway Helm Chart

This chart installs the `pma-gateway` microservice, which bundles the Go gateway API, phpMyAdmin, nginx, and PHP-FPM into a single workload.

## Prerequisites

- Helm 3.8 or later
- Kubernetes 1.26 or later
- Access to `ghcr.io/michibiki-io/charts/pma-gateway` for OCI installs
- A `Secret` that provides at least:
  - `PMA_GATEWAY_MASTER_KEY_BASE64`
  - `PMA_GATEWAY_INTERNAL_SHARED_SECRET`

If you keep the default SQLite backend, use a single replica and mount persistent storage at the gateway data directory.

## Install from Local Path

```bash
helm upgrade --install pma-gateway ./deploy/chart \
  --namespace pma-gateway \
  --create-namespace \
  --set secrets.PMA_GATEWAY_MASTER_KEY_BASE64="$PMA_GATEWAY_MASTER_KEY_BASE64" \
  --set secrets.PMA_GATEWAY_INTERNAL_SHARED_SECRET="$PMA_GATEWAY_INTERNAL_SHARED_SECRET"
```

## Install from OCI Registry

```bash
helm install pma-gateway \
  oci://ghcr.io/michibiki-io/charts/pma-gateway \
  --version 0.1.0 \
  --namespace pma-gateway \
  --create-namespace \
  --set secrets.PMA_GATEWAY_MASTER_KEY_BASE64="$PMA_GATEWAY_MASTER_KEY_BASE64" \
  --set secrets.PMA_GATEWAY_INTERNAL_SHARED_SECRET="$PMA_GATEWAY_INTERNAL_SHARED_SECRET"
```

The published chart version and `appVersion` are overridden during release packaging so they match the application release version. If `image.tag` is empty, the chart uses `.Chart.AppVersion`.

## Upgrade

```bash
helm upgrade pma-gateway \
  oci://ghcr.io/michibiki-io/charts/pma-gateway \
  --version 0.1.0 \
  --namespace pma-gateway
```

## Uninstall

```bash
helm uninstall pma-gateway --namespace pma-gateway
```

## Common Examples

### SQLite with Persistent Storage

```bash
helm upgrade --install pma-gateway ./deploy/chart \
  --namespace pma-gateway \
  --create-namespace \
  --set persistence.enabled=true \
  --set persistence.size=5Gi \
  --set secrets.PMA_GATEWAY_MASTER_KEY_BASE64="$PMA_GATEWAY_MASTER_KEY_BASE64" \
  --set secrets.PMA_GATEWAY_INTERNAL_SHARED_SECRET="$PMA_GATEWAY_INTERNAL_SHARED_SECRET"
```

### Ingress

```bash
helm upgrade --install pma-gateway ./deploy/chart \
  --namespace pma-gateway \
  --create-namespace \
  --set config.PMA_GATEWAY_PUBLIC_BASE_PATH=/dbadmin \
  --set ingress.enabled=true \
  --set ingress.className=traefik \
  --set ingress.hosts[0].host=dbadmin.example.com \
  --set ingress.hosts[0].paths[0].path=/dbadmin \
  --set ingress.hosts[0].paths[0].pathType=Prefix
```

### External Secret

```bash
kubectl create secret generic pma-gateway-secret \
  --namespace pma-gateway \
  --from-literal=PMA_GATEWAY_MASTER_KEY_BASE64="$PMA_GATEWAY_MASTER_KEY_BASE64" \
  --from-literal=PMA_GATEWAY_INTERNAL_SHARED_SECRET="$PMA_GATEWAY_INTERNAL_SHARED_SECRET"

helm upgrade --install pma-gateway ./deploy/chart \
  --namespace pma-gateway \
  --create-namespace \
  --set existingSecret=pma-gateway-secret
```

### MySQL and Redis for Multi-Replica Deployments

```bash
helm upgrade --install pma-gateway ./deploy/chart \
  --namespace pma-gateway \
  --create-namespace \
  --set replicaCount=2 \
  --set persistence.enabled=false \
  --set config.PMA_GATEWAY_DATABASE_DRIVER=mysql \
  --set config.PMA_GATEWAY_PHP_SESSION_STORE=redis \
  --set config.PMA_GATEWAY_MYSQL_HOST=mysql.example.internal \
  --set config.PMA_GATEWAY_REDIS_HOST=redis.example.internal \
  --set secrets.PMA_GATEWAY_MASTER_KEY_BASE64="$PMA_GATEWAY_MASTER_KEY_BASE64" \
  --set secrets.PMA_GATEWAY_INTERNAL_SHARED_SECRET="$PMA_GATEWAY_INTERNAL_SHARED_SECRET" \
  --set secrets.PMA_GATEWAY_MYSQL_PASSWORD="$PMA_GATEWAY_MYSQL_PASSWORD" \
  --set secrets.PMA_GATEWAY_REDIS_PASSWORD="$PMA_GATEWAY_REDIS_PASSWORD"
```

## Notes on Replicas and State

- The default chart values use SQLite and local PHP sessions. That mode should run with `replicaCount=1`.
- Multiple replicas require external shared state. Set `PMA_GATEWAY_DATABASE_DRIVER=mysql` and `PMA_GATEWAY_PHP_SESSION_STORE=redis` before scaling.
- Keep `PMA_GATEWAY_MASTER_KEY_BASE64` and `PMA_GATEWAY_INTERNAL_SHARED_SECRET` identical across all replicas.

## Listing Published Versions

```bash
oras repo tags ghcr.io/michibiki-io/charts/pma-gateway
```
