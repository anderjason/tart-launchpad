package launchpad

import (
	"os"
)

type HostEnvironment interface {
	CurrentDirectory() (string, error)
	FileExists(path string) (bool, error)
}

type RealHostEnvironment struct{}

func (RealHostEnvironment) CurrentDirectory() (string, error) {
	return os.Getwd()
}

func (RealHostEnvironment) FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
