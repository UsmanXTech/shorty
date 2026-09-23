package password

import (
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// TestMain lowers the bcrypt cost for the whole package so the suite stays
// fast. Production code always uses bcrypt.DefaultCost.
func TestMain(m *testing.M) {
	hashCost = bcrypt.MinCost
	os.Exit(m.Run())
}
