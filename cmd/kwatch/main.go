package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pontostroy/kwatch/internal/client"
	"github.com/pontostroy/kwatch/internal/config"
	"github.com/pontostroy/kwatch/internal/constant"
	"github.com/pontostroy/kwatch/internal/controller"
	"github.com/pontostroy/kwatch/internal/handler"
	"github.com/pontostroy/kwatch/internal/health"
	"github.com/pontostroy/kwatch/internal/k8s"
	"github.com/pontostroy/kwatch/internal/pvc"
	"github.com/pontostroy/kwatch/internal/startup"
	"github.com/pontostroy/kwatch/internal/storage/memory"
	"github.com/pontostroy/kwatch/internal/upgrader"
	"github.com/pontostroy/kwatch/internal/version"
	"k8s.io/klog/v2"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		klog.ErrorS(err, "failed to load config")
		os.Exit(1)
	}

	klog.InfoS(fmt.Sprintf(constant.WelcomeMsg, version.Short()))

	k8sClient := client.Create(&cfg.App)

	sm := startup.NewStartupManager(
		k8sClient,
		k8s.GetNamespace(),
		cfg.Alert,
		&cfg.App,
	)
	sm.HandleStartup(context.Background())

	healthServer := health.NewHealthServer(cfg.HealthCheck)
	healthServer.Start(context.Background())

	up := upgrader.NewUpgrader(&cfg.Upgrader, sm.GetAlertManager(), sm.GetStateManager())
	go up.CheckUpdates()

	pvcMonitor := pvc.NewPvcMonitor(k8sClient, &cfg.PvcMonitor, sm.GetAlertManager())
	go pvcMonitor.Start()

	h := handler.NewHandler(
		k8sClient,
		cfg,
		memory.NewMemory(),
		sm.GetAlertManager(),
	)

	ctrl, cleanup := controller.New(k8sClient, cfg, h)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := ctrl.Run(ctx, 1); err != nil {
			klog.ErrorS(err, "controller error")
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	klog.InfoS("shutting down gracefully...")
	cancel()
	healthServer.Stop(context.Background())
	os.Exit(0)
}
