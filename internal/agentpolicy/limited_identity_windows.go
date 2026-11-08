//go:build windows

package agentpolicy

import "errors"

func limitedDirectoryIdentity(string) (string, error) {
	return "", errors.New("limited_directory_identity_unverified")
}
