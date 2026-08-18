# INGRESS_CLASS_NOT_FOUND

**Also emits:** `INGRESS_NO_CLASS`
**Severity:** critical (`INGRESS_NO_CLASS` is a warning)
**Confidence:** certain / likely
**Applies to:** Ingress

## What it detects

An Ingress that no controller is going to serve:

- `INGRESS_CLASS_NOT_FOUND` — `spec.ingressClassName` names an IngressClass that does not exist. Certain: the API server says so.
- `INGRESS_NO_CLASS` — the Ingress names no class and the cluster has no default one. Likely rather than certain, because a controller *can* be configured to watch every Ingress regardless of class.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `ingress` | `spec.ingressClassName` | The class asked for, or `unset`. |
| `api` | `ingressclass/<name>` | `NotFound`. |
| `api` | `defaultIngressClass` | Whether any class carries the default annotation. |

## Only checked while nothing has an address

Reading IngressClasses is a cluster-scoped request, and many users cannot make one. So the check only runs when **no controller has published an address** for the Ingress — the only situation where the answer changes anything. An Ingress that is already being served costs no cluster-scoped read, and a user without the permission gets a degradation note rather than a failure.

The legacy `kubernetes.io/ingress.class` annotation names a controller rather than an object, so there is nothing to look up and no finding is produced for it.

## Example

```text
ROOT CAUSE
  IngressClass "nginx" does not exist

  An Ingress is served by the controller its class points at. The class it names
  is not present in the cluster, so no controller will claim this Ingress.

  Evidence
    spec.ingressClassName  nginx
    ingressclass/nginx     NotFound
```

## Limitations

- KubeWhy cannot see whether a controller is actually running, only which classes are registered.
- A controller configured with `--watch-ingress-without-class` serves an Ingress with no class perfectly well; that is why `INGRESS_NO_CLASS` is a warning at likely confidence and lists it as a possible cause.
