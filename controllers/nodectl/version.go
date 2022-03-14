package nodectl

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ibm/cassandra-operator/controllers/nodectl/jolokia"
	"github.com/pkg/errors"
)

func (n *client) Version(nodeIP string) (major, minor, patch int, err error) {
	req := jolokia.JMXRequest{
		Type:       jmxRequestTypeRead,
		Mbean:      mbeanCassandraDBStorageService,
		Attributes: []string{"ReleaseVersion"},
	}

	var resp jolokia.JMXResponse
	resp, err = n.jolokia.Post(req, nodeIP)
	if err != nil {
		return
	}

	versionInfo := make(map[string]string)
	err = json.Unmarshal(resp.Value, &versionInfo)
	if err != nil {
		err = errors.Wrapf(err, "can't unmarshal version info, raw body: %s", string(resp.Value))
		return
	}

	releaseVersion, exists := versionInfo["ReleaseVersion"]
	if !exists {
		err = errors.Errorf("version is empty, raw body: %s", string(resp.Value))
		return
	}
	semVer := strings.Split(releaseVersion, ".")

	if len(semVer) == 0 {
		err = errors.Errorf("version info is empty. Raw string info %q", string(resp.Value))
		return
	}

	if len(semVer) > 0 {
		major, err = strconv.Atoi(semVer[0])
		if err != nil {
			// require only the major version to be present
			err = errors.Errorf("can't parse major version. Raw string: %q", resp.Value)
			return
		}
	}
	if len(semVer) > 1 {
		minor, err = strconv.Atoi(semVer[1])
		if err != nil {
			minor = -1
		}
	}
	if len(semVer) > 2 {
		patch, err = strconv.Atoi(semVer[2])
		if err != nil {
			patch = -1
		}
	}

	return major, minor, patch, nil
}
