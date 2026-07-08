package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireBearerToken(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := requireBearerToken("s3cret", inner)

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
		wantCalled bool
	}{
		{"correct token", "Bearer s3cret", http.StatusOK, true},
		{"missing header", "", http.StatusUnauthorized, false},
		{"wrong token", "Bearer nope", http.StatusUnauthorized, false},
		{"wrong scheme", "Basic s3cret", http.StatusUnauthorized, false},
		{"token only", "s3cret", http.StatusUnauthorized, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			req := httptest.NewRequest(http.MethodGet, "/status", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if called != tc.wantCalled {
				t.Errorf("inner called = %v, want %v", called, tc.wantCalled)
			}
		})
	}
}
