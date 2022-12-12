package icarus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type RestoreRequest struct {
	Type                      string            `json:"type"`
	StorageLocation           string            `json:"storageLocation"`
	SnapshotTag               string            `json:"snapshotTag"`
	DataDirs                  []string          `json:"dataDirs"`
	GlobalRequest             bool              `json:"globalRequest"`
	RestorationStrategyType   string            `json:"restorationStrategyType"`
	RestorationPhase          string            `json:"restorationPhase"`
	Import                    RestoreImport     `json:"import"`
	K8sNamespace              string            `json:"k8sNamespace,omitempty"`
	K8sSecretName             string            `json:"k8sSecretName,omitempty"`
	ConcurrentConnections     int64             `json:"concurrentConnections,omitempty"`
	Entities                  string            `json:"entities,omitempty"`
	NoDeleteTruncates         bool              `json:"noDeleteTruncates,omitempty"`
	NoDeleteDownloads         bool              `json:"noDeleteDownloads,omitempty"`
	NoDownloadData            bool              `json:"noDownloadData,omitempty"`
	Timeout                   int64             `json:"timeout,omitempty"`
	ResolveHostIdFromTopology bool              `json:"resolveHostIdFromTopology,omitempty"`
	Insecure                  bool              `json:"insecure,omitempty"`
	SkipBucketVerification    bool              `json:"skipBucketVerification,omitempty"`
	Retry                     Retry             `json:"retry,omitempty"`
	Rename                    map[string]string `json:"rename,omitempty"`
	SinglePhase               bool              `json:"singlePhase,omitempty"`
	DC                        string            `json:"dc,omitempty"`
	SchemaVersion             string            `json:"schemaVersion,omitempty"`
	ExactSchemaVersion        bool              `json:"exactSchemaVersion,omitempty"`
}

type Restore struct {
	Id                        string            `json:"id"`
	CreationTime              string            `json:"creationTime"`
	State                     string            `json:"state"`
	Errors                    []Error           `json:"errors"`
	Progress                  float64           `json:"progress"`
	StartTime                 string            `json:"startTime"`
	Type                      string            `json:"type"`
	StorageLocation           string            `json:"storageLocation"`
	ConcurrentConnections     int64             `json:"concurrentConnections"`
	CassandraConfigDirectory  string            `json:"cassandraConfigDirectory"`
	RestoreSystemKeyspace     bool              `json:"restoreSystemKeyspace"`
	SnapshotTag               string            `json:"snapshotTag"`
	Entities                  string            `json:"entities"`
	RestorationStrategyType   string            `json:"restorationStrategyType"`
	RestorationPhase          string            `json:"restorationPhase"`
	Import                    RestoreImport     `json:"import"`
	NoDeleteTruncates         bool              `json:"noDeleteTruncates"`
	NoDeleteDownloads         bool              `json:"noDeleteDownloads"`
	NoDownloadData            bool              `json:"noDownloadData"`
	ExactSchemaVersion        bool              `json:"exactSchemaVersion"`
	GlobalRequest             bool              `json:"globalRequest"`
	Timeout                   int64             `json:"timeout"`
	ResolveHostIdFromTopology bool              `json:"resolveHostIdFromTopology"`
	Insecure                  bool              `json:"insecure"`
	SkipBucketVerification    bool              `json:"skipBucketVerification"`
	Retry                     Retry             `json:"retry"`
	SinglePhase               bool              `json:"singlePhase"`
	DataDirs                  []string          `json:"dataDirs"`
	DC                        string            `json:"dc"`
	K8sNamespace              string            `json:"k8sNamespace"`
	K8sSecretName             string            `json:"k8sSecretName"`
	Rename                    map[string]string `json:"rename"`
}

type RestoreImport struct {
	Type               string `json:"type"`
	SourceDir          string `json:"sourceDir"`
	KeepLevel          bool   `json:"keepLevel,omitempty"`
	NoVerify           bool   `json:"noVerify,omitempty"`
	NoVerifyTokens     bool   `json:"noVerifyTokens,omitempty"`
	NoInvalidateCaches bool   `json:"noInvalidateCaches,omitempty"`
	Quick              bool   `json:"quick,omitempty"`
	ExtendedVerify     bool   `json:"extendedVerify,omitempty"`
	KeepRepaired       bool   `json:"keepRepaired,omitempty"`
}

func (c *client) Restore(ctx context.Context, restoreRequest RestoreRequest) error {
	restoreRequest.Type = "restore"
	body, err := json.Marshal(restoreRequest)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.addr+"/operations", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("backup request failed: code: %d, body: %s", resp.StatusCode, string(b))
	}

	return nil
}

func (c *client) Restores(ctx context.Context) ([]Restore, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.addr+"/operations?type=restore", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("backup request failed: code: %d, body: %s", resp.StatusCode, string(b))
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var backups []Restore
	err = json.Unmarshal(b, &backups)
	if err != nil {
		return nil, err
	}

	return backups, nil
}
