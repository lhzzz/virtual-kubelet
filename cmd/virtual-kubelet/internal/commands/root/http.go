// Copyright © 2017 The virtual-kubelet authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package root

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/virtual-kubelet/virtual-kubelet/log"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var (
	defaultCertPath = filepath.Join(homedir.HomeDir(), "client.cert")
	defaultKeyPath  = filepath.Join(homedir.HomeDir(), "client.key")
	defaultCaPath   = filepath.Join(homedir.HomeDir(), "ca.cert")
)

type apiServerConfig struct {
	CertPath              string
	KeyPath               string
	CACertPath            string
	Addr                  string
	MetricsAddr           string
	StreamIdleTimeout     time.Duration
	StreamCreationTimeout time.Duration
}

func getAPIConfig(c Opts) (*apiServerConfig, error) {
	cert, key, ca := getCertPath()

	config := apiServerConfig{
		CertPath:   cert, //os.Getenv("APISERVER_CERT_LOCATION"),
		KeyPath:    key,  //os.Getenv("APISERVER_KEY_LOCATION"),
		CACertPath: ca,   //os.Getenv("APISERVER_CA_CERT_LOCATION"),
	}

	config.Addr = fmt.Sprintf(":%d", c.ListenPort)
	config.MetricsAddr = c.MetricsAddr
	config.StreamIdleTimeout = c.StreamIdleTimeout
	config.StreamCreationTimeout = c.StreamCreationTimeout

	return &config, nil
}

func getCertPath() (cert, key, ca string) {
	cert = os.Getenv("APISERVER_CERT_LOCATION")
	key = os.Getenv("APISERVER_KEY_LOCATION")
	ca = os.Getenv("APISERVER_CA_CERT_LOCATION")

	if cert != "" && key != "" && ca != "" {
		return
	}
	ctx := context.TODO()
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	restconf, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.G(ctx).Error("get restconfig failed,err=", err)
		return
	}

	certfile, err := os.OpenFile(defaultCertPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		log.G(ctx).Error("open cert path failed,err=", err)
		return
	}
	defer certfile.Close()
	_, err = certfile.Write(restconf.CertData)
	if err != nil {
		log.G(ctx).Error("write cert date failed,err=", err)
		return
	}
	cert = defaultCertPath

	keyfile, err := os.OpenFile(defaultKeyPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		log.G(ctx).Error("open key path failed,err=", err)
		return
	}
	defer keyfile.Close()
	_, err = keyfile.Write(restconf.KeyData)
	if err != nil {
		log.G(ctx).Error("write key date failed,err=", err)
		return
	}
	key = defaultKeyPath

	cafile, err := os.OpenFile(defaultCaPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		log.G(ctx).Error("open ca path failed,err=", err)
		return
	}
	defer cafile.Close()
	_, err = cafile.Write(restconf.CAData)
	if err != nil {
		log.G(ctx).Error("write ca date failed,err=", err)
		return
	}
	ca = defaultCaPath
	return
}
