package redirect

import (
	"os"
	"testing"

	"github.com/UsmanXTech/shorty/internal/password"
	"golang.org/x/crypto/bcrypt"
)

// TestMain lowers the bcrypt cost used by the password package for the whole
// suite so unlock tests stay fast.
func TestMain(m *testing.M) {
	password.SetTestCost(bcrypt.MinCost)
	os.Exit(m.Run())
}
