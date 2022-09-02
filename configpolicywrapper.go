package main

import (
	"fmt"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type ConfigurationPolicyWrapper struct {
	Annotations          map[string]string `json:"configurationPolicyAnnotations,omitempty"`
	ComplianceType       string            `json:"complianceType,omitempty"`
	ConsolidateManifests bool              `json:"consolidateManifests,omitempty"`
	EvaluationInterval   struct {
		Compliant    string `json:"compliant,omitempty"`
		NonCompliant string `json:"noncompliant,omitempty"`
	} `json:"evaluationInterval,omitempty"`
	MetadataComplianceType string `json:"metadataComplianceType,omitempty"`
	NamespaceSelector      struct {
		Include          []string                 `json:"include,omitempty"`
		Exclude          []string                 `json:"exclude,omitempty"`
		MatchLabels      map[string]string        `json:"matchLabels,omitempty"`
		MatchExpressions []map[string]interface{} `json:"matchExpressions,omitempty"`
	} `json:"namespaceSelector,omitempty"`
	PolicyName          string `json:"policyName"`
	PruneObjectBehavior string `json:"pruneObjectBehavior,omitempty"`
	RemediationAction   string `json:"remediationAction,omitempty"`
	Severity            string `json:"severity,omitempty"`
}

func NewConfigurationPolicyWrapper() ConfigurationPolicyWrapper {
	// Note: leaving things unset in the config will not overwrite these defaults
	// with the "empty" golang values (eg not setting ConsolidateManifests will
	// not make it act like it's false). Stuff "just works" like we'd want.
	return ConfigurationPolicyWrapper{
		ComplianceType:       "musthave",
		ConsolidateManifests: true,
		RemediationAction:    "inform",
	}
}

func (c ConfigurationPolicyWrapper) Filter(operand []*yaml.RNode) ([]*yaml.RNode, error) {
	_, err := ClearInternalAnnotations(operand)
	if err != nil {
		return operand, err
	}

	if c.ConsolidateManifests {
		out := make([]*yaml.RNode, 1)

		out[0], err = c.NewPolicy()
		if err != nil {
			return out, err
		}

		for _, rsrc := range operand {
			wrapped, err := c.WrapResource(rsrc)
			if err != nil {
				return out, err
			}

			err = out[0].PipeE(
				yaml.LookupCreate(yaml.SequenceNode, "spec", "object-templates"),
				yaml.Append(wrapped.YNode()),
			)
		}

		return out, nil
	}

	out := make([]*yaml.RNode, len(operand))

	for i, rsrc := range operand {
		policy, err := c.NewPolicy()
		if err != nil {
			return out, err
		}

		wrapped, err := c.WrapResource(rsrc)
		if err != nil {
			return out, err
		}

		err = policy.PipeE(
			yaml.LookupCreate(yaml.SequenceNode, "spec", "object-templates"),
			yaml.Append(wrapped.YNode()),
		)
		if err != nil {
			return out, err
		}

		err = policy.SetName(fmt.Sprintf("%v-%v", c.PolicyName, i))
		if err != nil {
			return out, err
		}

		out[i] = policy
	}

	return out, nil
}

func (c ConfigurationPolicyWrapper) WrapResource(res *yaml.RNode) (*yaml.RNode, error) {
	wrapped := yaml.MustParse(`{}`)

	err := wrapped.PipeE(
		yaml.SetField("objectDefinition", res),
	)
	if err != nil {
		return wrapped, err
	}

	if c.ComplianceType != "" {
		err := wrapped.PipeE(
			yaml.SetField("complianceType", yaml.MustParse(c.ComplianceType)),
		)
		if err != nil {
			return wrapped, err
		}
	}

	if c.MetadataComplianceType != "" {
		err := wrapped.PipeE(
			yaml.SetField("metadataComplianceType", yaml.MustParse(c.MetadataComplianceType)),
		)
		if err != nil {
			return wrapped, err
		}
	}

	return wrapped, nil
}

const baseConfigPolicy = `
apiVersion: policy.open-cluster-management.io/v1
kind: ConfigurationPolicy
`

func (c ConfigurationPolicyWrapper) NewPolicy() (*yaml.RNode, error) {
	policy := yaml.MustParse(baseConfigPolicy)

	err := policy.SetName(c.PolicyName)
	if err != nil {
		return policy, err
	}

	if len(c.Annotations) != 0 {
		err := policy.SetAnnotations(c.Annotations)
		if err != nil {
			return policy, err
		}
	}

	if c.EvaluationInterval.Compliant != "" {
		err := policy.PipeE(
			yaml.LookupCreate(yaml.MappingNode, "spec", "evaluationInterval"),
			yaml.SetField("compliant", yaml.MustParse(c.EvaluationInterval.Compliant)),
		)
		if err != nil {
			return policy, err
		}
	}

	if c.EvaluationInterval.NonCompliant != "" {
		err := policy.PipeE(
			yaml.LookupCreate(yaml.MappingNode, "spec", "evaluationInterval"),
			yaml.SetField("noncompliant", yaml.MustParse(c.EvaluationInterval.NonCompliant)),
		)
		if err != nil {
			return policy, err
		}
	}

	if len(c.NamespaceSelector.Include) != 0 {
		err := policy.PipeE(
			yaml.LookupCreate(yaml.MappingNode, "spec", "namespaceSelector"),
			yaml.SetField("include", yaml.NewListRNode(c.NamespaceSelector.Include...)),
		)
		if err != nil {
			return policy, err
		}
	}

	if len(c.NamespaceSelector.Exclude) != 0 {
		err := policy.PipeE(
			yaml.LookupCreate(yaml.MappingNode, "spec", "namespaceSelector"),
			yaml.SetField("exclude", yaml.NewListRNode(c.NamespaceSelector.Exclude...)),
		)
		if err != nil {
			return policy, err
		}
	}

	if len(c.NamespaceSelector.MatchLabels) != 0 {
		err := policy.PipeE(
			yaml.LookupCreate(yaml.MappingNode, "spec", "namespaceSelector"),
			yaml.SetField("matchLabels", yaml.NewMapRNode(&c.NamespaceSelector.MatchLabels)),
		)
		if err != nil {
			return policy, err
		}
	}

	for _, exp := range c.NamespaceSelector.MatchExpressions {
		obj, err := yaml.FromMap(exp)
		if err != nil {
			return policy, err
		}

		err = policy.PipeE(
			yaml.LookupCreate(yaml.SequenceNode, "spec", "namespaceSelector", "matchExpressions"),
			yaml.Append(obj.YNode()),
		)
		if err != nil {
			return policy, err
		}
	}

	if c.PruneObjectBehavior != "" {
		err := policy.PipeE(
			yaml.LookupCreate(yaml.MappingNode, "spec"),
			yaml.SetField("pruneObjectBehavior", yaml.MustParse(c.PruneObjectBehavior)),
		)
		if err != nil {
			return policy, err
		}
	}

	if c.RemediationAction != "" {
		err := policy.PipeE(
			yaml.LookupCreate(yaml.MappingNode, "spec"),
			yaml.SetField("remediationAction", yaml.MustParse(c.RemediationAction)),
		)
		if err != nil {
			return policy, err
		}
	}

	if c.Severity != "" {
		err := policy.PipeE(
			yaml.LookupCreate(yaml.MappingNode, "spec"),
			yaml.SetField("severity", yaml.MustParse(c.Severity)),
		)
		if err != nil {
			return policy, err
		}
	}

	return policy, nil
}
