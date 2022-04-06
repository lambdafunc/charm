package kv

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/charm/client"
	"github.com/charmbracelet/charm/server"
	"github.com/charmbracelet/keygen"
	badger "github.com/dgraph-io/badger/v3"
)

// Helpers

func startServer(t *testing.T, testName string, testFunc func()) {
	// set up template server configurations
	cfg := server.DefaultConfig()
	td := t.TempDir()
	cfg.DataDir = filepath.Join(td, ".data")
	sp := filepath.Join(td, ".ssh")
	kp, err := keygen.NewWithWrite(sp, "charm_server", []byte(""), keygen.Ed25519)
	if err != nil {
		t.Fatalf("keygen error: %s", err)
	}
	cfg = cfg.WithKeys(kp.PublicKey, kp.PrivateKeyPEM)
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("new server error: %s", err)
	}
	go s.Start()
	t.Run("health-ping", func(t *testing.T) {
		_, err := fetchURL(fmt.Sprintf("http://localhost:%d", cfg.HealthPort), 3)
		if err != nil {
			t.Fatalf("could not ping server: %s", err)
		}
	})
	t.Run(testName, func(t *testing.T) {
		testFunc()
	})
	t.Cleanup(func() {
		err := s.Close()
		if err != nil {
			log.Printf("error closing server: %s", err)
		}
	})
}

func fetchURL(url string, retries int) (*http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		if retries > 0 {
			time.Sleep(time.Second)
			return fetchURL(url, retries-1)
		}
		return nil, err
	}
	if resp.StatusCode != 200 {
		return resp, fmt.Errorf("bad http status code: %d", resp.StatusCode)
	}
	return resp, nil
}

func setup(t *testing.T) *KV {
	t.Helper()
	opt := badger.DefaultOptions("").WithInMemory(true)
	cc, err := client.NewClientWithDefaults()
	if err != nil {
		log.Fatal(err)
	}
	kv, err := Open(cc, "test", opt)
	if err != nil {
		log.Fatal(err)
	}
	return kv
}

// TestGet

func TestGetForEmptyDB(t *testing.T) {
	startServer(t, "get for empty DB", func() {
		kv := setup(t)
		_, err := kv.Get([]byte("1234"))
		if err == nil {
			t.Errorf("expected error")
		}
	})
}

func TestGet(t *testing.T) {
	startServer(t, "get for non-empty DB", func() {
		tests := []struct {
			testname  string
			key       []byte
			want      []byte
			expectErr bool
		}{
			{"valid kv pair", []byte("1234"), []byte("valid"), false},
			{"invalid key", []byte{}, []byte{}, true},
		}

		for _, tc := range tests {
			kv := setup(t)
			kv.Set(tc.key, tc.want)
			got, err := kv.Get(tc.key)
			if tc.expectErr {
				if err == nil {
					t.Errorf("%s: expected error", tc.testname)
				}
			} else {
				if err != nil {
					t.Errorf("%s: unexpected error %v", tc.testname, err)
				}
				if bytes.Compare(got, tc.want) != 0 {
					t.Errorf("%s: got %s, want %s", tc.testname, got, tc.want)
				}
			}
		}
	})
}

// TestSetReader

func TestSetReader(t *testing.T) {
	startServer(t, "set reader", func() {
		tests := []struct {
			testname  string
			key       []byte
			want      string
			expectErr bool
		}{
			{"set valid value", []byte("am key"), "hello I am a very powerful test *flex*", false},
			{"set empty key", []byte(""), "", true},
		}

		for _, tc := range tests {
			kv := setup(t)
			kv.SetReader(tc.key, strings.NewReader(tc.want))
			got, err := kv.Get(tc.key)
			if tc.expectErr {
				if err == nil {
					t.Errorf("case: %s expected an error but did not get one", tc.testname)
				}
			} else {
				if err != nil {
					t.Errorf("case: %s unexpected error %v", tc.testname, err)
				}
				if bytes.Compare(got, []byte(tc.want)) != 0 {
					t.Errorf("case: %s got %s, want %s", tc.testname, got, tc.want)

				}
			}
		}
	})
}

// TestDelete

func TestDelete(t *testing.T) {
	startServer(t, "set reader", func() {
		tests := []struct {
			testname  string
			key       []byte
			value     []byte
			expectErr bool
		}{
			{"valid key", []byte("hello"), []byte("value"), false},
			{"empty key with value", []byte{}, []byte("value"), true},
			{"empty key no value", []byte{}, []byte{}, true},
		}

		for _, tc := range tests {
			kv := setup(t)
			kv.Set(tc.key, tc.value)
			if tc.expectErr {
				if err := kv.Delete(tc.key); err == nil {
					t.Errorf("%s: expected error", tc.testname)
				}
			} else {
				if err := kv.Delete(tc.key); err != nil {
					t.Errorf("%s: unexpected error in Delete %v", tc.testname, err)
				}
				want := []byte{} // want an empty result
				if get, _ := kv.Get(tc.key); bytes.Compare(get, want) != 0 {
					t.Errorf("%s: expected an empty string %s, got %s", tc.testname, want, get)
				}
			}
		}
	})
}

// TestSync

func TestSync(t *testing.T) {
	startServer(t, "set reader", func() {
		kv := setup(t)
		err := kv.Sync()
		if err != nil {
			t.Errorf("unexpected error")
		}
	})
}

// TestOptionsWithEncryption

func TestOptionsWithEncryption(t *testing.T) {
	startServer(t, "set reader", func() {
		_, err := OptionsWithEncryption(badger.DefaultOptions(""), []byte("1234"), -2)
		if err == nil {
			t.Errorf("expected an error")
		}
	})
}

// TestKeys

func TestKeys(t *testing.T) {
	startServer(t, "test keys", func() {
		tests := []struct {
			testname string
			keys     [][]byte
		}{
			{"single value", [][]byte{[]byte("one")}},
			{"two values", [][]byte{[]byte("one"), []byte("two")}},
			{"multiple values", [][]byte{[]byte("one"), []byte("two"), []byte("three")}},
		}

		for _, tc := range tests {
			kv := setup(t)
			kv.addKeys(tc.keys)
			got, err := kv.Keys()
			if err != nil {
				t.Errorf("unexpected error")
			}
			if compareKeyLists(got, tc.keys) {
				t.Errorf("got did not match want")
			}
		}
	})
}

func (kv *KV) addKeys(values [][]byte) {
	for _, val := range values {
		kv.Set(val, []byte("hello"))
	}
}

func compareKeyLists(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if bytes.Compare(a[i], b[i]) != 0 {
			return false
		}
	}
	return true
}
