package cli

// Exit codes are part of KubeWhy's contract with scripts and CI, so they are
// documented and kept stable.
const (
	// ExitOK means the resource was analysed and no issue was detected.
	ExitOK = 0
	// ExitIssueFound means the resource was analysed and an issue was found.
	ExitIssueFound = 1
	// ExitError means KubeWhy could not run: bad flags, no kubeconfig, or an
	// API error that is not a permission or lookup failure.
	ExitError = 2
	// ExitNotFound means the requested resource does not exist.
	ExitNotFound = 3
	// ExitForbidden means the current user may not read the requested
	// resource. Missing permissions on related resources degrade the report
	// instead, and do not produce this code.
	ExitForbidden = 4
)
