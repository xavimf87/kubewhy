# INGRESS_SERVICE_NOT_FOUND

**Also emits:** `INGRESS_SERVICE_PORT_NOT_FOUND`, `INGRESS_SERVICE_NO_ENDPOINTS`, `INGRESS_NO_RULES`
**Severity:** critical (`INGRESS_NO_RULES` is a warning)
**Confidence:** certain
**Applies to:** Ingress

## What it detects

This rule walks the chain an Ingress depends on and reports the first link that is broken:

```text
Ingress → Service → port → EndpointSlice → Pods
```

| Identifier | Where the chain breaks |
| --- | --- |
| `INGRESS_SERVICE_NOT_FOUND` | The backend Service does not exist. |
| `INGRESS_SERVICE_PORT_NOT_FOUND` | The Service exists but does not expose the port the Ingress names. |
| `INGRESS_SERVICE_NO_ENDPOINTS` | The Service exists and exposes the port, but has no ready endpoints. |
| `INGRESS_NO_RULES` | The Ingress declares no rules and no default backend, so there is nothing to route. |

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `ingress` | `route` (one per affected path) | `api.example.com/ -> Service/api:80` |
| `api` | `service/<name>` | `NotFound`. |
| `ingress` | `backend.service.port` | The port the Ingress asks for. |
| `service` | `spec.ports` | The ports the Service actually exposes. |
| `endpointslice` | `readyEndpoints`, `notReadyEndpoints` | What the Service publishes. |

## One finding per Service, not per path

Several paths often route to the same Service. Reporting the same broken backend three times buries it, so findings are produced per Service and every affected route is listed as evidence.

## The Pods supply the reason

When a backend has no endpoints, KubeWhy collects the Pods behind that Service and runs the Pod rules over them, aggregated. This only happens for Services that publish nothing: a healthy backend costs no Pod lookup at all.

## Example

```text
ROOT CAUSE
  Container "api" is failing its readiness probe (2 of 2 Pods)
  …

CONSEQUENCE
  Service "api" has no ready endpoints, so its routes return errors

  Evidence
    route              api.example.com/ -> Service/api:80
    readyEndpoints     0
    notReadyEndpoints  2
```

## Limitations

- **No controller-specific knowledge.** KubeWhy does not interpret NGINX, Traefik, HAProxy, ALB or any other controller: their behaviour is not in the Kubernetes API. It diagnoses the resource chain and stops there.
- Annotations that change routing behaviour are not interpreted.
- An Ingress cannot route to a Service in another namespace; that is listed as a possible cause, not detected.
- An ExternalName backend publishes no endpoints by design and is not reported.
