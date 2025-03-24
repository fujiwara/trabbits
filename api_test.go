package trabbits_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/fujiwara/trabbits"
	"github.com/google/go-cmp/cmp"
)

func TestAPIGetPutGetConfig(t *testing.T) {
	endpoint := fmt.Sprintf("http://localhost:%d/config", testAPIPort)
	var testConfig trabbits.Config
	t.Run("GET returns 200 OK current config", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected Content-Type: got %v want %v", ct, "application/json")
		}
		if err := json.NewDecoder(resp.Body).Decode(&testConfig); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
	})

	if len(testConfig.Upstreams) != 2 {
		t.Fatalf("unexpected number of upstreams: got %v want %v", len(testConfig.Upstreams), 2)
	}
	// update config
	testConfig.Upstreams[1].Routing.KeyPatterns = append(
		testConfig.Upstreams[1].Routing.KeyPatterns, rand.Text(),
	)

	t.Run("PUT returns 200 OK", func(t *testing.T) {
		b := new(bytes.Buffer)
		json.NewEncoder(b).Encode(testConfig)
		req, _ := http.NewRequest(http.MethodPut, endpoint, io.NopCloser(b))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected Content-Type: got %v want %v", ct, "application/json")
		}
	})

	t.Run("GET returns 200 OK with updated config", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		if code := resp.StatusCode; code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusOK)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected Content-Type: got %v want %v", ct, "application/json")
		}
		var respondConfig trabbits.Config
		if err := json.NewDecoder(resp.Body).Decode(&respondConfig); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		if diff := cmp.Diff(testConfig, respondConfig); diff != "" {
			t.Errorf("unexpected response: %s", diff)
		}
	})
}

func TestAPIPutInvalidConfig(t *testing.T) {
	endpoint := fmt.Sprintf("http://localhost:%d/config", testAPIPort)
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode("invalid")
	req, _ := http.NewRequest(http.MethodPut, endpoint, b)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	if code := resp.StatusCode; code != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", code, http.StatusBadRequest)
	}
}
