// Package cli wires the command line to the analysis pipeline.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/xavimf87/kubewhy/internal/analyze"
	"github.com/xavimf87/kubewhy/internal/diagnosis"
	"github.com/xavimf87/kubewhy/internal/kube"
	"github.com/xavimf87/kubewhy/internal/output"
	"github.com/xavimf87/kubewhy/internal/version"
)

// options holds every flag the root command accepts.
type options struct {
	kubeconfig string
	context    string
	namespace  string
	output     string
	color      string
	noColor    bool
	verbose    bool
	timeout    time.Duration

	stdout io.Writer
	stderr io.Writer

	// newClient builds the Kubernetes client. It is a field so that tests can
	// exercise the command end to end against a fake API.
	newClient func(kube.ConfigFlags) (*kube.Client, error)

	// issueFound records whether the analysed resource had findings, which
	// decides the process exit code.
	issueFound bool
}

// Execute runs KubeWhy and returns the process exit code.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return execute(ctx, args, stdout, stderr, kube.New)
}

func execute(ctx context.Context, args []string, stdout, stderr io.Writer, newClient func(kube.ConfigFlags) (*kube.Client, error)) int {
	opts := &options{stdout: stdout, stderr: stderr, newClient: newClient}
	cmd := newRootCommand(opts)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err := cmd.ExecuteContext(ctx); err != nil {
		return reportError(stderr, err)
	}
	if opts.issueFound {
		return ExitIssueFound
	}
	return ExitOK
}

func newRootCommand(opts *options) *cobra.Command {
	// Cobra derives the command name from the first word of Use, which has to
	// be a single token; the invocation shown in examples and errors is the
	// friendlier "kubectl why".
	name := invocationName()

	cmd := &cobra.Command{
		Use:   binaryName() + " RESOURCE NAME",
		Short: "Explain why a Kubernetes resource is not working",
		Long: strings.TrimSpace(`
KubeWhy correlates the status, conditions, events and related objects of a
Kubernetes resource, and explains what they say about it.

It is read-only: it never modifies, restarts or deletes anything, never runs
commands inside containers, and never reads Secret contents.`),
		Example: examples(fmt.Sprintf(`
  # Diagnose a Pod in the current namespace
  %[1]s pod api-7b89d8c9-xfd2

  # Diagnose a Pod in another namespace and cluster
  %[1]s pod api-7b89d8c9-xfd2 -n production --context prod-cluster

  # Machine-readable output for scripts and CI
  %[1]s pod api-7b89d8c9-xfd2 -o json`, name)),
		Args:              cobra.MaximumNArgs(2),
		SilenceUsage:      true,
		SilenceErrors:     true,
		ValidArgsFunction: completeArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion, _ := cmd.Flags().GetBool("version"); showVersion {
				fmt.Fprintln(opts.stdout, version.Get())
				return nil
			}
			// Called with nothing to diagnose, the useful answer is the help
			// text rather than an error.
			if len(args) == 0 {
				return cmd.Help()
			}
			return opts.run(cmd.Context(), args)
		},
	}

	flags := cmd.PersistentFlags()
	flags.StringVar(&opts.kubeconfig, "kubeconfig", "", "path to the kubeconfig file to use")
	flags.StringVar(&opts.context, "context", "", "name of the kubeconfig context to use")
	flags.StringVarP(&opts.namespace, "namespace", "n", "", "namespace of the resource (defaults to the context's namespace)")
	flags.StringVarP(&opts.output, "output", "o", "text", "output format: text or json")
	flags.StringVar(&opts.color, "color", "auto", "when to colour the output: auto, always or never")
	flags.BoolVar(&opts.noColor, "no-color", false, "disable coloured output (same as --color never)")
	flags.BoolVarP(&opts.verbose, "verbose", "v", false, "show every piece of evidence and what KubeWhy inspected")
	flags.DurationVar(&opts.timeout, "timeout", 15*time.Second, "maximum time to wait for the Kubernetes API")

	cmd.Flags().BoolP("version", "V", false, "print version information and exit")

	cmd.AddCommand(newVersionCommand(opts))
	cmd.AddCommand(newRulesCommand(opts))
	return cmd
}

// invocationName returns the command as a user is meant to type it: through
// kubectl when the binary is installed as a plugin, and by its own name
// otherwise.
func invocationName() string {
	base := binaryName()
	if strings.HasPrefix(base, "kubectl-") {
		return "kubectl " + strings.TrimPrefix(base, "kubectl-")
	}
	return base
}

// binaryName returns the name the binary was invoked with, falling back to
// the canonical plugin name when that cannot be determined.
func binaryName() string {
	base := filepath.Base(os.Args[0])
	if base == "" || base == "." || strings.HasPrefix(base, "-") {
		return "kubectl-why"
	}
	return base
}

// examples keeps the indentation of an example block while dropping the
// blank lines a raw string literal adds around it.
func examples(block string) string {
	return strings.Trim(block, "\n")
}

func (o *options) run(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("a resource name is required.\n\nUsage\n  %s RESOURCE NAME\n\nSupported resources\n  %s",
			invocationName(), strings.Join(kube.SupportedResources(), "\n  "))
	}
	if o.output != "text" && o.output != "json" {
		return fmt.Errorf("unknown output format %q: use text or json", o.output)
	}
	if _, err := o.colorMode(); err != nil {
		return err
	}

	kind, err := kube.ResolveKind(args[0])
	if err != nil {
		return err
	}

	client, err := o.newClient(kube.ConfigFlags{
		Kubeconfig: o.kubeconfig,
		Context:    o.context,
		Namespace:  o.namespace,
		Timeout:    o.timeout,
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	report, err := analyze.Analyze(ctx, client, kind, client.Namespace, args[1])
	if err != nil {
		return err
	}

	o.issueFound = report.Status == diagnosis.StatusUnhealthy || report.Status == diagnosis.StatusDegraded
	return o.render(report)
}

func (o *options) render(report *diagnosis.Report) error {
	if o.output == "json" {
		return output.JSON(o.stdout, report)
	}
	mode, err := o.colorMode()
	if err != nil {
		return err
	}
	return output.Text(o.stdout, report, output.TextOptions{
		Style:   output.DetectStyle(o.stdout, mode),
		Verbose: o.verbose,
	})
}

// colorMode resolves the two flags that control colour. --no-color is kept as
// the familiar spelling of --color=never.
func (o *options) colorMode() (output.ColorMode, error) {
	if o.noColor {
		return output.ColorNever, nil
	}
	return output.ParseColorMode(o.color)
}

func completeArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return kube.KindAliases(), cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// reportError prints a user-facing error and maps it to an exit code.
func reportError(stderr io.Writer, err error) int {
	var notFound *kube.NotFoundError
	var forbidden *kube.ForbiddenError

	switch {
	case errors.As(err, &notFound):
		fmt.Fprintf(stderr, "Error: %s\n", notFound)
		return ExitNotFound
	case errors.As(err, &forbidden):
		fmt.Fprintf(stderr, "Error: %s\n", forbidden)
		return ExitForbidden
	case errors.Is(err, context.DeadlineExceeded):
		fmt.Fprintf(stderr, "Error: the Kubernetes API did not answer in time.\n\nTry\n  raising --timeout, or checking that the cluster is reachable\n")
		return ExitError
	default:
		fmt.Fprintf(stderr, "Error: %s\n", err)
		return ExitError
	}
}
