package framework

import (
	"bytes"
	"context"
	"fmt"
	"log"

	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

func VerifyPodsAreRunning(g Gomega, ctx context.Context, client runtimeclient.Client, ns *v1.Namespace, labelKey string, labelInValue string) {
	r, err := labels.NewRequirement(labelKey, selection.In, []string{labelInValue})
	labelSelector := labels.NewSelector().Add(*r)
	g.Expect(err).NotTo(HaveOccurred())
	pods := &v1.PodList{}
	err = client.List(ctx, pods, &runtimeclient.ListOptions{
		Namespace:     ns.Name,
		LabelSelector: labelSelector,
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pods.Items).NotTo(BeEmpty())
	g.Expect(pods.Items).Should(HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase {
		return p.Status.Phase
	}, Equal(v1.PodRunning))))
}

func GetPodsWithLabel(ctx context.Context, client runtimeclient.Client, namespace, labelKey, labelInValue string) (*v1.PodList, error) {
	r, err := labels.NewRequirement(labelKey, "in", []string{labelInValue})
	labelSelector := labels.NewSelector().Add(*r)
	Expect(err).NotTo(HaveOccurred())
	pods := &v1.PodList{}
	err = client.List(ctx, pods, &runtimeclient.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pods found with label %s=%s in namespace %s", labelKey, labelInValue, namespace)
	}
	return pods, nil
}

func GetPodLog(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName, containerName string) (string, error) {
	podLogOpts := v1.PodLogOptions{
		Container: containerName,
	}
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get logs for pod %s: %w", podName, err)
	}
	defer func() {
		if err := podLogs.Close(); err != nil {
			log.Printf("Error closing logs for pod %s: %v", podName, err)
		}
	}()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(podLogs)
	if err != nil {
		return "", fmt.Errorf("failed to read logs for pod %s: %w", podName, err)
	}

	return buf.String(), nil
}

func StorePodsLog(ctx context.Context, clientset *kubernetes.Clientset, client runtimeclient.Client, namespace, labelKey, labelInValue, containerName, dir string) error {
	pods, err := GetPodsWithLabel(ctx, client, namespace, labelKey, labelInValue)
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		log.Printf("Getting logs for pod %s", pod.Name)
		logs, err := GetPodLog(ctx, clientset, utils.Namespace(), pod.Name, containerName)
		if err != nil {
			log.Printf("Failed to get logs for pod %s: %v", pod.Name, err)
			continue
		}
		err = WriteToFile(dir, fmt.Sprintf("%s.log", pod.Name), logs)
		if err != nil {
			log.Printf("Failed to write logs to file: %v", err)
			return err
		} else {
			return nil
		}
	}
	return nil
}