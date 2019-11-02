package utils

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"strings"
)

func HashedName(name string) string {
	s := md5.Sum([]byte(name))
	ss := base64.RawStdEncoding.EncodeToString(s[:])
	if len(ss) > 6 {
		ss = ss[:6]
	}
	return strings.ToLower(ss)
}

func HashWithPrefix(prefix, name string) string {
	return fmt.Sprintf("%s-%s", prefix, HashedName(name))
}
