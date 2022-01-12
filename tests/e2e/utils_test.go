package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/controller-runtime/pkg/client"

	prometheusClient "github.com/prometheus/client_model/go"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	httpClient = &http.Client{
		Timeout: time.Second * 5,
	}
)

type ExecResult struct {
	stdout string
	stderr string
}

func waitForPodsReadiness(namespace string, labels map[string]string, expectedNumberOfPods int32) {
	Eventually(func() (bool, error) {
		podList := &v1.PodList{}
		err = restClient.List(context.Background(), podList, client.InNamespace(namespace), client.MatchingLabels(labels))
		if err != nil {
			return false, err
		}

		if len(podList.Items) == 0 || (expectedNumberOfPods != 0 && int32(len(podList.Items)) != expectedNumberOfPods) {
			return false, nil
		}

		for _, pod := range podList.Items {
			if len(pod.Status.ContainerStatuses) == 0 {
				return false, nil
			}
			for _, container := range pod.Status.ContainerStatuses {
				if !container.Ready {
					return false, nil
				}
			}
		}
		return true, nil
	}, time.Minute*15, time.Second*2).Should(BeTrue(), fmt.Sprintf("Pods should become ready: %s", labels[v1alpha1.CassandraClusterComponent]))
}

func waitForPodsTermination(namespace string, labels map[string]string) {
	Eventually(func() bool {
		podList := &v1.PodList{}
		err = restClient.List(context.Background(), podList, client.InNamespace(namespace), client.MatchingLabels(labels))
		Expect(err).To(Succeed())

		return len(podList.Items) == 0 //return true if expression is true
	}, time.Minute*10, time.Second*5).Should(BeTrue())
}

func waitForCassandraClusterSchemaDeletion(namespace string, release string) {
	Eventually(func() metav1.StatusReason {
		err = restClient.Get(context.Background(), types.NamespacedName{
			Namespace: namespace,
			Name:      release,
		}, &v1alpha1.CassandraCluster{})

		if err != nil {
			if statusErr, ok := err.(*errors.StatusError); ok {
				return statusErr.ErrStatus.Reason
			}
		}

		return "Incorrect. Cassandracluster resource still exit."
	}, time.Second*5).Should(Equal(metav1.StatusReasonNotFound), "Cassandra cluster CassandraCluster resource should be deleted")
}

func portForwardPod(namespace string, labels map[string]string, portMappings []string) *portforward.PortForwarder {
	rt, upgrader, err := spdy.RoundTripperFor(restClientConfig)
	Expect(err).To(Succeed())

	podList := &v1.PodList{}
	err = restClient.List(context.Background(), podList, client.InNamespace(namespace), client.MatchingLabels(labels))
	Expect(err).To(Succeed())
	Expect(len(podList.Items)).ToNot(BeEquivalentTo(0), "pods not found for port forwarding")

	pod := podList.Items[0]
	fmt.Printf("port forwarding pod: %s\n", pod.Name)
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", pod.Namespace, pod.Name)
	hostIP := strings.TrimLeft(restClientConfig.Host, "htps:/")
	serverURL := url.URL{Scheme: "https", Path: path, Host: hostIP}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: rt}, http.MethodPost, &serverURL)

	stopChan, readyChan := make(chan struct{}, 1), make(chan struct{}, 1)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	pf, err := portforward.New(dialer, portMappings, stopChan, readyChan, out, errOut)
	Expect(err).To(Succeed())

	go func() {
		defer GinkgoRecover()
		for range readyChan {
		}

		if len(errOut.String()) != 0 {
			Fail(fmt.Sprintf("Port forwarding failed. Error: %s", errOut.String()))
		} else if len(out.String()) != 0 {
			_, err := fmt.Fprintf(GinkgoWriter, "Message from port forwarder: %s", out.String())
			Expect(err).To(Succeed())
		}
	}()

	go func() {
		defer GinkgoRecover()
		Expect(pf.ForwardPorts()).To(Succeed())
	}()

	for range readyChan {
	} // wait till ready

	return pf
}

func doHTTPRequest(method string, url string) ([]byte, int, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	return body, resp.StatusCode, err
}

func findFirstMapByKV(repair []map[string]interface{}, k string, v []string) map[string]interface{} {
	for _, iterMap := range repair {
		str1 := fmt.Sprintf("%v", iterMap[k].([]interface{}))
		str2 := fmt.Sprintf("%v", v)
		if str1 == str2 {
			return iterMap
		}
	}
	return nil
}

func getPodLogs(pod v1.Pod, podLogOpts v1.PodLogOptions) (string, error) {
	req := kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	podLogs, err := req.Stream(context.Background())
	if err != nil {
		return "", err
	}
	defer func() { _ = podLogs.Close() }()

	buff := new(bytes.Buffer)
	_, err = io.Copy(buff, podLogs)
	if err != nil {
		return "", err
	}
	str := buff.String()

	return str, err
}

func execPod(podName string, namespace string, cmd []string) (ExecResult, error) {
	req := kubeClient.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
		Namespace(namespace).SubResource("exec")
	option := &v1.PodExecOptions{
		Command: cmd,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     false,
	}

	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(restClientConfig, "POST", req.URL())
	Expect(err).ToNot(HaveOccurred())

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})

	return ExecResult{
		stdout: fmt.Sprint(stdout),
		stderr: fmt.Sprint(stderr),
	}, err
}

