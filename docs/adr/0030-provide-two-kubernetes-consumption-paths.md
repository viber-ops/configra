# Provide two Kubernetes consumption paths

Configra supports the Secrets Store CSI Driver provider interface for direct file mounts and a namespace-scoped binding controller for native ConfigMap/Secret consumers. Both use workload-specific credentials and a deployment-configured Configra origin; neither exposes a global privileged Configra credential to arbitrary binding parameters.

Native synchronization defaults to Secrets, refuses to overwrite unrelated objects, and requires explicit opt-in to put Vault-resolved content in a ConfigMap. Environment variables still require Pod restart after synchronization; CSI rotation updates files but applications must reload them. Each Configra resource has its own consistent revision identity, rather than implying a transaction spanning multiple resources.
