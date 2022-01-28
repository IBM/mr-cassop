package e2e

import (
	"bytes"
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

	. "github.com/onsi/ginkgo/v2"
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
	start := time.Now()
	Eventually(func() error {
		podList := &v1.PodList{}
		err := kubeClient.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels(labels))
		if err != nil {
			return err
		}

		if len(podList.Items) == 0 || (expectedNumberOfPods != 0 && int32(len(podList.Items)) != expectedNumberOfPods) {
			return fmt.Errorf("unexpected number of found pods with labels %v: %d, expected %d", labels, len(podList.Items), expectedNumberOfPods)
		}

		for _, pod := range podList.Items {
			podScheduled := false
			for _, condition := range pod.Status.Conditions {
				if condition.Type == v1.PodScheduled {
					if condition.Status == v1.ConditionTrue {
						podScheduled = true
					}
				}
			}
			if !podScheduled {
				return fmt.Errorf("pod %s/%s is not yet scheduled", pod.Namespace, pod.Name)
			}
		}
		return nil
	}, time.Minute*20, time.Second*10).Should(Succeed(), fmt.Sprintf("Pod haven't been scheduled: %s", labels[v1alpha1.CassandraClusterComponent]))
	By(fmt.Sprintf("pods were scheduled in %s", time.Since(start)))

	start = time.Now()
	Eventually(func() error {
		podList := &v1.PodList{}
		err := kubeClient.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels(labels))
		if err != nil {
			return err
		}

		if len(podList.Items) == 0 || (expectedNumberOfPods != 0 && int32(len(podList.Items)) != expectedNumberOfPods) {
			return fmt.Errorf("unexpected number of found pods: %d, expected %d", len(podList.Items), expectedNumberOfPods)
		}

		for _, pod := range podList.Items {
			if len(pod.Status.ContainerStatuses) == 0 {
				return fmt.Errorf("container statuses not found for pod %s/%s", pod.Namespace, pod.Name)

			}
			for _, container := range pod.Status.ContainerStatuses {
				if !container.Ready {
					return fmt.Errorf("pod %s/%s is not ready. Expected container %s to be ready", pod.Namespace, pod.Name, container.Name)
				}
			}
		}
		return nil
	}, time.Minute*20, time.Second*10).Should(Succeed(), fmt.Sprintf("Pods should become ready: %s", labels[v1alpha1.CassandraClusterComponent]))
	By(fmt.Sprintf("pods become ready in %s", time.Since(start)))
}

func waitForPodsTermination(namespace string, labels map[string]string) {
	Eventually(func() bool {
		podList := &v1.PodList{}
		err := kubeClient.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels(labels))
		Expect(err).To(Succeed())

		return len(podList.Items) == 0 //return true if expression is true
	}, time.Minute*10, time.Second*5).Should(BeTrue())
}

