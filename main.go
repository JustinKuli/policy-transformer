package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const fakeNode = `
apiVersion: fake/v1
kind: FakeNode
metadata:
  name: foo
`

type PolicyTransformer struct {
	Config *TransfomerConfig
}

type TransfomerConfig struct {
	// ResourceMeta has APIVersion, Kind, and a subset of the k8s metadata fields
	yaml.ResourceMeta `json:",inline" yaml:",inline"`

	InputResourceList []byte
	CommandArgs       []string
	LsOut             []byte
}

func (t PolicyTransformer) Filter(operand []*yaml.RNode) ([]*yaml.RNode, error) {
	out := make([]*yaml.RNode, len(operand))

	for i, inp := range operand {
		err := inp.SetAnnotations(map[string]string{
			"jkulikau.io/kind": fmt.Sprint(t.Config.Kind),
		})
		if err != nil {
			return out, err
		}

		out[i] = inp
	}

	rsrc, err := yaml.Parse(fakeNode)
	if err != nil {
		return out, err
	}

	err = rsrc.SetAnnotations(map[string]string{
		"jkulikau.io/function-config-annotation": t.Config.Annotations["config.kubernetes.io/function"],
		"jkulikau.io/input-debug":                string(t.Config.InputResourceList),
		"jkulikau.io/args":                       strings.Join(t.Config.CommandArgs, "::"),
		"jkulikau.io/config-cat":                 string(t.Config.LsOut),
	})
	if err != nil {
		return out, err
	}

	out = append(out, rsrc)

	return out, nil
}

func main() {
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	var lsOut []byte

	if len(os.Args) > 1 {
		lsOut, err = exec.Command("cat", os.Args[1]).CombinedOutput()
	}

	cfg := TransfomerConfig{
		// Set defaults here?
		InputResourceList: stdin,
		CommandArgs:       os.Args,
		LsOut:             lsOut,
	}

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
