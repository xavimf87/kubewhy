package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	deploymentrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/deployment"
	ingressrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/ingress"
	podrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pod"
	pvcrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pvc"
	servicerules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/service"
	statefulsetrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/statefulset"
)

// newRulesCommand lists the diagnostic rules. KubeWhy's value depends on
// users trusting its findings, and a rule set nobody can enumerate is a black
// box, so listing them is part of the product.
func newRulesCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "rules",
		Short: "List the diagnostic rules and the identifiers they produce",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			groups := ruleGroups()
			if opts.output == "json" {
				encoder := json.NewEncoder(opts.stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(groups)
			}
			for _, group := range groups {
				fmt.Fprintf(opts.stdout, "%s\n", strings.ToUpper(group.Kind))
				for _, rule := range group.Rules {
					fmt.Fprintf(opts.stdout, "  %s\n", rule.Title)
					for _, id := range rule.IDs() {
						fmt.Fprintf(opts.stdout, "    %s\n", id)
					}
				}
				fmt.Fprintln(opts.stdout)
			}
			return nil
		},
	}
}

// ruleGroup is the JSON shape of the rule listing.
type ruleGroup struct {
	Kind  string               `json:"kind"`
	Rules []diagnosis.RuleMeta `json:"rules"`
}

// ruleGroups lists every rule, including the fallbacks. A fallback is not a
// rule, but its identifier appears in the output like any other, so leaving it
// out of the listing would make the tool less explainable, not more accurate.
func ruleGroups() []ruleGroup {
	return []ruleGroup{
		{Kind: "Pod", Rules: append(podrules.Catalog(), podrules.FallbackMeta())},
		{Kind: "Service", Rules: servicerules.Catalog()},
		{Kind: "Deployment", Rules: deploymentrules.Catalog()},
		{Kind: "StatefulSet", Rules: statefulsetrules.Catalog()},
		{Kind: "Ingress", Rules: ingressrules.Catalog()},
		{Kind: "PersistentVolumeClaim", Rules: append(pvcrules.Catalog(), pvcrules.FallbackMeta())},
	}
}
