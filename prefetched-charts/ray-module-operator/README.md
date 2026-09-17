# Ray module operator Helm chart

This chart contains the module-controller installation bundle only. KubeRay
operand manifests are managed separately by the Ray module operator.

The rendered installation bundle under `files/` is synchronized from the
existing Kustomize installation with:

```sh
make helm-chart-sync
```

The Helm rendering supports overriding the controller image, related operand
images, replica count, resources, application namespace, and installation
namespace through `values.yaml`. The namespace is intentionally not managed as
a chart resource; use `--create-namespace` or create it before installing.

In the platform deployment, the ODH Operator renders this chart and applies
the manifests with server-side apply. A standalone Helm install is intended
for a cluster where the module CRD is not already owned by OLM; Helm cannot
adopt an existing OLM-owned CRD because of its ownership metadata.