func deployCassandraCluster(cassandraCluster *v1alpha1.CassandraCluster) {
	By("Deploy cassandra cluster and all of its components")
	Expect(restClient).ToNot(BeNil())

	By("Cassandra cluster is starting...")
	Expect(restClient.Create(context.Background(), cassandraCluster)).To(Succeed())

	By("Waiting for prober pods readiness...")
	waitForPodsReadiness(cassandraCluster.Namespace, proberPodLabels, 1)

	By("Waiting for cassandra cluster pods readiness...")
	for _, dc := range cassandraCluster.Spec.DCs {
		waitForPodsReadiness(cassandraCluster.Namespace, labels.WithDCLabel(cassandraClusterPodLabels, dc.Name), *dc.Replicas)
	}
}

func showPodLogs(labels map[string]string) {
	podList := &v1.PodList{}
	err = restClient.List(context.Background(), podList, client.InNamespace(cassandraNamespace), client.MatchingLabels(labels))
	if err != nil {
		fmt.Println("Unable to get pods. Error: ", err)
	}

	for _, pod := range podList.Items {
		fmt.Println("Logs from pod: ", pod.Name)

		for _, container := range pod.Spec.Containers {
			str, err := getPodLogs(pod, v1.PodLogOptions{TailLines: &tailLines, Container: container.Name})
			if err != nil {
				continue
			}
			fmt.Println(str)
		}
	}
}

// getMetrics returns all metrics in the specified metric family with a given metric name
func getMetrics(metricFamilies map[string]*prometheusClient.MetricFamily, metricName string) ([]*prometheusClient.Metric, bool) {
	mf, found := metricFamilies[metricName]
	if found && mf.Name != nil && *mf.Name == metricName {
		if len(mf.Metric) == 0 {
			return []*prometheusClient.Metric{}, false
		}
		return mf.Metric, true
	}
	return []*prometheusClient.Metric{}, false
}

// getMetric returns the first metric in the specified metric family
func getMetric(metricFamilies map[string]*prometheusClient.MetricFamily, metricName string) (prometheusClient.Metric, bool) {
	metrics, found := getMetrics(metricFamilies, metricName)
	if found {
		return *metrics[0], true
	}
	return prometheusClient.Metric{}, false
}

// getMetricByLabel returns the first metric in the specified metric family that has the specified label and label value. The label value parameter is a substring.
func getMetricByLabel(metricFamilies map[string]*prometheusClient.MetricFamily, metricName, labelName, labelValue string) (prometheusClient.Metric, bool) {
	metrics, found := getMetrics(metricFamilies, metricName)
	if found {
		for _, metric := range metrics {
			for _, label := range metric.Label {
				if *label.Name == labelName && strings.Contains(*label.Value, labelValue) {
					return *metric, true
				}
			}
		}
	}
	return prometheusClient.Metric{}, false
}

func prepareNamespace(namespaceName string) {
	ns := &v1.Namespace{}
	err = restClient.Get(context.Background(), types.NamespacedName{Name: namespaceName}, ns)
	if err != nil && errors.IsNotFound(err) {
		ns = &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
		Expect(restClient.Create(context.Background(), ns)).To(Succeed())
	} else {
		Expect(err).ToNot(HaveOccurred())
	}

	copySecret(cassandraNamespace, imagePullSecret, namespaceName)
	copySecret(cassandraNamespace, imagePullSecret, namespaceName)
	copySecret(cassandraNamespace, ingressSecret, namespaceName)
	copySecret(cassandraNamespace, ingressSecret, namespaceName)
}

func copySecret(namespaceFrom, secretName, namespaceTo string) {
	secret := &v1.Secret{}
	err := restClient.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: namespaceTo}, secret)
	if err == nil {
		return
	}

	Expect(restClient.Get(context.Background(), types.NamespacedName{Namespace: namespaceFrom, Name: secretName}, secret)).To(Succeed())
	newSecret := secret.DeepCopy()
	newSecret.Namespace = namespaceTo
	newSecret.ResourceVersion = ""
	newSecret.UID = ""
	delete(newSecret.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
	Expect(restClient.Create(context.Background(), newSecret)).To(Succeed())
}

func createSecret(namespace, name string, data map[string][]byte) {
	err := restClient.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, &v1.Secret{})
	if err == nil {
		return
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: data,
	}
	Expect(restClient.Create(context.Background(), secret)).To(Succeed())
}

func removeNamespaces(namespaces ...string) {
	for _, namespace := range namespaces {
		ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		err := restClient.Get(context.Background(), types.NamespacedName{Name: namespace}, ns)
		if err == nil {
			Expect(restClient.Delete(context.Background(), ns)).To(Succeed())
		}
	}
	By(fmt.Sprintf("Waiting for namespaces %v to be removed", namespaces))
	start := time.Now()

	for _, namespace := range namespaces {
		Eventually(func() error {
			return restClient.Get(context.Background(), types.NamespacedName{Name: namespace}, &v1.Namespace{})
		}, 5*time.Minute, 10*time.Second).ShouldNot(Succeed())
	}
	By(fmt.Sprintf("Namespaces removed after %s", time.Since(start)))
}
