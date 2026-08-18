package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xavimf87/kubewhy/internal/diagnosis"
	podrules "github.com/xavimf87/kubewhy/internal/diagnosis/rules/pod"
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

func ruleGroups() []ruleGroup {
	return []ruleGroup{
		{Kind: "Pod", Rules: podrules.Catalog()},
	}
}
