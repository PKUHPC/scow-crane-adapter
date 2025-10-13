package app

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

func GetCertPath() (string, string, string) {
	currentPwd, _ := os.Getwd()
	logrus.Tracef("current pwd: %s", currentPwd)
	caCertPath := filepath.Join(currentPwd, "certs/ca.crt")
	adapterCertPath := filepath.Join(currentPwd, "certs/adapter.crt")
	adapterPrivateKeyPath := filepath.Join(currentPwd, "certs/adapter.key")
	caCertPathExists, err := pathExists(caCertPath)
	if err != nil {
		logrus.Tracef("cert pwd: %s, %s, %s", caCertPath, adapterCertPath, adapterPrivateKeyPath)
		return GConfig.Ssl.CaCertPath, GConfig.Ssl.AdapterCertPath, GConfig.Ssl.AdapterPrivateKeyPath
	}
	adapterCertPathExists, err := pathExists(adapterCertPath)
	if err != nil {
		logrus.Tracef("cert pwd: %s, %s, %s", caCertPath, adapterCertPath, adapterPrivateKeyPath)
		return GConfig.Ssl.CaCertPath, GConfig.Ssl.AdapterCertPath, GConfig.Ssl.AdapterPrivateKeyPath
	}
	adapterPrivateKeyPathExists, err := pathExists(adapterPrivateKeyPath)
	if err != nil {
		logrus.Tracef("cert pwd: %s, %s, %s", caCertPath, adapterCertPath, adapterPrivateKeyPath)
		return GConfig.Ssl.CaCertPath, GConfig.Ssl.AdapterCertPath, GConfig.Ssl.AdapterPrivateKeyPath
	}

	logrus.Tracef("cert pwd: %s, %s, %s", caCertPath, adapterCertPath, adapterPrivateKeyPath)
	if caCertPathExists && adapterCertPathExists && adapterPrivateKeyPathExists {
		return caCertPath, adapterCertPath, adapterPrivateKeyPath
	}
	logrus.Tracef("cert pwd: %s, %s, %s", caCertPath, adapterCertPath, adapterPrivateKeyPath)
	return GConfig.Ssl.CaCertPath, GConfig.Ssl.AdapterCertPath, GConfig.Ssl.AdapterPrivateKeyPath
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
