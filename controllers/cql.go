package controllers

import (
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
)

func newCassandraConfig(cc *v1alpha1.CassandraCluster, adminRole string, adminPwd string) *gocql.ClusterConfig {
	cassCfg := gocql.NewCluster(fmt.Sprintf("%s.%s.svc.cluster.local", names.DCService(cc.Name, cc.Spec.DCs[0].Name), cc.Namespace))
	cassCfg.Authenticator = &gocql.PasswordAuthenticator{
		Username: adminRole,
		Password: adminPwd,
	}

	cassCfg.Timeout = 6 * time.Second
	cassCfg.ConnectTimeout = 6 * time.Second
	cassCfg.ProtoVersion = 4
	cassCfg.Consistency = gocql.LocalQuorum
	cassCfg.ReconnectionPolicy = &gocql.ConstantReconnectionPolicy{
		MaxRetries: 3,
		Interval:   time.Second * 1,
	}

	if cc.Spec.Encryption.Client.Enabled {
		cassCfg.SslOpts = &gocql.SslOptions{
			CertPath: fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc),
				cc.Spec.Encryption.Client.TLSSecret.TLSCrtFileKey),
			KeyPath: fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc),
				cc.Spec.Encryption.Client.TLSSecret.TLSFileKey),
			CaPath: fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc),
				cc.Spec.Encryption.Client.TLSSecret.CAFileKey),
			EnableHostVerification: false,
		}
	}

	return cassCfg
}
