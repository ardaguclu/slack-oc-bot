package filemanager

import (
	"fmt"
	"os"
	"sync"
)

type FileManager struct {
	lock    sync.Mutex
	configs map[string]string
}

func NewFileManager() *FileManager {
	return &FileManager{
		lock:    sync.Mutex{},
		configs: make(map[string]string),
	}
}

func (f *FileManager) Add(channelID string, file []byte) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	conf, err := os.CreateTemp("/tmp", fmt.Sprintf("config-%s", channelID))
	if err != nil {
		return err
	}

	_, err = conf.Write(file)
	if err != nil {
		return err
	}

	f.configs[channelID] = conf.Name()
	return nil
}

func (f *FileManager) Get(channelID string) (string, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if val, ok := f.configs[channelID]; ok {
		return val, nil
	} else {
		return "", fmt.Errorf("no valid kubeconfig was found")
	}
}
