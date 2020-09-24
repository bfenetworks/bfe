// Copyright (c) 2019 The BFE Authors.
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

package server_cert_conf

import (
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
)

import (
	"github.com/baidu/go-lib/log"
	"golang.org/x/crypto/ocsp"
)

import (
	"github.com/bfenetworks/bfe/bfe_tls"
	"github.com/bfenetworks/bfe/bfe_util"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

const (
	DefaultCert = "BFE_DEFAULT_CERT"
)

// ServerCertConf is conf of certificate
type ServerCertConf struct {
	ServerCertFile   string // path to server certificate
	ServerKeyFile    string // path to server priavet key
	OcspResponseFile string // path to ocsp response file
}

type ServerCertConfMap struct {
	Default  string                    // default cert name
	CertConf map[string]ServerCertConf // (cert name, cert config)
}

// BfeServerCertConf is conf of all bfe certificate
type BfeServerCertConf struct {
	Version string // version of config
	Config  ServerCertConfMap
}

// ServerCertConfCheck check ServerCertConf config.
func (conf *ServerCertConf) Check(confRoot string) error {
	// check ServerCertConf
	if len(conf.ServerCertFile) == 0 {
		return fmt.Errorf("no ServerCertFile")
	}
	conf.ServerCertFile = bfe_util.ConfPathProc(conf.ServerCertFile, confRoot)
	if _, err := os.Stat(conf.ServerCertFile); os.IsNotExist(err) {
		return fmt.Errorf("server cert file not exist: %s", conf.ServerCertFile)
	}

	// check ServerKeyFile
	if len(conf.ServerKeyFile) == 0 {
		return fmt.Errorf("no ServerKeyFile")
	}
	conf.ServerKeyFile = bfe_util.ConfPathProc(conf.ServerKeyFile, confRoot)
	if _, err := os.Stat(conf.ServerKeyFile); os.IsNotExist(err) {
		return fmt.Errorf("server key not exist: %s", conf.ServerKeyFile)
	}

	// check OcspResponseFile
	// Note: if not specified, it means Ocsp stapling is not enabled
	if len(conf.OcspResponseFile) > 0 {
		conf.OcspResponseFile = bfe_util.ConfPathProc(conf.OcspResponseFile, confRoot)
		if _, err := os.Stat(conf.OcspResponseFile); os.IsNotExist(err) {
			return fmt.Errorf("ocsp response not exist: %s", conf.OcspResponseFile)
		}
	}

	return nil
}

// BfeServerCertConfCheck check integrity of config.
func (conf *BfeServerCertConf) Check(confRoot string) error {
	if len(conf.Version) == 0 {
		return fmt.Errorf("no Version")
	}

	certConfMap := conf.Config.CertConf
	for name, certConf := range certConfMap {
		if name == DefaultCert {
			return fmt.Errorf("CertName should not be %s", DefaultCert)
		}

		err := certConf.Check(confRoot)
		if err != nil {
			return fmt.Errorf("BfeServerCertConf.Config for %s:%s", name, err.Error())
		}
		certConfMap[name] = certConf
	}

	defaultCert := conf.Config.Default
	if _, ok := certConfMap[defaultCert]; !ok {
		return fmt.Errorf("BfeServerCertConf.Config default certificate %s not exit", defaultCert)
	}

	return nil
}

// ServerCertConfLoad loads config of certificate from file.
func ServerCertConfLoad(filename string, confRoot string) (BfeServerCertConf, error) {
	var config BfeServerCertConf

	// open the file
	file, err := os.Open(filename)
	if err != nil {
		return config, err
	}
	defer file.Close()

	// decode the file
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return config, err
	}

	// check conf
	err = config.Check(confRoot)
	if err != nil {
		return config, err
	}

	return config, nil
}

func OcspResponseCheck(bytes []byte, cert bfe_tls.Certificate) (*ocsp.Response, error) {
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}

	parse, err := ocsp.ParseResponse(bytes, nil)
	if err != nil {
		return nil, err
	}

	// make sure cert and ocsp's SerialNumber is same so they match with each other
	if parse.SerialNumber.Cmp(x509Cert.SerialNumber) != 0 {
		return nil, fmt.Errorf("ocsp serial number does not match with certificate")
	}

	// ocsp status should be Good
	if parse.Status != ocsp.Good {
		return nil, fmt.Errorf("ocsp status is not Good")
	}

	// ocsp update time range should be correct
	if !bfe_tls.OcspTimeRangeCheck(parse) {
		return nil, fmt.Errorf("ocsp time is out of range")
	}

	return parse, nil
}

func OcspResponseLoad(filename string) ([]byte, error) {
	// read binary data from file
	ocspResponse, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return ocspResponse, nil
}

func ServerCertParse(certConf BfeServerCertConf) (map[string]*bfe_tls.Certificate, error) {
	certMap := make(map[string]*bfe_tls.Certificate)

	for name, conf := range certConf.Config.CertConf {
		var cert bfe_tls.Certificate

		// load x509 certificate and key
		cert, err := bfe_tls.LoadX509KeyPair(conf.ServerCertFile, conf.ServerKeyFile)
		if err != nil {
			return nil, fmt.Errorf("in LoadX509KeyPair() :%s", err.Error())
		}

		// load ocsp response for stapling
		if len(conf.OcspResponseFile) != 0 {
			OcspResponse, err := OcspResponseLoad(conf.OcspResponseFile)
			if err != nil {
				return nil, fmt.Errorf("in OcspResponseLoad() :%s", err.Error())
			}
			// check ocsp response, only output a log if error
			OcspParse, err := OcspResponseCheck(OcspResponse, cert)
			if err != nil {
				log.Logger.Warn("ignore invalid ocsp response file [%s]: %s", conf.OcspResponseFile, err)
			} else {
				cert.OCSPStaple = OcspResponse
				cert.OCSPParse = OcspParse
			}
		}
		certMap[name] = &cert
	}

	certMap[DefaultCert] = certMap[certConf.Config.Default]
	return certMap, nil
}

// GetNamesForCert returns all subject names of cert.
func GetNamesForCert(cert *bfe_tls.Certificate) []string {
	// parse certificate
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil
	}

	// get subject name and alternative name of cert
	names := make([]string, 0)
	if len(x509Cert.Subject.CommonName) > 0 {
		names = append(names, x509Cert.Subject.CommonName)
	}
	names = append(names, x509Cert.DNSNames...)

	return names
}
