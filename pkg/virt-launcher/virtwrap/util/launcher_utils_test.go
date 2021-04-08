package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "kubevirt.io/client-go/api/v1"
)

var _ = Describe("Launcher utils", func() {
	var tmpDir string
	var filePath string
	BeforeEach(func() {
		tmpDir, _ = ioutil.TempDir("", "launcher-util-test")
		filePath = filepath.Join(tmpDir, "annotations")
	})
	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("should detect early exit annotation from downward api", func() {

		withAnnotation := fmt.Sprintf("someannotation=1\nsomeotherannotation=2\n%s\nomeannotation=3\n", v1.MigrationJobCleanupSignalAnnotation)
		withOutAnnotation := "someannotation=1\nsomeotherannotation=2\nomeannotation=3\n"

		err := ioutil.WriteFile(filePath, []byte(withOutAnnotation), 0644)
		Expect(err).To(Not(HaveOccurred()), "could not create file: ", filePath)

		val := HasEarlyExitAnnotation(filePath)
		Expect(val).To(BeFalse())

		err = ioutil.WriteFile(filePath, []byte(withAnnotation), 0644)
		Expect(err).To(Not(HaveOccurred()), "could not create file: ", filePath)

		val = HasEarlyExitAnnotation(filePath)
		Expect(val).To(BeTrue())
	})

})
