// Package collect turns Kubernetes API objects into snapshots.
//
// Collectors decide which API calls are worth making. They query only what a
// diagnosis can actually use, and they degrade instead of failing when a
// related object cannot be read.
package collect

import (
	"context"
	"errors"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/snapshot"
)

// degrade records that a related read failed, so the report can tell the user
// which part of the analysis is incomplete and why.
func degrade(c *snapshot.Collection, resource, requiredFor string, err error) {
	if err == nil {
		return
	}
	reason := "Error"
	switch {
	case kube.IsForbidden(err):
		reason = "Forbidden"
	case kube.IsNotFound(err):
		reason = "NotFound"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		reason = "Timeout"
	}
	c.Degrade(diagnosis.Degradation{
		Resource:    resource,
		Reason:      reason,
		RequiredFor: requiredFor,
		Detail:      kube.APIMessage(err),
	})
}

// existence interprets the error of a "does this object exist" read. A read
// that was denied yields Unknown, never Missing: KubeWhy must not report an
// object as absent when it simply could not look.
func existence(c *snapshot.Collection, resource, requiredFor string, err error) snapshot.Existence {
	switch {
	case err == nil:
		return snapshot.Found
	case kube.IsNotFound(err):
		return snapshot.Missing
	default:
		degrade(c, resource, requiredFor, err)
		return snapshot.Unknown
	}
}
