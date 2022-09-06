package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type PolicyTransformer struct {
	Config *TransfomerConfig
}

type TransfomerConfig struct {
	// ResourceMeta has APIVersion, Kind, and a subset of the k8s metadata fields
	yaml.ResourceMeta `json:",inline" yaml:",inline"`

	Spec map[string]interface{}
}

func (t PolicyTransformer) Filter(operand []*yaml.RNode) ([]*yaml.RNode, error) {
	configSpec, err := json.Marshal(t.Config.Spec)
	if err != nil {
		return operand, err
	}

	var transformer kio.Filter

	switch t.Config.Kind {
	case "ConfigurationPolicyWrapper":
		w := NewConfigurationPolicyWrapper()

		err = json.Unmarshal(configSpec, &w)
		if err != nil {
			return operand, err
		}

		w.PolicyName = t.Config.Name

		transformer = w
	case "PolicyWrapper":
		w := NewPolicyWrapper()

		err = json.Unmarshal(configSpec, &w)
		if err != nil {
			return operand, err
		}

		w.PolicyName = t.Config.Name

		transformer = w
	default:
		return operand, fmt.Errorf("unknown PolicyTransformer kind '%v'", t.Config.Kind)
	}

	return transformer.Filter(operand)
}

func ClearInternalAnnotations(operand []*yaml.RNode) ([]*yaml.RNode, error) {
	for _, rsrc := range operand {
		internalAnnos := kioutil.GetInternalAnnotations(rsrc)
		for key := range internalAnnos {
			_, err := yaml.ClearAnnotation(key).Filter(rsrc)
			if err != nil {
				return operand, err
			}
		}

		// one more annotation that isn't in `GetInternalAnnotations`
		_, err := yaml.ClearAnnotation("kustomize.config.k8s.io/id").Filter(rsrc)
		if err != nil {
			return operand, err
		}
	}

	return operand, nil
}

func main() {
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	cfg := TransfomerConfig{}

	proc := framework.SimpleProcessor{Filter: PolicyTransformer{Config: &cfg}, Config: &cfg}

	err = framework.Execute(proc, &kio.ByteReadWriter{
		Reader:                bytes.NewReader(stdin),
		Writer:                os.Stdout,
		KeepReaderAnnotations: true,
	})
	if err != nil {
		log.Fatal(err)
	}
}
