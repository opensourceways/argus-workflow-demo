package main

import (
	"fmt"
	"io/ioutil"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func parseWorkflowFromYAML(yamlFile string) (*wfv1.Workflow, error) {
	// 读取 YAML 文件
	yamlBytes, err := ioutil.ReadFile(yamlFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file: %w", err)
	}

	// 创建 Workflow 对象
	var workflow wfv1.Workflow

	// 解析 YAML 到 Workflow 对象
	if err := yaml.Unmarshal(yamlBytes, &workflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	return &workflow, nil
}

func main() {
	// 解析工作流文件
	wf, err := parseWorkflowFromYAML("linux-arm-npu-a2b4-1.yaml")
	if err != nil {
		panic(err)
	}

	// 现在你可以访问工作流的各种属性
	fmt.Printf("Workflow Name: %s\n", wf.Name)
	fmt.Printf("Namespace: %s\n", wf.Namespace)
	fmt.Printf("Entrypoint: %s\n", wf.Spec.Entrypoint)
}