func waitForCassandraClusterSchemaDeletion(namespace string, release string) {
	Eventually(func() metav1.StatusReason {
		err := kubeClient.Get(ctx, types.NamespacedName{
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
	err = kubeClient.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels(labels))
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
			GinkgoWriter.Printf("Message from port forwarder: %s", out.String())
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
	req := k8sClientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	podLogs, err := req.Stream(ctx)
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
	req := k8sClientset.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
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
	Expect(kubeClient).ToNot(BeNil())

	adminSecret := &v1.Secret{}
	err := kubeClient.Get(ctx, types.NamespacedName{Name: cassandraCluster.Spec.AdminRoleSecretName, Namespace: cassandraCluster.Namespace}, adminSecret)
	if err != nil && errors.IsNotFound(err) {
		adminSecret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cassandraCluster.Spec.AdminRoleSecretName,
				Namespace: cassandraCluster.Namespace,
			},
			Data: map[string][]byte{
				v1alpha1.CassandraOperatorAdminRole:     []byte("cassandra-operator"),
				v1alpha1.CassandraOperatorAdminPassword: []byte("cassandra-operator-password"),
			},
		}
		Expect(kubeClient.Create(ctx, adminSecret)).To(Succeed())
	}

	By("Cassandra cluster is starting...")
	Expect(kubeClient.Create(ctx, cassandraCluster)).To(Succeed())

	By("Waiting for prober pods readiness...")
	waitForPodsReadiness(cassandraCluster.Namespace, labels.Prober(cassandraCluster), 1)

	By("Waiting for cassandra cluster pods readiness...")
	for _, dc := range cassandraCluster.Spec.DCs {
		waitForPodsReadiness(cassandraCluster.Namespace, labels.WithDCLabel(labels.Cassandra(cassandraCluster), dc.Name), *dc.Replicas)
	}

	By("Waiting for reaper pods readiness...")
	waitForPodsReadiness(cassandraCluster.Namespace, labels.Reaper(cassandraCluster), int32(len(cassandraCluster.Spec.DCs)))
	By("Cassandra cluster ready")
}

func showPodLogs(labels map[string]string, namespace string) {
	podList := &v1.PodList{}
	err := kubeClient.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels(labels))
	if err != nil {
		fmt.Println("Unable to get pods. Error: ", err)
		return
	}

	for _, pod := range podList.Items {
		fmt.Println("Logs from pod: ", pod.Name)

		for _, container := range pod.Spec.Containers {
			logFileName := fmt.Sprintf("%s%s-%s-%s.txt", debugLogsDir, pod.Namespace, pod.Name, container.Name)
			str, err := getPodLogs(pod, v1.PodLogOptions{TailLines: &cfg.tailLines, Container: container.Name})
			if err != nil {
				fileContent := []byte(fmt.Sprintf("couldn't get logs for pod %s/%s: %s", pod.Namespace, pod.Name, err.Error()))
				Expect(ioutil.WriteFile(logFileName, fileContent, 0777)).To(Succeed())
				continue
			}
			fmt.Println(str)
			allLogs, err := getPodLogs(pod, v1.PodLogOptions{Container: container.Name})
			if err != nil {
				fileContent := []byte(fmt.Sprintf("couldn't get logs for pod %s/%s container %s: %s", pod.Namespace, pod.Name, container.Name, err.Error()))
				Expect(ioutil.WriteFile(logFileName, fileContent, 0777)).To(Succeed())
				continue
			}

			Expect(ioutil.WriteFile(logFileName, []byte(allLogs), 0777)).To(Succeed())
		}
	}
}

// showClusterEvents shows all events from the cluster. Helpful if the pods were not able to be schedule
func showClusterEvents() {
	eventsList := &v1.EventList{}
	err := kubeClient.List(ctx, eventsList)
	if err != nil {
		fmt.Println("Unable to get events. Error: ", err)
		return
	}

	var eventsOutput []string
	for _, event := range eventsList.Items {
		eventsOutput = append(eventsOutput, fmt.Sprintf("%s %s %s/%s %s/%s: %s - %s",
			event.LastTimestamp.String(), event.Type,
			event.InvolvedObject.APIVersion, event.InvolvedObject.Kind,
			event.InvolvedObject.Namespace, event.InvolvedObject.Name,
			event.Name, event.Message,
		))
	}

	startIndex := len(eventsOutput) - int(cfg.tailLines)
	if startIndex < 0 {
		startIndex = 0
	}

	fmt.Println("Kubernetes events: ")
	fmt.Print(strings.Join(eventsOutput[startIndex:], "\n"))
	Expect(ioutil.WriteFile(debugLogsDir+"cluster-events.txt", []byte(strings.Join(eventsOutput, "\n")), 0777)).To(Succeed())

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
	err := kubeClient.Get(ctx, types.NamespacedName{Name: namespaceName}, ns)
	if err != nil && errors.IsNotFound(err) {
		ns = &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
		Expect(kubeClient.Create(ctx, ns)).To(Succeed())
	} else {
		Expect(err).ToNot(HaveOccurred())
	}

	copySecret(cfg.operatorNamespace, cfg.imagePullSecret, namespaceName)
	copySecret(cfg.operatorNamespace, cfg.ingressSecret, namespaceName)
}

