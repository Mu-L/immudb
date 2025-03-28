/*
Copyright 2025 Codenotary Inc. All rights reserved.

SPDX-License-Identifier: BUSL-1.1
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://mariadb.com/bsl11/

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package immudb

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var key = `
-----BEGIN PRIVATE KEY-----
MIIBVAIBADANBgkqhkiG9w0BAQEFAASCAT4wggE6AgEAAkEApIGvkc3ROIC18zdL
K3I4VhKmY7gq1YU53dIwikrd5uy/fjUTmW73rLlthkV1zPvG/34rsW9kMDanmndb
QENmHwIDAQABAkEAkOW9vCJaP3d3TCQO7NStdHr23eywpeO0BYMGyDiLXcMOcjaT
MrkeCMyXsgtIaoh/sPFO356z2DXjz8Z4PY53EQIhANPSp0dOKi4OeqHdSsj8wjwt
eeAxCcASe3cz18gnBUB5AiEAxtDJ3spTLNogOoOzQA8g7rVT8xW5R5xQwfmvZwQx
JVcCICAIkGGZMYnLiMInzCJ/DwS4v+CmqdnRMbjCL1TGieXJAiBvQXNWCx6UYNPc
KsrqNA0Xx7zcsPFn01+VzOWM3lmqLQIgdUukJDJHjdX203wOJMd51jq3I+c1n09A
SXr+Ea7CjsE=
-----END PRIVATE KEY-----
`

var cert = `
-----BEGIN CERTIFICATE-----
MIIB4TCCAYugAwIBAgIUIZwZa1cYqrwK+McrPStDwFD+e1AwDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMTA3MTYxNDMwMThaFw0yMjA3
MTYxNDMwMThaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwXDANBgkqhkiG9w0BAQEF
AANLADBIAkEApIGvkc3ROIC18zdLK3I4VhKmY7gq1YU53dIwikrd5uy/fjUTmW73
rLlthkV1zPvG/34rsW9kMDanmndbQENmHwIDAQABo1MwUTAdBgNVHQ4EFgQU+ENZ
1LFa7+HsPySAYZuPgz2tufkwHwYDVR0jBBgwFoAU+ENZ1LFa7+HsPySAYZuPgz2t
ufkwDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAANBAHnfiyFv0PYoCCIW
d/ax/lUR3RCVV6A+hzTgOhYKvoV1U6iX21hUarcm6MB6qaeORCHfQzQpn62nRe6X
4LbTf3k=
-----END CERTIFICATE-----
`

func TestSetUpTLS(t *testing.T) {
	_, err := setUpTLS("banana", "", "banana", false, false)
	require.Error(t, err)
	_, err = setUpTLS("banana", "banana", "banana", false, false)
	require.Error(t, err)
	_, err = setUpTLS("", "", "", true, false)
	require.Error(t, err)
	_, err = setUpTLS("banana", "", "", true, false)
	require.Error(t, err)

	defer os.Remove("xxkey.pem")
	f, _ := os.Create("xxkey.pem")
	fmt.Fprint(f, key)
	f.Close()

	defer os.Remove("xxcert.pem")
	f, _ = os.Create("xxcert.pem")
	fmt.Fprint(f, cert)
	f.Close()

	_, err = setUpTLS("xxkey.pem", "xxcert.pem", "banana", true, false)
	require.Error(t, err)
}

func TestSetUpTLSWithAutoHTTPS(t *testing.T) {
	t.Run("use specified paths", func(t *testing.T) {
		tempDir := t.TempDir()

		certFile := filepath.Join(tempDir, "immudb.cert")
		keyFile := filepath.Join(tempDir, "immudb.key")

		tlsConfig, err := setUpTLS(certFile, keyFile, "", false, false)
		require.Error(t, err)
		require.Nil(t, tlsConfig)

		tlsConfig, err = setUpTLS(certFile, keyFile, "", false, true)
		require.NoError(t, err)
		require.NotNil(t, tlsConfig)

		require.FileExists(t, certFile)
		require.FileExists(t, keyFile)

		tlsConfig, err = setUpTLS(certFile, keyFile, "", false, false)
		require.NoError(t, err)
		require.NotNil(t, tlsConfig)
	})

	t.Run("use default paths", func(t *testing.T) {
		certPath, keyPath, err := getCertAndKeyPath("", "", true)
		require.NoError(t, err)

		defer func() {
			os.RemoveAll(certPath)
			os.Remove(keyPath)
		}()

		tlsConfig, err := setUpTLS("", "", "", false, true)
		require.NoError(t, err)
		require.NotNil(t, tlsConfig)

		require.FileExists(t, certPath)
		require.FileExists(t, keyPath)
	})
}
