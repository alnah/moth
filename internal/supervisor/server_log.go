package supervisor

import (
	"net/http"
	"time"
)

const maxLoggedMethodBytes = 64

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (recorder *statusRecorder) WriteHeader(status int) {
	if recorder.status != 0 {
		return
	}
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(data []byte) (int, error) {
	if recorder.status == 0 {
		recorder.WriteHeader(http.StatusOK)
	}
	n, err := recorder.ResponseWriter.Write(data)
	recorder.bytes += int64(n)
	return n, err
}

func (recorder *statusRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}

func (recorder *statusRecorder) statusCode() int {
	if recorder.status == 0 {
		return http.StatusOK
	}
	return recorder.status
}

func logMethod(method string) string {
	if len(method) <= maxLoggedMethodBytes {
		return method
	}
	return method[:maxLoggedMethodBytes]
}

func (server *Server) logCall(
	call Call,
	recorder *statusRecorder,
	errorCode string,
	started time.Time,
) {
	server.logger.Info("daemon call",
		"method", logMethod(call.Method),
		"status", recorder.statusCode(),
		"bytes", recorder.bytes,
		"duration_ms", server.clock().Sub(started).Milliseconds(),
		"error_code", errorCode,
	)
}
