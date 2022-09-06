package main

import (
	"fmt"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type PolicyWrapper struct {
	Categories            []string `json:"categories,omitempty"`
	Controls              []string `json:"controls,omitempty"`
	ConsolidateManifests  bool     `json:"consolidateManifests,omitempty"`
	ConsolidatePlacements bool     `json:"consolidatePlacements,omitempty"`
	Disabled              bool     `json:"disabled,omitempty"`
	IgnoreNonPolicies     bool     `json:"ignoreNonPolicies,omitempty"`
	PlacementSpec         struct {
		IgnoreExisting   bool              `json:"ignoreExisting,omitempty"`
		ClusterSelectors map[string]string `json:"clusterSelectors,omitempty"` // These (unnecessarily?) limit what can
		LabelSelector    map[string]string `json:"labelSelector,omitempty"`    // be defined in the placements...
	} `json:"placement,omitempty"`
	PolicyName        string   `json:"policyName,omitempty"`
	RemediationAction string   `json:"remediationAction,omitempty"`
	Standards         []string `json:"standards,omitempty"`
}

func NewPolicyWrapper() PolicyWrapper {
	// Note: leaving things unset in the config will not overwrite these defaults
	// with the "empty" golang values (eg not setting ConsolidateManifests will
	// not make it act like it's false). Stuff "just works" like we'd want.
	w := PolicyWrapper{
		ConsolidateManifests:  true,
		ConsolidatePlacements: false,
		Disabled:              false,
		IgnoreNonPolicies:     true,
	}
	w.PlacementSpec.IgnoreExisting = true

	return w
}

func (c PolicyWrapper) Filter(operand []*yaml.RNode) ([]*yaml.RNode, error) {
	policies, other, inputPlacement := Split(operand)

	if c.IgnoreNonPolicies { // only wrap policies, leave others unchanged
		operand = policies
	}

	_, err := ClearInternalAnnotations(operand)
	if err != nil {
		return operand, err
	}

	out := make([]*yaml.RNode, 0)

	if c.ConsolidateManifests {
		policy, err := c.NewPolicy(c.PolicyName)
		if err != nil {
			return out, err
		}

		for _, rsrc := range operand {
			wrapped, err := c.WrapResource(rsrc)
			if err != nil {
				return out, err
			}

			err = policy.PipeE(
				yaml.LookupCreate(yaml.SequenceNode, "spec", "policy-templates"),
				yaml.Append(wrapped.YNode()),
			)
		}

		out = append(out, policy)

		if c.PlacementSpec.IgnoreExisting || inputPlacement == nil {
			placement, err := c.NewPlacement(c.PolicyName)
			if err != nil {
				return out, err
			}

			out = append(out, placement)
		}

		binding, err := c.NewPlacementBinding(c.PolicyName, []string{c.PolicyName}, inputPlacement)
		if err != nil {
			return out, err
		}

		out = append(out, binding)
	} else {
		policiesToBind := make([]string, len(operand)) // only used if consolidating placements

		for i, rsrc := range operand {
			baseName := fmt.Sprintf("%v-%v", c.PolicyName, i)

			policy, err := c.NewPolicy(baseName)
			if err != nil {
				return out, err
			}

			wrapped, err := c.WrapResource(rsrc)
			if err != nil {
				return out, err
			}

			err = policy.PipeE(
				yaml.LookupCreate(yaml.SequenceNode, "spec", "policy-templates"),
				yaml.Append(wrapped.YNode()),
			)
			if err != nil {
				return out, err
			}

			out = append(out, policy)

			// TODO: need to think about interactions with input Placement and placement.ignoreExisting
			// For now, do something easier
			if !c.ConsolidatePlacements {
				placement, err := c.NewPlacement(baseName)
				if err != nil {
					return out, err
				}

				out = append(out, placement)

				binding, err := c.NewPlacementBinding(baseName, []string{baseName}, nil)
				if err != nil {
					return out, err
				}

				out = append(out, binding)
			} else {
				policiesToBind[i] = baseName // only used if consolidating placements
			}
		}

		if c.ConsolidatePlacements {
			if c.PlacementSpec.IgnoreExisting || inputPlacement == nil {
				placement, err := c.NewPlacement(c.PolicyName)
				if err != nil {
					return out, err
				}

				out = append(out, placement)
			}

			binding, err := c.NewPlacementBinding(c.PolicyName, policiesToBind, inputPlacement)
			if err != nil {
				return out, err
			}

			out = append(out, binding)
		}
	}

	if c.IgnoreNonPolicies { // emit non-policies unchanged
		out = append(out, other...)
	}

	return out, nil
}

func (c PolicyWrapper) WrapResource(res *yaml.RNode) (*yaml.RNode, error) {
	wrapped := yaml.NewMapRNode(nil)

	err := wrapped.PipeE(
		yaml.SetField("objectDefinition", res),
	)

	return wrapped, err
}

const basePolicy = `
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
`

func (c PolicyWrapper) NewPolicy(name string) (*yaml.RNode, error) {
	policy := yaml.MustParse(basePolicy)

	err := policy.SetName(name)
	if err != nil {
		return policy, err
	}

	annos := make(map[string]string)
	annos["policy.open-cluster-management.io/categories"] = strings.Join(c.Categories, ",")
	annos["policy.open-cluster-management.io/controls"] = strings.Join(c.Controls, ",")
	annos["policy.open-cluster-management.io/standards"] = strings.Join(c.Standards, ",")
	policy.SetAnnotations(annos)

	if c.Disabled {
		err := policy.PipeE(
			yaml.LookupCreate(yaml.MappingNode, "spec"),
			yaml.SetField("disabled", yaml.NewScalarRNode("true")),
		)
		if err != nil {
			return policy, err
		}
	}

	if c.RemediationAction != "" {
		err := policy.PipeE(
			yaml.LookupCreate(yaml.MappingNode, "spec"),
			yaml.SetField("remediationAction", yaml.NewScalarRNode(c.RemediationAction)),
		)
		if err != nil {
			return policy, err
		}
	}

	return policy, nil
}

const basePlacement = `
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
spec:
  predicates: []
`

const basePlacementPredicate = `
requiredClusterSelector:
  labelSelector:
    matchExpressions: []
`

const basePlacementRule = `
apiVersion: apps.open-cluster-management.io/v1
kind: PlacementRule
spec:
  clusterSelector:
    matchExpressions: []
`

func (c PolicyWrapper) NewPlacement(name string) (*yaml.RNode, error) {
	var placement *yaml.RNode

	if len(c.PlacementSpec.ClusterSelectors) != 0 {
		placement = yaml.MustParse(basePlacementRule)

		exprs, err := BuildMatchExpressions(c.PlacementSpec.ClusterSelectors)
		if err != nil {
			return nil, err
		}

		for _, expr := range exprs {
			err = placement.PipeE(
				yaml.LookupCreate(yaml.SequenceNode, "spec", "clusterSelector", "matchExpressions"),
				yaml.Append(expr.YNode()),
			)
			if err != nil {
				return nil, err
			}

		}
	} else {
		predicate := yaml.MustParse(basePlacementPredicate)

		exprs, err := BuildMatchExpressions(c.PlacementSpec.LabelSelector)
		if err != nil {
			return nil, err
		}

		for _, expr := range exprs {
			err = predicate.PipeE(
				yaml.LookupCreate(yaml.SequenceNode, "requiredClusterSelector", "labelSelector", "matchExpressions"),
				yaml.Append(expr.YNode()),
			)
			if err != nil {
				return nil, err
			}
		}

		placement = yaml.MustParse(basePlacement)

		err = placement.PipeE(
			yaml.LookupCreate(yaml.SequenceNode, "spec", "predicates"),
			yaml.Append(predicate.YNode()),
		)
		if err != nil {
			return nil, err
		}
	}

	placement.SetName("placement-" + name)

	return placement, nil
}

const basePlacementBinding = `
apiVersion: policy.open-cluster-management.io/v1
kind: PlacementBinding
`

func (c PolicyWrapper) NewPlacementBinding(name string, policies []string, placement *yaml.RNode) (*yaml.RNode, error) {
	binding := yaml.MustParse(basePlacementBinding)

	var placementKind, placementGroup *yaml.RNode

	if placement == nil {
		if len(c.PlacementSpec.ClusterSelectors) != 0 {
			placementKind = yaml.NewScalarRNode("PlacementRule")
			placementGroup = yaml.NewScalarRNode("apps.open-cluster-management.io")
		} else {
			placementKind = yaml.NewScalarRNode("Placement")
			placementGroup = yaml.NewScalarRNode("cluster.open-cluster-management.io")
		}
	} else {
		placementKind = yaml.NewScalarRNode(placement.GetKind())
		placementGroup = yaml.NewScalarRNode(strings.Split(placement.GetApiVersion(), "/")[0])
	}

	err := binding.PipeE(
		yaml.LookupCreate(yaml.MappingNode, "placementRef"),
		yaml.Tee(yaml.SetField("name", yaml.NewScalarRNode("placement-"+name))),
		yaml.Tee(yaml.SetField("kind", placementKind)),
		yaml.Tee(yaml.SetField("apiGroup", placementGroup)),
	)
	if err != nil {
		return binding, err
	}

	for _, policy := range policies {
		subject := yaml.NewMapRNode(nil)

		err = subject.PipeE(
			yaml.Tee(yaml.SetField("name", yaml.NewScalarRNode(policy))),
			yaml.Tee(yaml.SetField("kind", yaml.NewScalarRNode("Policy"))),
			yaml.Tee(yaml.SetField("apiGroup", yaml.NewScalarRNode("policy.open-cluster-management.io"))),
		)
		if err != nil {
			return binding, err
		}

		err = binding.PipeE(
			yaml.LookupCreate(yaml.SequenceNode, "subjects"),
			yaml.Append(subject.YNode()),
		)
		if err != nil {
			return binding, err
		}
	}

	binding.SetName("binding-" + name)
	return binding, nil
}

func BuildMatchExpressions(sel map[string]string) ([]*yaml.RNode, error) {
	list := make([]*yaml.RNode, 0)

	for key, val := range sel {

		item := yaml.NewMapRNode(nil)

		var err error
		if val != "" {
			err = item.PipeE(
				yaml.Tee(yaml.SetField("key", yaml.NewScalarRNode(key))),
				yaml.Tee(yaml.SetField("operator", yaml.NewScalarRNode(`In`))),
				yaml.LookupCreate(yaml.SequenceNode, "values"),
				yaml.Append(yaml.NewScalarRNode(val).YNode()),
			)
		} else {
			err = item.PipeE(
				yaml.Tee(yaml.SetField("key", yaml.NewScalarRNode(key))),
				yaml.Tee(yaml.SetField("operator", yaml.NewScalarRNode(`Exists`))),
				yaml.LookupCreate(yaml.SequenceNode, "values"),
			)
		}
		if err != nil {
			return list, err
		}

		list = append(list, item)
	}

	return list, nil
}

func Split(operand []*yaml.RNode) (policies, other []*yaml.RNode, placement *yaml.RNode) {
	policies = make([]*yaml.RNode, 0)
	other = make([]*yaml.RNode, 0)

	// Separate policy objects from non-policies
	for _, obj := range operand {
		if obj.GetApiVersion() != "policy.open-cluster-management.io/v1" {
			other = append(other, obj)
			continue
		}

		if !strings.HasSuffix(obj.GetKind(), "Policy") {
			other = append(other, obj)
			continue
		}

		policies = append(policies, obj)
	}

	// Find the first Placement[Rule] object and return
	for _, obj := range other {
		apiV := obj.GetApiVersion()

		if strings.HasPrefix("cluster.open-cluster-management.io", apiV) && obj.GetKind() == "Placement" {
			return policies, other, obj
		}

		if apiV == "apps.open-cluster-management.io/v1" && obj.GetKind() == "PlacementRule" {
			return policies, other, obj
		}
	}

	// No Placement[Rule] found
	return policies, other, nil
}
