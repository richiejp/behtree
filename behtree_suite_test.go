package behtree_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBehtree(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Behtree Suite")
}
