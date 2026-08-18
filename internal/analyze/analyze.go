// Package analyze turns a resource reference into a finished report.
//
// It is the only place that knows the whole picture: it collects a snapshot,
// runs the rules for that kind, and adds the factual context a user wants to
// see whether or not anything is wrong.
package analyze

import (
	"context"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
)

// Analyze diagnoses one resource and returns its report.
func Analyze(ctx context.Context, client *kube.Client, kind kube.Kind, namespace, name string) (*diagnosis.Report, error) {
	switch kind {
	case kube.KindPod:
		return Pod(ctx, client, namespace, name)
	case kube.KindService:
		return Service(ctx, client, namespace, name)
	case kube.KindDeployment:
		return Deployment(ctx, client, namespace, name)
	case kube.KindIngress:
		return Ingress(ctx, client, namespace, name)
	case kube.KindPVC:
		return PVC(ctx, client, namespace, name)
	default:
		return nil, &UnsupportedKindError{Kind: kind}
	}
}

// UnsupportedKindError reports a kind KubeWhy resolves but cannot diagnose
// yet, which keeps the CLI honest about its scope.
type UnsupportedKindError struct {
	Kind kube.Kind
}

func (e *UnsupportedKindError) Error() string {
	return string(e.Kind) + " diagnostics are not implemented yet in this build of KubeWhy."
}

// headline renders the one-line verdict shared by every kind.
func headline(ref diagnosis.ResourceRef, status diagnosis.Status) string {
	switch status {
	case diagnosis.StatusUnhealthy:
		return ref.String() + " is unhealthy"
	case diagnosis.StatusDegraded:
		return ref.String() + " is degraded"
	case diagnosis.StatusUnknown:
		return ref.String() + " could not be fully analysed"
	default:
		return ref.String() + " appears healthy"
	}
}
