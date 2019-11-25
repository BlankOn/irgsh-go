package httputil

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// StandardError response
type StandardError struct {
	Message string `json:"message"`
}

//ResponseJSON response http request with application/json
func ResponseJSON(data interface{}, status int, writer http.ResponseWriter) (err error) {
	writer.Header().Set("Content-type", "application/json")
	writer.WriteHeader(status)

	d, err := json.Marshal(data)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		d, _ = json.Marshal(StandardError{Message: "ResponseJSON: Failed to response " + err.Error()})
		err = fmt.Errorf("ResponseJSON: Failed to response : %s", err)
	}

	writer.Write(d)
	return
}

// ResponseError response http request with standard error
func ResponseError(message string, status int, writer http.ResponseWriter) (err error) {
	return ResponseJSON(StandardError{Message: message}, status, writer)
}
