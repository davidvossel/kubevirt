package util

import (
	"bufio"
	"os"
	"strings"

	v1 "kubevirt.io/client-go/api/v1"
	"kubevirt.io/client-go/log"
)

func HasEarlyExitAnnotation(annotationFilePath string) bool {
	file, err := os.Open(annotationFilePath)
	if err != nil {
		log.Log.V(4).Infof("unable to read annotaions file from downward api at path %s: %v", annotationFilePath, err)
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, v1.MigrationJobCleanupSignalAnnotation) {
			return true
		}
	}

	return false
}
