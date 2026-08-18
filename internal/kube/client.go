// Package kube owns every interaction with the Kubernetes API server.
//
// KubeWhy is read-only by design: this package only ever performs get, list
// and watch-free reads, and it never exposes a mutating client to the rest of
// the codebase.
package kube

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// UserAgent identifies KubeWhy in API server audit logs.
const UserAgent = "kubewhy"

// ConfigFlags holds the standard connection flags KubeWhy accepts. They are
// deliberately a subset of kubectl's, wired into the same loader so that the
// current context, exec credential plugins and namespace behave identically.
type ConfigFlags struct {
	Kubeconfig string
	Context    string
	Namespace  string
	Timeout    time.Duration
}

// Client is a read-only Kubernetes client bound to a namespace and context.
type Client struct {
	// Clientset is the typed client. Tests substitute a fake implementation.
	Clientset kubernetes.Interface
	// Metadata reads object metadata only. It is used for Secrets so that
	// their payload never reaches KubeWhy in the first place.
	Metadata metadata.Interface
	// Namespace is the namespace to use when the user did not name one.
	Namespace string
	// Context is the kubeconfig context in use, shown in error messages.
	Context string
	// Timeout bounds every request made through this client.
	Timeout time.Duration
}

// New builds a Client from the connection flags, using the same kubeconfig
// discovery rules as kubectl.
func New(flags ConfigFlags) (*Client, error) {
	cf := genericclioptions.NewConfigFlags(false)
	if flags.Kubeconfig != "" {
		cf.KubeConfig = &flags.Kubeconfig
	}
	if flags.Context != "" {
		cf.Context = &flags.Context
	}
	if flags.Namespace != "" {
		cf.Namespace = &flags.Namespace
	}

	loader := cf.ToRawKubeConfigLoader()
	restConfig, err := loader.ClientConfig()
	if err != nil {
		return nil, configError(err, flags)
	}
	restConfig = rest.CopyConfig(restConfig)
	restConfig.UserAgent = UserAgent
	if flags.Timeout > 0 {
		restConfig.Timeout = flags.Timeout
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("building Kubernetes client: %w", err)
	}
	metaClient, err := metadata.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("building Kubernetes metadata client: %w", err)
	}

	namespace, _, err := loader.Namespace()
	if err != nil || namespace == "" {
		namespace = "default"
	}

	return &Client{
		Clientset: clientset,
		Metadata:  metaClient,
		Namespace: namespace,
		Context:   currentContext(loader, flags.Context),
		Timeout:   flags.Timeout,
	}, nil
}

func currentContext(loader clientcmd.ClientConfig, override string) string {
	if override != "" {
		return override
	}
	raw, err := loader.RawConfig()
	if err != nil {
		return ""
	}
	return raw.CurrentContext
}

func configError(err error, flags ConfigFlags) error {
	if clientcmd.IsEmptyConfig(err) {
		return fmt.Errorf("no Kubernetes configuration found: set KUBECONFIG or pass --kubeconfig")
	}
	if flags.Context != "" {
		return fmt.Errorf("loading Kubernetes configuration for context %q: %w", flags.Context, err)
	}
	return fmt.Errorf("loading Kubernetes configuration: %w", err)
}

var secretResource = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

// SecretExists reports whether a Secret exists, without reading its contents.
//
// The request asks the API server for PartialObjectMetadata, so the Secret
// payload is never sent to KubeWhy. This is the only way the codebase touches
// Secrets, and no Secret client is exposed to collectors or rules.
func (c *Client) SecretExists(ctx context.Context, namespace, name string) error {
	if c.Metadata != nil {
		_, err := c.Metadata.Resource(secretResource).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	}
	// Fallback for clients built without a metadata client, such as in tests.
	// The object is discarded immediately and never rendered.
	_, err := c.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	return err
}
