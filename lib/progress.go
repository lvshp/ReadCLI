package lib

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type ProgressStore struct {
	Books map[string]int `json:"books"`
}

func progressFilePath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(dataDirEnv)); dir != "" {
		return filepath.Join(dir, "progress.json"), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, dataDirName, "progress.json"), nil
}

func legacyRootProgressFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, legacyRootDir, "progress.json"), nil
}

func legacyReadcliProgressFilePath() (string, error) {
	return filepath.Join(mustUserConfigDir(), appName, "progress.json"), nil
}

func legacyProgressFilePath() (string, error) {
	return filepath.Join(mustUserConfigDir(), legacyAppName, "progress.json"), nil
}

func LoadProgress() (*ProgressStore, error) {
	path, err := progressFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		legacyFns := []func() (string, error){
			legacyRootProgressFilePath,
			legacyReadcliProgressFilePath,
			legacyProgressFilePath,
		}
		for _, pathFn := range legacyFns {
			legacyPath, legacyErr := pathFn()
			if legacyErr != nil {
				continue
			}
			data, err = os.ReadFile(legacyPath)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			break
		}
		if os.IsNotExist(err) {
			return &ProgressStore{Books: make(map[string]int)}, nil
		}
	}
	if err != nil {
		return nil, err
	}

	store := &ProgressStore{Books: make(map[string]int)}
	if err := json.Unmarshal(data, store); err != nil {
		return nil, err
	}

	if store.Books == nil {
		store.Books = make(map[string]int)
	}

	return store, nil
}

func SaveProgress(store *ProgressStore) error {
	path, err := progressFilePath()
	if err != nil {
		return err
	}

	if store == nil {
		store = &ProgressStore{Books: make(map[string]int)}
	}
	if store.Books == nil {
		store.Books = make(map[string]int)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func mustUserConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "."
	}
	return configDir
}
