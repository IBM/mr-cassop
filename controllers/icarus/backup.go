package icarus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	StatePending   = "PENDING"
	StateRunning   = "RUNNING"
	StateCompleted = "COMPLETED"
	StateCancelled = "CANCELLED"
	StateFailed    = "FAILED"
)

type BackupRequest struct {
	Type                   string    `json:"type"`
	StorageLocation        string    `json:"storageLocation"`
	DataDirs               []string  `json:"dataDirs"`
	GlobalRequest          bool      `json:"globalRequest"`
	SnapshotTag            string    `json:"snapshotTag"`
	K8sNamespace           string    `json:"k8sNamespace,omitempty"`
	K8sSecretName          string    `json:"k8sSecretName,omitempty"`
	Duration               string    `json:"duration,omitempty"`
	Bandwidth              *DataRate `json:"bandwidth,omitempty"`
	ConcurrentConnections  int64     `json:"concurrentConnections,omitempty"`
	DC                     string    `json:"dc,omitempty"`
	Entities               string    `json:"entities,omitempty"`
	Timeout                int64     `json:"timeout,omitempty"`
	MetadataDirective      string    `json:"metadataDirective,omitempty"`
	Insecure               bool      `json:"insecure"`
	CreateMissingBucket    bool      `json:"createMissingBucket"`
	SkipRefreshing         bool      `json:"skipRefreshing"`
	SkipBucketVerification bool      `json:"skipBucketVerification"`
	Retry                  Retry     `json:"retry,omitempty"`
}

type DataRate struct {
	Value int64  `json:"value"`
	Unit  string `json:"unit"`
}

type Backup struct {
	ID                     string    `json:"id"`
	CreationTime           string    `json:"creationTime"`
	State                  string    `json:"state"`
	Errors                 []Error   `json:"errors"`
	Progress               float64   `json:"progress"`
	StartTime              string    `json:"startTime"`
	Type                   string    `json:"type"`
	StorageLocation        string    `json:"storageLocation"`
	ConcurrentConnections  int64     `json:"concurrentConnections"`
	MetadataDirective      string    `json:"metadataDirective"`
	Entities               string    `json:"entities"`
	SnapshotTag            string    `json:"snapshotTag"`
	GlobalRequest          bool      `json:"globalRequest"`
	Timeout                int64     `json:"timeout"`
	Insecure               bool      `json:"insecure"`
	SchemaVersion          string    `json:"schemaVersion"`
	CreateMissingBucket    bool      `json:"createMissingBucket"`
	SkipBucketVerification bool      `json:"skipBucketVerification"`
	UploadClusterTopology  bool      `json:"uploadClusterTopology"`
	Retry                  Retry     `json:"retry"`
	SkipRefreshing         bool      `json:"skipRefreshing"`
	DataDirs               []string  `json:"dataDirs"`
	DC                     string    `json:"dc"`
	K8sNamespace           string    `json:"k8sNamespace"`
	K8sSecretName          string    `json:"k8sSecretName"`
	Duration               string    `json:"duration"`
	Bandwidth              *DataRate `json:"bandwidth"`
}

type Error struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

func (c *client) Backup(ctx context.Context, backupReq BackupRequest) (Backup, error) {
	backupReq.Type = "backup"
	body, err := json.Marshal(backupReq)
	if err != nil {
		return Backup{}, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.addr+"/operations", bytes.NewReader(body))
	if err != nil {
		return Backup{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Backup{}, err
	}
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return Backup{}, fmt.Errorf("backup request failed: code: %d, body: %s", resp.StatusCode, string(b))
	}

	backup := Backup{}
	err = json.Unmarshal(b, &backup)
	if err != nil {
		return Backup{}, err
	}

	return backup, nil
}

func (c *client) Backups(ctx context.Context) ([]Backup, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.addr+"/operations?type=backup", nil)
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
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("backup request failed: code: %d, body: %s", resp.StatusCode, string(b))
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var backups []Backup
	err = json.Unmarshal(b, &backups)
	if err != nil {
		return nil, err
	}

	return backups, nil
}
