package webhooks

import (
	"os"
	"testing"
	"time"
)

// TestMain shrinks the delivery-retry backoff so failure-path tests don't
// sleep through the suite. Production always retries after one second.
func TestMain(m *testing.M) {
	retryBackoff = time.Millisecond
	os.Exit(m.Run())
}
