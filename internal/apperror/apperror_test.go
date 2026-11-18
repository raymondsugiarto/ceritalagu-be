package apperror

import (
	"net/http"
	"testing"
)

func TestFromStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status  int
		code    Code
		message string
	}{
		{http.StatusBadRequest, CodeBadRequest, "x"},
		{http.StatusUnauthorized, CodeUnauthorized, "x"},
		{http.StatusNotFound, CodeNotFound, "x"},
		{http.StatusMethodNotAllowed, CodeNotFound, "x"},
		{http.StatusTooManyRequests, CodeTooMany, "x"},
		{http.StatusBadGateway, CodeInternal, "internal server error"}, // 5xx never leaks the message
	}
	for _, tt := range tests {
		got := FromStatus(tt.status, "x")
		if got.Code != tt.code || got.Message != tt.message || got.Status != tt.status || got.Reason != "" {
			t.Errorf("FromStatus(%d) = %+v", tt.status, got)
		}
	}
}
