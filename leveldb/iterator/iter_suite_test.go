package iterator_test

import (
	"testing"

	"github.com/rev3z/ledger_base/leveldb/testutil"
)

func TestIterator(t *testing.T) {
	testutil.RunSuite(t, "Iterator Suite")
}
