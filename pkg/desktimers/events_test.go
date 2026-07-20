package desktimers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReportTaskSelected(t *testing.T) {
	var received map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/git-client/events" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer dtg_abc" {
			t.Errorf("missing bearer, got %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(`{"success":true,"data":{"taskId":"t-1"}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "dtg_abc")
	if err := client.ReportTaskSelected("LOUD-124", "loudowls/deskgit", "feature/LOUD-124-x"); err != nil {
		t.Fatal(err)
	}

	if received["type"] != "task_selected" ||
		received["code"] != "LOUD-124" ||
		received["repo"] != "loudowls/deskgit" ||
		received["branch"] != "feature/LOUD-124-x" {
		t.Errorf("unexpected payload: %v", received)
	}
	if _, err := time.Parse(time.RFC3339, received["occurredAt"]); err != nil {
		t.Errorf("occurredAt not RFC3339: %v", received["occurredAt"])
	}
}

func TestReportTaskSelectedErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	if err := NewClient(server.URL, "dtg_x").ReportTaskSelected("LOUD-1", "", ""); err == nil {
		t.Error("expected error on 401")
	}
	if err := NewClient("http://127.0.0.1:1", "dtg_x").ReportTaskSelected("LOUD-1", "", ""); err == nil {
		t.Error("expected network error")
	}
}

func TestRemoteURLToSlug(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"git@github.com:loudowls/deskgit.git", "loudowls/deskgit"},
		{"git@github-alias:debuging-life/lazygit.git", "debuging-life/lazygit"},
		{"https://github.com/loudowls/deskgit.git", "loudowls/deskgit"},
		{"https://github.com/loudowls/deskgit", "loudowls/deskgit"},
		{"ssh://git@github.com/loudowls/deskgit.git", "loudowls/deskgit"},
		{"", ""},
		{"not-a-url", ""},
	}
	for _, tt := range tests {
		if got := remoteURLToSlug(tt.url); got != tt.want {
			t.Errorf("remoteURLToSlug(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
