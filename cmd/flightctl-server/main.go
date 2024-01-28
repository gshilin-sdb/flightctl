package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	cacheutil "github.com/argoproj/argo-cd/v2/util/cache"
	oapimiddleware "github.com/deepmap/oapi-codegen/pkg/chi-middleware"
	api "github.com/flightctl/flightctl/api/v1alpha1"
	"github.com/flightctl/flightctl/internal/config"
	"github.com/flightctl/flightctl/internal/configprovider/git"
	"github.com/flightctl/flightctl/internal/crypto"
	device_updater "github.com/flightctl/flightctl/internal/monitors/device-updater"
	"github.com/flightctl/flightctl/internal/monitors/repotester"
	"github.com/flightctl/flightctl/internal/server"
	"github.com/flightctl/flightctl/internal/service"
	"github.com/flightctl/flightctl/internal/store"
	"github.com/flightctl/flightctl/pkg/log"
	"github.com/flightctl/flightctl/pkg/thread"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	gracefulShutdownTimeout     = 5 * time.Second
	cacheExpirationTime         = 10 * time.Minute
	caCertValidityDays          = 365 * 10
	serverCertValidityDays      = 365 * 1
	clientBootStrapValidityDays = 365 * 1
	signerCertName              = "ca"
	serverCertName              = "server"
	clientBootstrapCertName     = "client-enrollment"
)

func main() {
	log := log.InitLogs()
	log.Println("Starting device management service")
	defer log.Println("Device management service stopped")

	cfg, err := config.LoadOrGenerate(config.ConfigFile())
	if err != nil {
		log.Fatalf("reading configuration: %v", err)
	}
	log.Printf("Using config: %s", cfg)

	ca, _, err := crypto.EnsureCA(certFile(signerCertName), keyFile(signerCertName), "", signerCertName, caCertValidityDays)
	if err != nil {
		log.Fatalf("ensuring CA cert: %v", err)
	}

	// default certificate hostnames to localhost if nothing else is configured
	if len(cfg.Service.AltNames) == 0 {
		cfg.Service.AltNames = []string{"localhost"}
	}

	serverCerts, _, err := ca.EnsureServerCertificate(certFile(serverCertName), keyFile(serverCertName), cfg.Service.AltNames, serverCertValidityDays)
	if err != nil {
		log.Fatalf("ensuring server cert: %v", err)
	}
	_, _, err = ca.EnsureClientCertificate(certFile(clientBootstrapCertName), keyFile(clientBootstrapCertName), clientBootstrapCertName, clientBootStrapValidityDays)
	if err != nil {
		log.Fatalf("ensuring bootstrap client cert: %v", err)
	}

	log.Println("Initializing data store")
	db, err := store.InitDB(cfg)
	if err != nil {
		log.Fatalf("initializing data store: %v", err)
	}
	store := store.NewStore(db, log.WithField("pkg", "store"))
	if err := store.InitialMigration(); err != nil {
		log.Fatalf("running initial migration: %v", err)
	}

	log.Println("Initializing caching layer")
	cache := cacheutil.NewCache(cacheutil.NewInMemoryCache(cacheExpirationTime))

	log.Println("Initializing config providers")
	_ = git.NewGitConfigProvider(cache, cacheExpirationTime, cacheExpirationTime)

	log.Println("Initializing API server")
	swagger, err := api.GetSwagger()
	if err != nil {
		log.Fatalf("loading swagger spec: %v", err)
	}
	// Skip server name validation
	swagger.Servers = nil

	router := chi.NewRouter()
	router.Use(
		middleware.RequestID,
		middleware.Logger,
		middleware.Recoverer,
		oapimiddleware.OapiRequestValidator(swagger),
	)

	h := service.NewServiceHandler(store, ca, log)
	server.HandlerFromMux(server.NewStrictHandler(h, nil), router)

	tlsConfig, err := crypto.TLSConfigForServer(ca.Config, serverCerts)
	if err != nil {
		log.Fatalf("creating TLS config: %v", err)
	}
	srv := &http.Server{
		Addr:         cfg.Service.Address,
		Handler:      router,
		TLSConfig:    tlsConfig,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	sigShutdown := make(chan os.Signal, 1)
	signal.Notify(sigShutdown, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sigShutdown
		log.Println("Shutdown signal received")

		ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		_ = srv.Shutdown(ctxTimeout)
	}()

	repoTester := repotester.NewRepoTester(log, db, store)
	repoTesterThread := thread.New(
		log.WithField("pkg", "repository-tester"), "Repository tester", time.Duration(2*float64(time.Minute)), repoTester.TestRepo)
	repoTesterThread.Start()
	defer repoTesterThread.Stop()

	deviceUpdater := device_updater.NewDeviceUpdater(log, db, store)
	deviceUpdaterThread := thread.New(
		log.WithField("pkg", "device-updater"), "Device updater", time.Duration(2*float64(time.Minute)), deviceUpdater.UpdateDevices)
	deviceUpdaterThread.Start()
	defer deviceUpdaterThread.Stop()

	log.Printf("Listening on %s...", srv.Addr)
	if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func certFile(name string) string {
	return filepath.Join(config.CertificateDir(), name+".crt")
}

func keyFile(name string) string {
	return filepath.Join(config.CertificateDir(), name+".key")
}
