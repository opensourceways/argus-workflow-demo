package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/argoproj/argo-workflows/v3/pkg/apiclient"
	workflowpkg "github.com/argoproj/argo-workflows/v3/pkg/apiclient/workflow"
	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	// 创建 API 客户端
	ctx, client, err := createArgoClient()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// 创建工作流
	workflow := createSampleWorkflow()

	// 提交工作流
	workflowClient := client.NewWorkflowServiceClient()
	createdWf, err := workflowClient.CreateWorkflow(ctx, &workflowpkg.WorkflowCreateRequest{
		Namespace: "default",
		Workflow:  workflow,
	})

	if err != nil {
		log.Fatalf("Failed to create workflow: %v", err)
	}

	fmt.Printf("工作流创建成功: %s\n", createdWf.Name)
	fmt.Printf("状态: %s\n", createdWf.Status.Phase)
}

func createArgoClient() (context.Context, apiclient.Client, error) {
	// 方法一：使用 kubeconfig 直接连接 Kubernetes 集群
	var kubeconfig *string

	// 方式1: 自动发现默认路径 [citation:9]
	// if home := homedir.HomeDir(); home != "" {
	// 	kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	// } else {
	// 	kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	// }

	// 方式2: 通过绝对路径直接指定 [citation:9]
	kubeconfig = flag.String("kubeconfig", "/home/k9s/argo/openmerlin-guiyang-006-cluster-kubeconfig.yaml", "absolute path to the kubeconfig file")

	// 方式3: 通过环境变量指定 [citation:5]
	// 你可以在运行程序前，在终端执行: export KUBECONFIG=/your/path/kubeconfig
	// envKubeconfig := os.Getenv("KUBECONFIG")
	// if envKubeconfig != "" {
	// 	kubeconfig = &envKubeconfig
	// }

	flag.Parse()

	// 使用 kubeconfig 配置
	opts := apiclient.Opts{
		ClientConfigSupplier: func() clientcmd.ClientConfig {
			loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
			if kubeconfig != nil && *kubeconfig != "" {
				loadingRules.ExplicitPath = *kubeconfig
			}
			return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				loadingRules,
				&clientcmd.ConfigOverrides{},
			)
		},
	}

	return apiclient.NewClientFromOpts(opts)
}

func createSampleWorkflow() *wfv1.Workflow {
	return &wfv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "hello-world-",
			Namespace:    "argo",
		},
		Spec: wfv1.WorkflowSpec{
			Entrypoint: "hello-world",
			ImagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: "huawei-swr-image-pull-secret-model-gy",
				},
			},
			Templates: []wfv1.Template{
				{
					Name: "hello-world",
					Container: &corev1.Container{
						Image:   "swr.cn-southwest-2.myhuaweicloud.com/modelfoundry/git:latest",
						Command: []string{"sh", "-c"},
						Args:    []string{"echo 'Hello World from Golang SDK!'"},
					},
				},
			},
		},
	}
}
