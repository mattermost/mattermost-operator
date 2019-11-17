package utils

import (
	"crypto/md5"
	"fmt"
	"strings"
)

// HashedName returns md5 sum truncated to 6 alphanumeric characters and converted to lowercase.
// The motivation behind is to render K8S resource definition's name with constant length deterministic string.
func HashedName(name string) string {
	s := md5.Sum([]byte(name))
	ss := fmt.Sprintf("%x", s)
	if len(ss) > 6 {
		ss = ss[:6]
	}
	return strings.ToLower(ss)
}

// HashWithPrefix encapsulates K8S resource name creator.
func HashWithPrefix(prefix, name string) string {
	return fmt.Sprintf("%s-%s", prefix, HashedName(name))
}
