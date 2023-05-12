package util

import (
	"errors"
	"os"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
)

func CheckFileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !errors.Is(err, os.ErrNotExist)
}

func GetFilesize(filePath string) int64 {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Errorf("Error on Stat %s: %v", filePath, err)
	}
	return fileInfo.Size()
}

func GetFilecount(path string) int {
	files, err := os.ReadDir(path)
	if err != nil {
		log.Errorf("Error on ReadDir %s: %v", path, err)
	}

	return len(files)
}
