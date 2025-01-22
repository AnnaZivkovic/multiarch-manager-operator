package framework

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/openshift/multiarch-tuning-operator/apis/multiarch/v1alpha1"
	"github.com/openshift/multiarch-tuning-operator/apis/multiarch/v1beta1"
)

func DecorateWithWaitGroup(wg *sync.WaitGroup, f func()) {
	wg.Add(1)
	go func() {
		defer GinkgoRecover()
		f()
		wg.Done()
	}()
}

func StartChannelMonitor(ch chan error, descr string) {
	go func() {
		defer GinkgoRecover()
		err := <-ch
		Expect(err).NotTo(HaveOccurred(), descr, err)
	}()
}

func EnsureNamespaces(ctx context.Context, client client.Client, namespaces ...string) {
	var err error
	for _, ns := range namespaces {
		namespace := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		}
		err = client.Create(ctx, namespace)
		Expect(err).NotTo(HaveOccurred())
	}
}

// LoadClient returns a new controller-runtime client.
func LoadClient() (client.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	return client.New(cfg, client.Options{})
}

// LoadClientset returns a new Kubernetes Clientset.
func LoadClientset() (*kubernetes.Clientset, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(cfg)
}

func RegisterScheme(s *runtime.Scheme) error {
	var errs []error
	errs = append(errs, admissionv1.AddToScheme(s))
	errs = append(errs, corev1.AddToScheme(s))
	errs = append(errs, appsv1.AddToScheme(s))
	errs = append(errs, v1alpha1.AddToScheme(s))
	errs = append(errs, v1beta1.AddToScheme(s))
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

func WriteToFile(dir, fileName, content string) error {
	if dir == "" {
		return fmt.Errorf("directory path is empty")
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return err
		}
	}
	filePath := fmt.Sprintf("%s/%s", dir, fileName)
	file, err := os.Create(filepath.Clean(filePath))
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing file: %v", err)
		}
	}()
	log.Printf("Writing content to file %s", filePath)
	_, err = file.WriteString(content)
	if err != nil {
		return fmt.Errorf("Failed to write content to file: %w", err)
	}
	return nil
}

func GetReplacedImageURI(image, replacedRegistry string) string {
	index := strings.Index(image, "/")
	if index == -1 {
		return image
	}
	return replacedRegistry + image[index:]
}

func GetImageRepository(image string) string {
	colonIndex := strings.LastIndex(image, ":")
	if colonIndex != -1 {
		image = image[:colonIndex]
	}
	return image
}
