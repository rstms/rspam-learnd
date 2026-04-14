package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/rstms/rspam-learnd/sample"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const Version = "0.0.4"

const DEFAULT_SERVER_NAME = "localhost"
const DEFAULT_SHUTDOWN_TIMEOUT_SECONDS = 10
const SAMPLE_QUEUE_LENGTH = 1024

type Server struct {
	Name                   string
	tls                    bool
	ServerName             string
	Address                string
	Port                   int
	verbose                bool
	debug                  bool
	wg                     sync.WaitGroup
	shutdown               chan struct{}
	caFile                 string
	certFile               string
	keyFile                string
	shutdownTimeoutSeconds int
	enableMenu             bool
	Queue                  chan *sample.Sample
	QueueCount             int
	DequeueCount           int
}

func expandFilename(filename string) (string, error) {
	filename = os.ExpandEnv(filename)
	if strings.HasPrefix(filename, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		filename = filepath.Join(homeDir, filename[1:])
	}
	return filepath.Clean(filename), nil
}

func NewServer() (*Server, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return nil, Fatal(err)
	}
	ViperSetDefault("server_name", DEFAULT_SERVER_NAME)
	ViperSetDefault("shutdown_timeout_seconds", DEFAULT_SHUTDOWN_TIMEOUT_SECONDS)
	ViperSetDefault("ca", filepath.Join(userConfigDir, ProgramName(), "ca.pem"))
	ViperSetDefault("cert", filepath.Join(userConfigDir, ProgramName(), "learnd.pem"))
	ViperSetDefault("key", filepath.Join(userConfigDir, ProgramName(), "learnd.key"))

	s := Server{
		Name:                   "learnd",
		tls:                    ViperGetBool("tls"),
		ServerName:             ViperGetString("server_name"),
		Address:                ViperGetString("address"),
		Port:                   ViperGetInt("port"),
		verbose:                ViperGetBool("verbose"),
		debug:                  ViperGetBool("debug"),
		shutdown:               make(chan struct{}, 1),
		shutdownTimeoutSeconds: ViperGetInt("shutdown_timeout_seconds"),
		caFile:                 ViperGetString("ca"),
		certFile:               ViperGetString("cert"),
		keyFile:                ViperGetString("key"),
		Queue:                  make(chan *sample.Sample, SAMPLE_QUEUE_LENGTH),
	}

	if s.debug {
		log.Printf("[%s] config: %+v\n", s.Name, s)
	}

	return &s, nil
}

func (s *Server) Stop() error {
	log.Printf("[%s] requesting shutdown", s.Name)
	s.shutdown <- struct{}{}
	log.Printf("[%s] waiting for shutdown", s.Name)
	s.wg.Wait()
	log.Printf("[%s] shutdown complete", s.Name)
	return nil
}

func (s *Server) Start() error {

	mux := http.NewServeMux()
	mux.HandleFunc("POST /learn/", s.HandlePostLearn)
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.Address, s.Port),
		Handler: mux,
	}
	serverMode := "HTTP"
	if s.tls {
		serverMode = "HTTPS"

		if s.certFile == "" || s.keyFile == "" || s.caFile == "" {
			return fmt.Errorf("incomplete TLS config: cert=%s key=%s ca=%s\n", s.certFile, s.keyFile, s.caFile)
		}

		cert, err := tls.LoadX509KeyPair(s.certFile, s.keyFile)
		if err != nil {
			return fmt.Errorf("error loading client certificate pair: %v", err)
		}

		caCerts, err := os.ReadFile(s.caFile)
		if err != nil {
			return fmt.Errorf("error loading certificate authority file: %v", err)
		}

		clientCertPool := x509.NewCertPool()
		ok := clientCertPool.AppendCertsFromPEM(caCerts)
		if !ok {
			return fmt.Errorf("error loading client validation certificate authority file: %v", err)
		}

		server.TLSConfig = &tls.Config{
			ServerName:   s.ServerName,
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.VerifyClientCertIfGiven,
			ClientCAs:    clientCertPool,
		}
		//fmt.Printf("configured TLS: %s %s %s\n", caFile, certFile, keyFile)
	}

	log.Printf("[%s] v%s started as PID %d\n", s.Name, Version, os.Getpid())

	s.wg.Add(1)
	go func() {
		defer log.Printf("[%s] %s server exiting", s.Name, serverMode)
		defer s.wg.Done()
		log.Printf("[%s] %s server listening on %s\n", s.Name, serverMode, server.Addr)
		if s.tls {
			err := server.ListenAndServeTLS("", "")
			if err != nil && err != http.ErrServerClosed {
				log.Fatalf("[%s] ListenAndServeTLS failed: %v", s.Name, err)
			}
		} else {
			err := server.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				log.Fatalf("[%s] ListenAndServe failed: %v", s.Name, err)
			}
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		<-s.shutdown
		log.Printf("[%s] received shutdown request", s.Name)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.shutdownTimeoutSeconds)*time.Second)
		defer cancel()

		log.Printf("[%s] shutting down %s server", s.Name, serverMode)
		err := server.Shutdown(ctx)
		if err != nil {
			log.Fatalf("[%s] %s Server Shutdown failed: %v", s.Name, serverMode, err)
		}

	}()

	// FIXME: detect listening server port here instead of just sleeping
	time.Sleep(1 * time.Second)

	return nil
}

