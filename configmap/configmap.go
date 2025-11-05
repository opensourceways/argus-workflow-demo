package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// GetConfigMap 获取指定 namespace 下的 ConfigMap
func GetConfigMap(clientset *kubernetes.Clientset, namespace, configMapName string) (*corev1.ConfigMap, error) {
	configMap, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s in namespace %s: %v", configMapName, namespace, err)
	}
	return configMap, nil
}

// GetConfigMapValue 获取 ConfigMap 中指定 key 的值
func GetConfigMapValue(clientset *kubernetes.Clientset, namespace, configMapName, key string) (string, error) {
	configMap, err := GetConfigMap(clientset, namespace, configMapName)
	if err != nil {
		return "", err
	}

	value, exists := configMap.Data[key]
	if !exists {
		return "", fmt.Errorf("key %s not found in ConfigMap %s", key, configMapName)
	}

	return value, nil
}

func main() {
	// Define command-line flags
	kubeconfig := flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	namespace := flag.String("namespace", "argo", "The namespace of the ConfigMap")
	configMapName := flag.String("configmap", "workflow-artifact-repository", "The name of the ConfigMap")
	key := flag.String("key", "artifact-repository", "The key to retrieve from the ConfigMap")
	help := flag.Bool("help", false, "Display help information")

	flag.Parse()

	// Display help if requested
	if *help {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(0)
	}

	// Determine kubeconfig path
	kubeconfigPath := *kubeconfig
	if kubeconfigPath == "" {
		// Try to use the default path if not specified
		if home := clientcmd.RecommendedHomeFile; home != "" {
			kubeconfigPath = home
		}
	}

	// Load kubeconfig file
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		log.Fatalf("Failed to load kubeconfig from %s: %v", kubeconfigPath, err)
	}

	// Create Kubernetes client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Get the entire ConfigMap
	configMap, err := GetConfigMap(clientset, *namespace, *configMapName)
	if err != nil {
		log.Fatalf("Error getting ConfigMap: %v", err)
	}

	fmt.Printf("ConfigMap '%s' in namespace '%s':\n", *configMapName, *namespace)
	for key, value := range configMap.Data {
		fmt.Printf("  %s: %s\n", key, value)
	}

	// Get specific key value
	value, err := GetConfigMapValue(clientset, *namespace, *configMapName, *key)
	if err != nil {
		log.Printf("Error getting key '%s': %v", *key, err)
	} else {
		fmt.Printf("\nValue of key '%s': %s\n", *key, value)
	}
}
