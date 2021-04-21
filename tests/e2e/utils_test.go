package e2e

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"io"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
	"net/http"
	"net/url"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"

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

func waitForPodsReadiness(namespace string, labels map[string]string) {
	Eventually(func() bool {
		podList := &v1.PodList{}
		err = restClient.List(context.Background(), podList, client.InNamespace(namespace), client.MatchingLabels(labels))
		Expect(err).To(Succeed())

		if len(podList.Items) == 0 {
			return false
		}

		for _, pod := range podList.Items {
			for _, container := range pod.Status.ContainerStatuses {
				if !container.Ready {
					return false
				}
			}
		}
		return true
	}, time.Minute*10, time.Second*2).Should(BeTrue(), "Pods should become ready: ", labels[v1alpha1.CassandraClusterComponent])
}

func waitPodsTermination(namespace string, labels map[string]string) {
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

		return "Incorrect. Schema still exit."
	}, time.Second*5).Should(Equal(metav1.StatusReasonNotFound), "Cassandra cluster CassandraCluster schema should be deleted")
}

func portForwardPod(namespace string, labels map[string]string, portMappings []string) *portforward.PortForwarder {
	rt, upgrader, err := spdy.RoundTripperFor(restClientConfig)
	Expect(err).To(Succeed())

	podList := &v1.PodList{}
	err = restClient.List(context.Background(), podList, client.InNamespace(namespace), client.MatchingLabels(labels))
	Expect(err).To(Succeed())
	Expect(len(podList.Items)).ToNot(BeEquivalentTo(0), "pods not found for port forwarding")

	pod := podList.Items[0]
	fmt.Printf("pod.Name: %v", pod.Name)
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
		err = pf.ForwardPorts() // this will block until stopped
		Expect(err).To(Succeed())
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
	defer resp.Body.Close()

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

func findFirstElemByKey(m map[string]interface{}, key string) interface{} {
	for k, v := range m {
		if k == key {
			return v
		}
	}
	return nil
}

func testReaperRescheduleTime(reqTime time.Time, respTime time.Time, nowTime time.Time) {
	Expect(respTime.Weekday()).To(BeEquivalentTo(reqTime.Weekday()), "Week day should match.")

	if respTime.Year() == reqTime.Year() { // There is a case when schedules set with previous year
		Expect(respTime.YearDay()).To(BeNumerically(">=", reqTime.YearDay()), "Year day should be equal or greater than scheduled.")
		Expect(respTime.YearDay()).To(BeNumerically(">=", nowTime.YearDay()), "Year day should be equal or greater than current.")
	}

	Expect(respTime.Unix()).To(BeNumerically(">=", respTime.Unix()), "Unix timestamp should be greater or equal scheduled.")
	Expect(respTime.Unix()).To(BeNumerically(">=", nowTime.Unix()), "Unix timestamp should be greater or equal current.")
	Expect(respTime.YearDay()-reqTime.YearDay()).To(BeNumerically("<=", 7), "Year day difference should be less or equal 7.")
}

func getPodLogs(pod v1.Pod, podLogOpts v1.PodLogOptions) (string, error) {
	req := kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	podLogs, err := req.Stream(context.Background())
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	buff := new(bytes.Buffer)
	_, err = io.Copy(buff, podLogs)
	if err != nil {
		return "", err
	}
	str := buff.String()

	return str, err
}

func readFile(file string) ([]byte, error) {
	content, err := ioutil.ReadFile(file)
	return content, err
}

func execPod(podName string, namespace string, cmd []string) ExecResult {
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
	Expect(err).ToNot(HaveOccurred())

	return ExecResult{
		stdout: fmt.Sprint(stdout),
		stderr: fmt.Sprint(stderr),
	}
}

func deployCassandraCluster(cassandraCluster *v1alpha1.CassandraCluster) {
	By("Deploy cassandra cluster and all of its components")
	Expect(restClient).ToNot(BeNil())

	By("Cassandra cluster is starting...")
	Expect(restClient.Create(context.Background(), cassandraCluster)).To(Succeed())

	By("Checking prober pods readiness...")
	waitForPodsReadiness(cassandraCluster.Namespace, proberPodLabels)

	By("Checking cassandra cluster pods readiness...")
	for _, dc := range cassandraCluster.Spec.DCs {
		waitForPodsReadiness(cassandraCluster.Namespace, labels.WithDCLabel(cassandraClusterPodLabels, dc.Name))
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
