package resource

import (
	"errors"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/http2"
)

func RetryOnTransientError(op func() error) error {
	bo := backoff.NewExponentialBackOff()
	if os.Getenv("TEST") == "true" {
		bo.InitialInterval = 5 * time.Millisecond
	} else {
		bo.InitialInterval = 5 * time.Second
	}
	bo.MaxInterval = 5 * time.Minute
	bo.MaxElapsedTime = 1 * time.Hour

	return backoff.RetryNotify(func() error {
		err := op()
		if err == nil {
			return nil
		}

		var transportErr *transport.Error
		if errors.As(err, &transportErr) {
			if transportErr.StatusCode == http.StatusTooManyRequests {
				return err
			}
		}

		if isTransientError(err) {
			return err
		}

		return backoff.Permanent(err)
	}, bo, func(err error, dur time.Duration) {
		logrus.Warnf("transient error: %s; retrying in %s", err, dur)
	})
}

func isTransientError(err error) bool {
	var streamErr http2.StreamError
	if errors.As(err, &streamErr) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	return false
}
