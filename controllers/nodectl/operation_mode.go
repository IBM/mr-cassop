package nodectl

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/ibm/cassandra-operator/controllers/nodectl/jolokia"
)

type OperationMode string

const (
	NodeOperationModeStarting       OperationMode = "STARTING"
	NodeOperationModeNormal         OperationMode = "NORMAL"
	NodeOperationModeJoining        OperationMode = "JOINING"
	NodeOperationModeLeaving        OperationMode = "LEAVING"
	NodeOperationModeDecommissioned OperationMode = "DECOMMISSIONED"
	NodeOperationModeMoving         OperationMode = "MOVING"
	NodeOperationModeDraining       OperationMode = "DRAINING"
	NodeOperationModeDrained        OperationMode = "DRAINED"
)

func (n *client) OperationMode(ctx context.Context, nodeIP string) (OperationMode, error) {
	req := jolokia.JMXRequest{
		Type:       jmxRequestTypeRead,
		Mbean:      mbeanCassandraDBStorageService,
		Attributes: []string{"OperationMode"},
	}

	resp, err := n.jolokia.Post(ctx, req, nodeIP)
	if err != nil {
		return "", err
	}

	opModeResponse := make(map[string]string)
	err = json.Unmarshal(resp.Value, &opModeResponse)
	if err != nil {
		return "", err
	}

	opModeStr, exists := opModeResponse["OperationMode"]
	if !exists {
		return "", errors.Errorf("couldn't find operation mode field, raw response: %s", string(resp.Value))
	}
	opMode := OperationMode(opModeStr)

	allowedOpModes := []OperationMode{
		NodeOperationModeStarting,
		NodeOperationModeNormal,
		NodeOperationModeJoining,
		NodeOperationModeLeaving,
		NodeOperationModeDecommissioned,
		NodeOperationModeMoving,
		NodeOperationModeDraining,
		NodeOperationModeDrained,
	}

	found := false
	for _, allowedMode := range allowedOpModes {
		if allowedMode == opMode {
			found = true
			break
		}
	}
	if !found {
		return "", errors.Errorf("unknown operation mode %s", opMode)
	}

	return opMode, nil
}
