package transport

import "google.golang.org/grpc/grpclog"

const logLevel = 2

func infof(format string, args ...interface{}) {
	if grpclog.V(logLevel) {
		grpclog.Infof(format, args...)
	}
}

func warningf(format string, args ...interface{}) {
	if grpclog.V(logLevel) {
		grpclog.Warningf(format, args...)
	}
}

func errorf(format string, args ...interface{}) {
	if grpclog.V(logLevel) {
		grpclog.Errorf(format, args...)
	}
}
