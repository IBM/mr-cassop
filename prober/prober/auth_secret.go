package prober

import (
	"reflect"
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
			AddFunc: p.handleAddAuthSecret,
		})
	stopCh := make(chan struct{})
	go controller.Run(stopCh)
	return stopCh
}

func (p *Prober) handleAddAuthSecret(new interface{}) {
	newSecret := new.(*v1.Secret)
	p.log.Info(newSecret.Name + " Secret has been re-added.")

	p.jolokia.SetAuth(string(newSecret.Data["admin-role"]), string(newSecret.Data["admin-password"]))
}

func (p *Prober) WatchBaseSecret() chan struct{} {
	p.log.Info("Watching Secret " + p.cfg.BaseAdminRoleSecretName + "...")

	watchList := cache.NewListWatchFromClient(
		p.kubeClient.CoreV1().RESTClient(),
		v1.ResourceSecrets.String(),
		p.cfg.PodNamespace,
		fields.OneTermEqualSelector("metadata.name", p.cfg.BaseAdminRoleSecretName),
	)

	_, controller := cache.NewInformer(
		watchList,
		&v1.Secret{},
		time.Second*1,
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: p.handleUpdateBaseSecret,
		})
	stopCh := make(chan struct{})
	go controller.Run(stopCh)
	return stopCh
}

func (p *Prober) handleUpdateBaseSecret(oldObj interface{}, newObj interface{}) {
	oldSecret := oldObj.(*v1.Secret)
	newSecret := newObj.(*v1.Secret)

	if !reflect.DeepEqual(oldSecret.Data, newSecret.Data) {
		p.log.Info(newSecret.Name + " Secret has been updated.")

		p.auth.User = string(newSecret.Data["admin-role"])
		p.auth.Password = string(newSecret.Data["admin-password"])
	}

}
