# Deliberately broken manifests

These manifests fail on purpose. They exist so you can see each diagnosis for real, and so contributors can reproduce a case before writing a rule for it.

They are safe in the sense that matters: nothing here is privileged, nothing escapes its container, nothing touches the host, and nothing runs a network service. They are unsafe only in that they will never become healthy.

**Apply them to a throwaway cluster** — kind, k3d, minikube. Not to anything you care about.

```bash
kubectl create namespace kubewhy-demo

kubectl apply -n kubewhy-demo -f examples/broken/oom.yaml
sleep 30                                    # give the kubelet time to report
kubectl why pod oom-demo -n kubewhy-demo

kubectl apply -n kubewhy-demo -f examples/broken/service-no-endpoints.yaml
sleep 45
kubectl why service payments-demo -n kubewhy-demo
```

Clean up with:

```bash
kubectl delete namespace kubewhy-demo
```

| Manifest | What breaks | Expected finding |
| --- | --- | --- |
| `oom.yaml` | A container allocates memory past its 32Mi limit. | `POD_OOM_KILLED` |
| `crash-loop.yaml` | The process exits with code 1 immediately, forever. | `POD_CRASH_LOOP` |
| `command-not-found.yaml` | The entrypoint does not exist in the image. | `POD_CRASH_LOOP` (exit code 127) |
| `image-pull.yaml` | The image reference does not exist. | `POD_IMAGE_PULL_FAILED` |
| `unschedulable-cpu.yaml` | Requests more CPU than any node has. | `POD_UNSCHEDULABLE_CPU` |
| `node-selector.yaml` | Selects a node label nothing carries. | `POD_UNSCHEDULABLE_NODE_AFFINITY` |
| `missing-configmap.yaml` | Mounts a ConfigMap that does not exist. | `POD_MISSING_CONFIGMAP` |
| `missing-secret.yaml` | Reads an environment variable from a Secret that does not exist. | `POD_MISSING_SECRET` |
| `readiness-probe.yaml` | The readiness probe targets a port nothing listens on. | `POD_READINESS_PROBE_FAILED` |
| `init-container.yaml` | The init container fails, so the Pod never starts. | `POD_INIT_CONTAINER_FAILED` |
| `pvc-missing-storageclass.yaml` | The claim asks for a storage class that does not exist. | `POD_PVC_NOT_BOUND` and `POD_UNSCHEDULABLE_VOLUME` |
| `service-selector.yaml` | A Service selecting a workload that does not exist. | `SERVICE_NO_MATCHING_PODS` |
| `service-no-endpoints.yaml` | Three backends that never become ready. | `SERVICE_NO_READY_ENDPOINTS`, caused by `POD_READINESS_PROBE_FAILED` |
| `deployment-oom.yaml` | Three replicas dying the same way. | `POD_OOM_KILLED` as "3 of 3 Pods", then `DEPLOYMENT_UNAVAILABLE_REPLICAS` |
| `ingress-missing-service.yaml` | An Ingress routing to a Service that does not exist. | `INGRESS_SERVICE_NOT_FOUND` |
| `healthy.yaml` | Nothing. It is the control case. | No findings |

The last one matters as much as the rest: a tool that finds a problem everywhere is as useless as one that finds none.
