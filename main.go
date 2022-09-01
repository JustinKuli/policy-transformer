package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var baseConfigMap = []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: game-config-aliens
  namespace: default
data: {}
`)

// func main() {
// 	cfg := make(map[string]interface{})
// 	cfg["my-default"] = "hello world"

// 	f := kio.FilterFunc(func(operand []*yaml.RNode) ([]*yaml.RNode, error) {
// 		out := make([]*yaml.RNode, len(operand))

// 		cfgRN, err := yaml.FromMap(cfg)
// 		if err != nil {
// 			return nil, err
// 		}

// 		for i, inp := range operand {
// 			newnodes, err := kio.FromBytes(baseConfigMap)
// 			if err != nil {
// 				return nil, err
// 			}

// 			data := make(map[string]string)
// 			data["contents"] = inp.MustString()
// 			data["config"] = cfgRN.MustString()
// 			// data["args"] = strings.Join(os.Args, "::")

// 			node := newnodes[0]
// 			node.SetName(fmt.Sprintf("input-%v", i))
// 			node.LoadMapIntoConfigMapData(data)

// 			out[i] = node
// 		}

// 		return out, nil
// 	})

// 	proc := framework.SimpleProcessor{Filter: f, Config: &cfg}

// 	err := framework.Execute(proc, nil)
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// }

func main() {
	inp, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	inpReader := bytes.NewReader(inp)

	cfg := make(map[string]interface{})
	cfg["my-default"] = "hello world"

	f := kio.FilterFunc(func(operand []*yaml.RNode) ([]*yaml.RNode, error) {
		cfgNode, err := kio.FromBytes(baseConfigMap)
		if err != nil {
			log.Fatal(err)
		}

		cfgRN, err := yaml.FromMap(cfg)
		if err != nil {
			log.Fatal(err)
		}

		cfgString := cfgRN.MustString()

		data := make(map[string]string)
		data["input"] = string(inp)
		data["config"] = cfgString
		data["args"] = strings.Join(os.Args, "::")

		cfgNode[0].LoadMapIntoConfigMapData(data)

		operand = append(operand, cfgNode[0])

		return operand, nil
		// out := make([]*yaml.RNode, len(operand))

		// cfgRN, err := yaml.FromMap(cfg)
		// if err != nil {
		// 	return nil, err
		// }

		// cfgString := cfgRN.MustString()

		// for i, inp := range operand {
		// 	newnodes, err := kio.FromBytes(baseConfigMap)
		// 	if err != nil {
		// 		return nil, err
		// 	}

		// 	data := make(map[string]string)
		// 	data["contents"] = inp.MustString()
		// 	data["config"] = cfgString
		// 	data["args"] = strings.Join(os.Args, "::")

		// 	node := newnodes[0]
		// 	node.SetName(fmt.Sprintf("input-%v", i))
		// 	node.LoadMapIntoConfigMapData(data)

		// 	out[i] = node
		// }

		// return out, nil
	})

	proc := framework.SimpleProcessor{Filter: f, Config: &cfg}

	rlSrc := &kio.ByteReadWriter{
		Reader:                inpReader,
		Writer:                os.Stdout,
		KeepReaderAnnotations: true,
	}
	err = framework.Execute(proc, rlSrc)
	if err != nil {
		log.Fatal(err)
	}
}