func (s *Server) Run() error {

	err := s.Start()
	if err != nil {
		return Fatal(err)
	}

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, syscall.SIGINT)
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case sample, ok := <-s.Queue:
				if ok {
					if s.verbose {
						log.Printf("[%s] dequed sample: %+v\n", s.Name, sample)
					}
					sample.Submit()
				} else {
					log.Printf("[%s] sample queue closed\n", s.Name)
					return
				}
			case <-sigint:
				log.Printf("[%s] received SIGINT\n", s.Name)
				return
			case <-sigterm:
				log.Printf("[%s] received SIGTERM\n", s.Name)
				return
			case <-s.shutdown:
				log.Printf("[%s] shutdown requested\n", s.Name)
				return
			}
		}
	}()

	if s.verbose {
		fmt.Println("CTRL-C to exit")
	}

	wg.Wait()

	err = s.Stop()
	if err != nil {
		return Fatal(err)
	}

	return nil
}

func fail(w http.ResponseWriter, message string, status int) {
	log.Printf("  [%d] %s", status, message)
	http.Error(w, message, status)
}

func (s *Server) HandlePostLearn(w http.ResponseWriter, r *http.Request) {

	if s.verbose {
		log.Printf("%s %s %s (%d) debug=%v\n", r.RemoteAddr, r.Method, r.RequestURI, r.ContentLength, s.debug)
	}
	path := strings.Split(r.URL.Path[7:], "/")
	if len(path) != 2 {
		fail(w, "invalid path", http.StatusBadRequest)
		return
	}
	class := path[0]
	username := path[1]
	if class != "ham" && class != "spam" {
		fail(w, "unknown class", http.StatusBadRequest)
		return
	}
	if len(username) < 1 {
		fail(w, "invalid user", http.StatusBadRequest)
		return
	}

	if !s.debug {
		usernameHeader, ok := r.Header["X-Client-Cert-Dn"]
		if !ok {
			fail(w, "missing client cert DN", http.StatusBadRequest)
			return
		}
		if s.verbose {
			log.Printf("client cert dn: %s\n", usernameHeader[0])
		}
		if usernameHeader[0] != "CN="+username {
			fail(w, fmt.Sprintf("client cert (%s) != path username (%s)", usernameHeader[0], username), http.StatusBadRequest)
			return
		}
	}
	if s.verbose {
		log.Printf("parsing form\n")
	}
	err := r.ParseMultipartForm(256 << 20) // limit file size to 256MB
	if err != nil {
		fail(w, fmt.Sprintf("failed parsing upload form: %v", err), http.StatusBadRequest)
		return
	}

	if s.verbose {
		log.Printf("creating uploadFile\n")
	}
	uploadFile, _, err := r.FormFile("file")
	if err != nil {
		fail(w, fmt.Sprintf("failed retreiving upload file: %v", err), http.StatusBadRequest)
		return
	}
	defer uploadFile.Close()

	if s.verbose {
		log.Printf("unmarshalling domains\n")
	}
	domainsString := r.FormValue("domains")
	var domains []string
	err = json.Unmarshal([]byte(domainsString), &domains)
	if err != nil {
		fail(w, "Failed to unmarshal domains", http.StatusBadRequest)
		return
	}

	if s.verbose {
		log.Printf("reading uploadFile into buffer\n")
	}
	var buf bytes.Buffer
	count, err := io.Copy(&buf, uploadFile)
	if err != nil {
		fail(w, err.Error(), http.StatusBadRequest)
		return
	}
	message := buf.Bytes()

	if s.verbose {
		log.Printf("creating sample: class=%s username=%s domains=%v message=(%d bytes)\n", class, username, domains, len(message))
	}

	sample := sample.NewSample(class, username, domains, &message)

	if s.verbose {
		log.Printf("enqueing sample: %v\n", sample)
	}
	s.Queue <- sample
	s.QueueCount++
	if s.verbose {
		log.Printf("queued %s %s sample: byteCount=%v queueCount=%d dequeCount=%d\n", username, class, count, s.QueueCount, s.DequeueCount)
	}

	fail(w, "WAT?", http.StatusNotFound)
}
