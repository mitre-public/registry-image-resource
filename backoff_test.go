package resource_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/http2"

	resource "github.com/concourse/registry-image-resource"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

var _ = Describe("Transient Error Handling", func() {

	Describe("isTransientError", func() {
		DescribeTable("identifies transient errors",
			func(err error, expected bool) {
				Expect(resource.IsTransientError(err)).To(Equal(expected))
			},
			Entry("http2 StreamError",
				http2.StreamError{StreamID: 1, Code: http2.ErrCodeInternal},
				true,
			),
			Entry("wrapped http2 StreamError",
				fmt.Errorf("extract image: %w", http2.StreamError{StreamID: 67, Code: http2.ErrCodeInternal}),
				true,
			),
			Entry("io.ErrUnexpectedEOF",
				io.ErrUnexpectedEOF,
				true,
			),
			Entry("wrapped io.ErrUnexpectedEOF",
				fmt.Errorf("read body: %w", io.ErrUnexpectedEOF),
				true,
			),
			Entry("ECONNRESET",
				os.NewSyscallError("read", syscall.ECONNRESET),
				true,
			),
			Entry("wrapped ECONNRESET",
				fmt.Errorf("download layer: %w", os.NewSyscallError("read", syscall.ECONNRESET)),
				true,
			),
			Entry("regular io.EOF",
				io.EOF,
				false,
			),
			Entry("generic error",
				errors.New("something went wrong"),
				false,
			),
			Entry("transport error 429 is not transient",
				&transport.Error{StatusCode: http.StatusTooManyRequests},
				false,
			),
		)
	})

	Describe("RetryOnTransientError", func() {
		BeforeEach(func() {
			// Makes the RetryOnTransientError() have an interval of 5ms instead of 5s
			os.Setenv("TEST", "true")
		})

		It("succeeds on first try without retrying", func() {
			calls := 0
			err := resource.RetryOnTransientError(func() error {
				calls++
				return nil
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(calls).To(Equal(1))
		})

		It("retries on transient error and succeeds", func() {
			calls := 0
			err := resource.RetryOnTransientError(func() error {
				calls++
				if calls == 1 {
					return fmt.Errorf("extract image: %w", http2.StreamError{StreamID: 67, Code: http2.ErrCodeInternal})
				}
				return nil
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(calls).To(Equal(2))
		})

		It("retries on rate limit and succeeds", func() {
			calls := 0
			err := resource.RetryOnTransientError(func() error {
				calls++
				if calls == 1 {
					return &transport.Error{StatusCode: http.StatusTooManyRequests}
				}
				return nil
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(calls).To(Equal(2))
		})

		It("does not retry permanent errors", func() {
			calls := 0
			err := resource.RetryOnTransientError(func() error {
				calls++
				return errors.New("permanent failure")
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("permanent failure"))
			Expect(calls).To(Equal(1))
		})
	})
})
