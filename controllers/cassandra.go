package controllers

import (
	"context"
	"fmt"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
)

type checksumContainer map[string]string

func (c checksumContainer) checksum() string {
	return util.Sha1(fmt.Sprintf("%v", c))
}

var (
	errTLSSecretNotFound = errors.New("TLS secret not found")
	errTLSSecretInvalid  = errors.New("TLS secret is not valid")
)

func (r *CassandraClusterReconciler) reconcileCassandra(ctx context.Context, cc *dbv1alpha1.CassandraCluster, restartChecksum checksumContainer) error {
	for _, dc := range cc.Spec.DCs {
		err := r.reconcileDCService(ctx, cc, dc)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile dc %q", dc.Name)
		}

		err = r.reconcileDCStatefulSet(ctx, cc, dc, restartChecksum)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile dc %q", dc.Name)
		}
	}

	if err := r.reconcileCassandraPodLabels(ctx, cc); err != nil {
		return errors.Wrap(err, "Failed to reconcile cassandra pods labels")
	}

	return nil
}
