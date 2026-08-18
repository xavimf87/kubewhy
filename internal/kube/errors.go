package kube

import (
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
)

// NotFoundError reports that the resource the user asked about does not
// exist. The CLI turns it into a dedicated exit code and a message that says
// where KubeWhy looked.
type NotFoundError struct {
	Ref     diagnosis.ResourceRef
	Context string
}

func (e *NotFoundError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s was not found", e.Ref)
	if e.Ref.Namespace != "" {
		fmt.Fprintf(&b, " in namespace %q", e.Ref.Namespace)
	}
	b.WriteString(".")
	if e.Context != "" {
		fmt.Fprintf(&b, "\n\nCurrent context\n  %s", e.Context)
	}
	fmt.Fprintf(&b, "\n\nTry\n  kubectl get %s", strings.ToLower(e.Ref.Kind))
	if e.Ref.Namespace != "" {
		fmt.Fprintf(&b, " -n %s", e.Ref.Namespace)
	}
	return b.String()
}

// ForbiddenError reports that the current user may not read the resource the
// analysis started from. Related resources degrade instead of failing.
type ForbiddenError struct {
	Ref     diagnosis.ResourceRef
	Context string
	Detail  string
}

func (e *ForbiddenError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "not allowed to read %s", e.Ref)
	if e.Ref.Namespace != "" {
		fmt.Fprintf(&b, " in namespace %q", e.Ref.Namespace)
	}
	b.WriteString(".")
	if e.Context != "" {
		fmt.Fprintf(&b, "\n\nCurrent context\n  %s", e.Context)
	}
	if e.Detail != "" {
		fmt.Fprintf(&b, "\n\nKubernetes returned\n  %s", e.Detail)
	}
	return b.String()
}

// Classify converts an API error on the primary resource into the typed error
// the CLI maps to an exit code. Other errors are returned unchanged.
func Classify(err error, ref diagnosis.ResourceRef, context string) error {
	switch {
	case err == nil:
		return nil
	case apierrors.IsNotFound(err):
		return &NotFoundError{Ref: ref, Context: context}
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return &ForbiddenError{Ref: ref, Context: context, Detail: apiReason(err)}
	default:
		return err
	}
}

// IsNotFound reports whether the error means the object does not exist.
func IsNotFound(err error) bool { return apierrors.IsNotFound(err) }

// IsForbidden reports whether the error means the current user lacks
// permission, including unauthenticated requests.
func IsForbidden(err error) bool {
	return apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err)
}

// apiReason extracts the API server message without the Go error wrapping.
func apiReason(err error) string {
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		if msg := status.Status().Message; msg != "" {
			return msg
		}
	}
	return err.Error()
}

// APIMessage returns the API server's message for an error, without Go's
// wrapping noise. It is safe to show to users.
func APIMessage(err error) string {
	if err == nil {
		return ""
	}
	return apiReason(err)
}
