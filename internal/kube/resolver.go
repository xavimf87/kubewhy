package kube

import (
	"fmt"
	"sort"
	"strings"
)

// Kind is a Kubernetes kind KubeWhy knows how to diagnose.
type Kind string

// Kinds supported by KubeWhy.
const (
	KindPod         Kind = "Pod"
	KindService     Kind = "Service"
	KindDeployment  Kind = "Deployment"
	KindStatefulSet Kind = "StatefulSet"
	KindIngress     Kind = "Ingress"
	KindPVC         Kind = "PersistentVolumeClaim"
)

// kindAliases maps every accepted user input to a supported kind. The aliases
// match the ones kubectl accepts, so muscle memory carries over.
var kindAliases = map[string]Kind{
	"pod":                    KindPod,
	"pods":                   KindPod,
	"po":                     KindPod,
	"service":                KindService,
	"services":               KindService,
	"svc":                    KindService,
	"deployment":             KindDeployment,
	"deployments":            KindDeployment,
	"deploy":                 KindDeployment,
	"statefulset":            KindStatefulSet,
	"statefulsets":           KindStatefulSet,
	"sts":                    KindStatefulSet,
	"ingress":                KindIngress,
	"ingresses":              KindIngress,
	"ing":                    KindIngress,
	"persistentvolumeclaim":  KindPVC,
	"persistentvolumeclaims": KindPVC,
	"pvc":                    KindPVC,
}

// ResolveKind maps a user-supplied resource argument to a supported kind.
// Input is case-insensitive and may carry an API group suffix, so both
// "deploy" and "deployments.apps" resolve.
func ResolveKind(arg string) (Kind, error) {
	key := strings.ToLower(strings.TrimSpace(arg))
	if i := strings.Index(key, "."); i > 0 {
		key = key[:i]
	}
	if kind, ok := kindAliases[key]; ok {
		return kind, nil
	}
	return "", fmt.Errorf("%q is not a resource KubeWhy can diagnose yet.\n\nSupported resources\n  %s",
		arg, strings.Join(SupportedResources(), "\n  "))
}

// SupportedResources lists the canonical resources with their aliases, for
// help text and error messages.
func SupportedResources() []string {
	byKind := map[Kind][]string{}
	for alias, kind := range kindAliases {
		byKind[kind] = append(byKind[kind], alias)
	}
	order := []Kind{KindPod, KindService, KindDeployment, KindStatefulSet, KindIngress, KindPVC}
	out := make([]string, 0, len(order))
	for _, kind := range order {
		canonical := strings.ToLower(string(kind))
		// Longest first, so the plural form leads and the short alias closes.
		aliases := make([]string, 0, len(byKind[kind]))
		for _, alias := range byKind[kind] {
			if alias != canonical {
				aliases = append(aliases, alias)
			}
		}
		sort.Slice(aliases, func(i, j int) bool {
			if len(aliases[i]) != len(aliases[j]) {
				return len(aliases[i]) > len(aliases[j])
			}
			return aliases[i] < aliases[j]
		})
		out = append(out, fmt.Sprintf("%-22s aliases: %s", canonical, strings.Join(aliases, ", ")))
	}
	return out
}

// KindAliases returns every accepted alias, sorted, for shell completion.
func KindAliases() []string {
	out := make([]string, 0, len(kindAliases))
	for alias := range kindAliases {
		out = append(out, alias)
	}
	sort.Strings(out)
	return out
}
