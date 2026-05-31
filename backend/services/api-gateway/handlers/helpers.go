package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"connectrpc.com/connect"
)

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// writeConnectError maps a Connect-RPC error to an HTTP error response.
func writeConnectError(w http.ResponseWriter, err error) {
	if connectErr := new(connect.Error); errors.As(err, &connectErr) {
		status, message := mapConnectCode(connectErr.Code(), connectErr.Message())
		writeError(w, status, message)
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// mapConnectCode converts a Connect error code to an HTTP status code and message.
func mapConnectCode(code connect.Code, message string) (int, string) {
	switch code {
	case connect.CodeInvalidArgument:
		return http.StatusBadRequest, message
	case connect.CodeUnauthenticated:
		return http.StatusUnauthorized, message
	case connect.CodePermissionDenied:
		return http.StatusForbidden, message
	case connect.CodeNotFound:
		return http.StatusNotFound, message
	case connect.CodeAlreadyExists:
		return http.StatusConflict, message
	case connect.CodeInternal:
		return http.StatusInternalServerError, "internal server error"
	case connect.CodeUnavailable:
		return http.StatusServiceUnavailable, "service unavailable"
	case connect.CodeDeadlineExceeded:
		return http.StatusGatewayTimeout, "request timeout"
	case connect.CodeCanceled:
		return http.StatusRequestTimeout, "request canceled"
	case connect.CodeFailedPrecondition:
		return http.StatusBadRequest, message
	case connect.CodeAborted:
		return http.StatusConflict, message
	case connect.CodeOutOfRange:
		return http.StatusBadRequest, message
	case connect.CodeUnimplemented:
		return http.StatusNotImplemented, message
	case connect.CodeDataLoss:
		return http.StatusInternalServerError, "internal server error"
	case connect.CodeResourceExhausted:
		return http.StatusTooManyRequests, message
	case connect.CodeUnknown:
		return http.StatusInternalServerError, "internal server error"
	default:
		return http.StatusInternalServerError, message
	}
}
