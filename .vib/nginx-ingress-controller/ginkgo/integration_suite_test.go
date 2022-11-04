package integration

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	netcv1 "k8s.io/client-go/kubernetes/typed/networking/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// For client auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const APP_NAME = "NGINX Ingress Controller"

var kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
var namespace = flag.String("namespace", "", "namespace where the resources are deployed")
var svcName = flag.String("service-name", "", "service name used to serve the kuard deployment")

func clusterConfigOrDie() *rest.Config {
	var config *rest.Config
	var err error

	if *kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		panic(err.Error())
	}

	return config
}

func createIngressOrDie(ctx context.Context, c netcv1.NetworkingV1Interface, name string, ingressRuleHost string, ingressRuleValue netv1.IngressRuleValue) {
	ingressClassName := "nginx"

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: *namespace,
			Name:      name,
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []netv1.IngressRule{
				{
					Host:             ingressRuleHost,
					IngressRuleValue: ingressRuleValue,
				},
			},
		},
	}

	result, err := c.Ingresses(*namespace).Create(ctx, ingress, metav1.CreateOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("Created ingress %q.\n", result.GetObjectMeta().GetName())
}

func getResponseBodyOrDie(ctx context.Context, address string) []string {
	var output []string
	var client http.Client

	resp, err := client.Get(address)
	if err != nil {
		panic(fmt.Sprintf("There was an error during the GET request: %q", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		scanner := bufio.NewScanner(interruptableReader{ctx, resp.Body})
		for scanner.Scan() {
			output = append(output, scanner.Text())
		}
		if scanner.Err() != nil {
			panic(scanner.Err())
		}
	}
	return output
}

func hasIPAssigned(ctx context.Context, c netcv1.NetworkingV1Interface, resourceName string) (bool, error) {
	var err error

	ingress, err := c.Ingresses(*namespace).Get(ctx, resourceName, metav1.GetOptions{})

	if len(ingress.Status.LoadBalancer.Ingress) > 0 {
		return true, err
	} else {
		return false, err
	}
}

func retry(name string, attempts int, sleep time.Duration, f func() (bool, error)) (res bool, err error) {
	for i := 0; i < attempts; i++ {
		fmt.Printf("[retriable] operation %q executing now [attempt %d/%d]\n", name, (i + 1), attempts)
		res, err = f()
		if res {
			fmt.Printf("[retriable] operation %q succedeed [attempt %d/%d]\n", name, (i + 1), attempts)
			return res, err
		}
		fmt.Printf("[retriable] operation %q failed, sleeping for %q now...\n", name, sleep)
		time.Sleep(sleep)
	}
	fmt.Printf("[retriable] operation %q failed [attempt %d/%d]\n", name, attempts, attempts)
	return res, err
}

type interruptableReader struct {
	ctx context.Context
	r   io.Reader
}

func (r interruptableReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if err != nil {
		return n, err
	}
	return n, r.ctx.Err()
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func CheckRequirements() {
	if *namespace == "" {
		panic(fmt.Sprintf("The namespace where %s is deployed must be provided. Use the '--namespace' flag", APP_NAME))
	}
	if *svcName == "" {
		panic(fmt.Sprintln("The testing service name used to serve the kuard deployment must be provided. Use the '--service-name' flag"))
	}
}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	CheckRequirements()
	RunSpecs(t, fmt.Sprintf("%s Integration Tests", APP_NAME))
}