func copySecret(namespaceFrom, secretName, namespaceTo string) {
	secret := &v1.Secret{}
	err := kubeClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespaceTo}, secret)
	if err == nil {
		return
	}

	Expect(kubeClient.Get(ctx, types.NamespacedName{Namespace: namespaceFrom, Name: secretName}, secret)).To(Succeed())
	newSecret := secret.DeepCopy()
	newSecret.Namespace = namespaceTo
	newSecret.ResourceVersion = ""
	newSecret.UID = ""
	delete(newSecret.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
	err = kubeClient.Create(ctx, newSecret)
	if err != nil && !errors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred())
	}
}

func createSecret(namespace, name string, data map[string][]byte) {
	existingSecret := &v1.Secret{}
	err := kubeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existingSecret)
	if err == nil {
		Expect(kubeClient.Delete(ctx, existingSecret)) //recreate the existing secret to ensure we have correct data in it
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: data,
	}
	Expect(kubeClient.Create(ctx, secret)).To(Succeed())
}

func removeNamespaces(namespaces ...string) {
	for _, namespace := range namespaces {
		ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		err := kubeClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)
		if err == nil {
			Expect(kubeClient.Delete(ctx, ns)).To(Succeed())
		}
	}
	By(fmt.Sprintf("Waiting for namespaces %v to be removed", namespaces))
	start := time.Now()

	for _, namespace := range namespaces {
		Eventually(func() error {
			return kubeClient.Get(ctx, types.NamespacedName{Name: namespace}, &v1.Namespace{})
		}, 15*time.Minute, 10*time.Second).ShouldNot(Succeed())
	}
	By(fmt.Sprintf("Namespaces removed after %s", time.Since(start)))
}

func cleanupResources(ccName, ccNamespace string) {
	By(fmt.Sprintf("cleaning up resources for CassandraCluster %s/%s", ccNamespace, ccName))
	cc := &v1alpha1.CassandraCluster{}
	err := kubeClient.Get(ctx, types.NamespacedName{Name: ccName, Namespace: ccNamespace}, cc)
	if err != nil && errors.IsNotFound(err) {
		return
	} else {
		Expect(err).ToNot(HaveOccurred())
	}

	Expect(kubeClient.Delete(ctx, cc)).To(Succeed())
	waitForPodsTermination(cc.Namespace, labels.Prober(cc))
	waitForPodsTermination(cc.Namespace, labels.Cassandra(cc))
	waitForPodsTermination(cc.Namespace, labels.Reaper(cc))

	if len(cc.Spec.AdminRoleSecretName) > 0 {
		adminRoleSecret := &v1.Secret{}
		err := kubeClient.Get(ctx, types.NamespacedName{Name: cc.Spec.AdminRoleSecretName, Namespace: ccNamespace}, adminRoleSecret)
		if err == nil {
			Expect(kubeClient.Delete(ctx, adminRoleSecret))
		}
	}

	Expect(kubeClient.DeleteAllOf(
		ctx,
		&v1.PersistentVolumeClaim{},
		client.InNamespace(cc.Namespace),
		client.MatchingLabels(labels.Cassandra(cc))),
	).To(Succeed())

	if len(cc.Spec.RolesSecretName) > 0 {
		roleSecret := &v1.Secret{}
		err := kubeClient.Get(ctx, types.NamespacedName{Name: cc.Spec.RolesSecretName, Namespace: ccNamespace}, roleSecret)
		if err == nil {
			Expect(kubeClient.Delete(ctx, roleSecret))
		}
	}
}
