package nodetool

import (
	"bytes"
	"fmt"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type NodetoolClient interface {
	RepairKeyspace(cc *v1alpha1.CassandraCluster, user string) error
}

type nodetoolClient struct {
	clientset  *kubernetes.Clientset
	restConfig *rest.Config
	roleName   string
	password   string
}

func NewNodetoolClient(clientset *kubernetes.Clientset, config *rest.Config, roleName, password string) NodetoolClient {
	return &nodetoolClient{
		clientset:  clientset,
		restConfig: config,
		roleName:   roleName,
		password:   password,
	}
}

func (n *nodetoolClient) RepairKeyspace(cc *v1alpha1.CassandraCluster, keyspace string) error {
	cmd := []string{
		"sh",
		"-c",
		fmt.Sprintf("nodetool -u %s -pw \"%s\" -h %s repair -full -dcpar -- %s", n.roleName, n.password, getFirstPodName(cc), keyspace),
	}

	return n.execOnPod(cc, cmd)
}

func (n *nodetoolClient) execOnPod(cc *v1alpha1.CassandraCluster, cmd []string) error {
	nodetoolReq := n.clientset.CoreV1().RESTClient().Post().Resource("pods").Name(getFirstPodName(cc)).
		Namespace(cc.Namespace).SubResource("exec")
	option := &v1.PodExecOptions{
		Command: cmd,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     false,
	}

	nodetoolReq.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(n.restConfig, "POST", nodetoolReq.URL())
	if err != nil {
		return err
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})
	if err != nil {
		return errors.Wrapf(err, "Exec failed.\n stdout: %s\n, stderr: %s\n", stdout, stderr)
	}

	return err
}

func getFirstPodName(cc *v1alpha1.CassandraCluster) string {
	return fmt.Sprintf("%s-cassandra-%s-0", cc.Name, cc.Spec.DCs[0].Name)
}
