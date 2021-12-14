package prober

import (
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
)

func (p *Prober) WatchAuthSecret() chan struct{} {
	p.log.Info("Watching Secret " + p.cfg.AdminRoleSecretName + "...")

	watchList := cache.NewListWatchFromClient(
		p.kubeClient.CoreV1().RESTClient(),
		v1.ResourceSecrets.String(),
		p.cfg.PodNamespace,
		fields.OneTermEqualSelector("metadata.name", p.cfg.AdminRoleSecretName),
	)

	_, controller := cache.NewInformer(
		watchList,
		&v1.Secret{},
		time.Second*1,
		cache.ResourceEventHandlerFuncs{
			AddFunc: p.handleAddSecret,
		})
	stopCh := make(chan struct{})
	go controller.Run(stopCh)
	return stopCh
}

func (p *Prober) handleAddSecret(new interface{}) {
	newSecret := new.(*v1.Secret)
	p.log.Info(newSecret.Name + " Secret has been added/updated.")

	username := string(newSecret.Data["admin-role"])
	password := string(newSecret.Data["admin-password"])
	p.auth.User = username
	p.auth.Password = password
	p.jolokia.SetAuth(username, password)
}
