package leveldb

import (
	"testing"

	"github.com/rev3z/ledger_base/leveldb/testutil"
)

func TestLevelDB(t *testing.T) {
	testutil.RunSuite(t, "LevelDB Suite")
}
