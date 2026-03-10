package httpx

import (
	"encoding/json"
	"net/http"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a standardized error response body.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorEnvelope{
		Error: errorBody{
			Code:    code,
			Message: message,
		},
	})
}

