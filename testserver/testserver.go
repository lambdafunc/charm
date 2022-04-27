package testserver

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/caarlos0/go-sshagent"
	"github.com/charmbracelet/charm/client"
	"github.com/charmbracelet/charm/server"
	"github.com/charmbracelet/keygen"
	"golang.org/x/crypto/ssh"
)

// Clients may hold one or more clients.
type Clients struct {
	// Full is a client with everything set, including the ssh agent option.
	Full *client.Client

	// NoAgent is a client without the ssh agent option set.
	NoAgent *client.Client
}

// SetupTestServerWithAgent starts a test server and a fake ssh agent with
// the given signers, and sets the needed environment variables so clients
// pick it up.
// It also returns a client forcing these settings in.
// Unless you use the given client, this is not really thread safe due
// to setting a bunch of environment variables.
func SetupTestServerWithAgent(tb testing.TB, signers ...ssh.Signer) Clients {
	tb.Helper()

	td := tb.TempDir()
	sp := filepath.Join(td, ".ssh")
	clientData := filepath.Join(td, ".client-data")

	cfg := server.DefaultConfig()
	cfg.DataDir = filepath.Join(td, ".data")
	cfg.SSHPort = randomPort(tb)
	cfg.HTTPPort = randomPort(tb)
	cfg.HealthPort = randomPort(tb)

	kp, err := keygen.NewWithWrite(filepath.Join(sp, "charm_server"), []byte(""), keygen.Ed25519)
	if err != nil {
		tb.Fatalf("keygen error: %s", err)
	}

	cfg = cfg.WithKeys(kp.PublicKey(), kp.PrivateKeyPEM())
	s, err := server.NewServer(cfg)
	if err != nil {
		tb.Fatalf("new server error: %s", err)
	}

	_ = os.Setenv("CHARM_HOST", cfg.Host)
	_ = os.Setenv("CHARM_SSH_PORT", fmt.Sprintf("%d", cfg.SSHPort))
	_ = os.Setenv("CHARM_HTTP_PORT", fmt.Sprintf("%d", cfg.HTTPPort))
	_ = os.Setenv("CHARM_DATA_DIR", clientData)

	go func() { _ = s.Start() }()

	resp, err := FetchURL(fmt.Sprintf("http://localhost:%d", cfg.HealthPort), 3)
	if err != nil {
		tb.Fatalf("server likely failed to start: %s", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	tb.Log("server ready!")

	var agt *sshagent.Agent
	if len(signers) > 0 {
		agt = sshagent.New(signers...)
		go func() {
			_ = agt.Start()
		}()

		tb.Cleanup(func() {
			_ = agt.Close()
		})

		for !agt.Ready() {
			time.Sleep(time.Millisecond * 100)
		}

		tb.Logf("fake ssh agent ready with %d keys", len(signers))
	}

	tb.Cleanup(func() {
		if err := s.Close(); err != nil {
			tb.Error("failed to close server:", err)
		}

		_ = os.Unsetenv("CHARM_HOST")
		_ = os.Unsetenv("CHARM_SSH_PORT")
		_ = os.Unsetenv("CHARM_HTTP_PORT")
		_ = os.Unsetenv("CHARM_DATA_DIR")
		if agt != nil {
			_ = os.Unsetenv("CHARM_USE_SSH_AGENT")
			_ = os.Unsetenv("CHARM_SSH_AGENT_ADDR")
		}
	})

	var clients Clients

	ccfg, err := client.ConfigFromEnv()
	if err != nil {
		tb.Fatalf("client config from env error: %s", err)
	}

	ccfg.Host = cfg.Host
	ccfg.SSHPort = cfg.SSHPort
	ccfg.HTTPPort = cfg.HTTPPort
	ccfg.DataDir = clientData

	cl, err := client.NewClient(ccfg)
	if err != nil {
		tb.Fatalf("new client error: %s", err)
	}
	clients.NoAgent = cl

	if agt != nil {
		_ = os.Setenv("CHARM_SSH_AGENT_ADDR", agt.Socket())
		_ = os.Setenv("CHARM_USE_SSH_AGENT", "true")

		ccfg, err := client.ConfigFromEnv()
		if err != nil {
			tb.Fatalf("client config from env error: %s", err)
		}
		ccfg.Host = cfg.Host
		ccfg.SSHPort = cfg.SSHPort
		ccfg.HTTPPort = cfg.HTTPPort
		ccfg.DataDir = clientData
		ccfg.UseSSHAgent = true
		ccfg.SSHAgentAddr = agt.Socket()

		cl, err := client.NewClient(ccfg)
		if err != nil {
			tb.Fatalf("new client error: %s", err)
		}
		clients.Full = cl
	}
	return clients
}

// SetupTestServer starts a test server and sets the needed environment
// variables so clients pick it up.
// It also returns a client forcing these settings in.
// Unless you use the given client, this is not really thread safe due
// to setting a bunch of environment variables.
func SetupTestServer(tb testing.TB) *client.Client {
	return SetupTestServerWithAgent(tb).NoAgent
}

// Fetch the given URL with N retries.
func FetchURL(url string, retries int) (*http.Response, error) {
	resp, err := http.Get(url) // nolint:gosec
	if err != nil {
		if retries > 0 {
			time.Sleep(time.Second)
			return FetchURL(url, retries-1)
		}
		return nil, err
	}
	if resp.StatusCode != 200 {
		return resp, fmt.Errorf("bad http status code: %d", resp.StatusCode)
	}
	return resp, nil
}

func randomPort(tb testing.TB) int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("could not get a random port: %s", err)
	}
	_ = listener.Close()

	addr := listener.Addr().String()

	p, _ := strconv.Atoi(addr[strings.LastIndex(addr, ":")+1:])
	return p
}
