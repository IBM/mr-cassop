package e2e

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"io"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
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

func waitForPodsReadiness(namespace string, labels map[string]string) {
	Eventually(func() bool {
		podList := &corev1.PodList{}
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
		podList := &corev1.PodList{}
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

func portForwardPod(namespace string, labels map[string]string, portMappings []string, restClientConfig *rest.Config) *portforward.PortForwarder {
	rt, upgrader, err := spdy.RoundTripperFor(restClientConfig)
	Expect(err).To(Succeed())

	podList := &corev1.PodList{}
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
			Fail(fmt.Sprintf("Port forwarding failed: %s", errOut.String()))
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

func doHTTPRequest(method string, url string) (error, []byte, int) {

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return err, nil, 0
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err, nil, 0
	}
	defer resp.Body.Close()

	// Parse response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err, nil, 0
	}

	return err, body, resp.StatusCode
}

func findFirstRepairMapByKV(repair []map[string]interface{}, k string, v string) map[string]interface{} {
	for _, iterMap := range repair {
		// This works only if the key has value of type []interface(array in term of json) and we take only the first its element. That's why you can't specify more than 1 table per reaper repair in e2e tests.
		if i := iterMap[k].([]interface{})[0]; i == v { // Extract string value from slice of interfaces in unstructured json
			return iterMap
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

func getPodLogs(pod corev1.Pod, kubeClient *kubernetes.Clientset, podLogOpts corev1.PodLogOptions) (error, string) {
	req := kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	podLogs, err := req.Stream(context.Background())
	if err != nil {
		return err, ""
	}
	defer podLogs.Close()

	buff := new(bytes.Buffer)
	_, err = io.Copy(buff, podLogs)
	if err != nil {
		return err, ""
	}
	str := buff.String()

	return err, str
}

func readFile(file string) []byte {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		log.Error(err, "Cannot read file ../../cassandra-operator/values.yaml")
	}
	return content
}
