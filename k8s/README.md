# Shorty on Kubernetes

Plain manifests plus a Kustomize entrypoint. SQLite is a single-writer
database, so the Deployment is pinned to **one replica** with the
**Recreate** strategy and a `ReadWriteOnce` PVC.

## Before you apply

1. **Set the image** in `deployment.yaml` to your registry/tag, e.g.
   `ghcr.io/usmanxtech/shorty:v0.2.0`.
2. **Set a real secret** in `secret.yaml` (`SHORTY_SECRET`):
   `openssl rand -base64 48`. For production use Sealed Secrets or
   External Secrets instead of a plaintext value.
3. **Set your hostname** in `ingress.yaml` and `configmap.yaml`
   (`SHORTY_BASE_URL`). The example uses `shorty.example.com`.
4. Make sure your cluster has a default StorageClass (for the PVC) and an
   ingress controller (for the Ingress).

## Deploy

```bash
# preview
kubectl kustomize k8s/

# apply
kubectl apply -k k8s/

# watch it come up
kubectl -n shorty rollout status deploy/shorty
kubectl -n shorty get pods,svc,ingress
```

## First-run admin key

Team management needs an admin API key. Mint it against the live PVC:

```bash
kubectl -n shorty exec deploy/shorty -- /app/shorty-cli bootstrap-admin --name ops --db /data/shorty.db
```

The key prints once — store it as `SHORTY_API_KEY`.)

## Notes

- **Scaling:** do not raise `replicas` above 1 — SQLite cannot take
  concurrent writers from multiple pods. For multi-replica setups, migrate
  to Postgres first.
- **Backups:** snapshot the `shorty-data` PVC (or copy `/data/shorty.db`
  from the pod) on a schedule.
- **TLS:** uncomment the TLS block in `ingress.yaml` and point it at your
  cert-manager issuer.
