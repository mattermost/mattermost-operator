//
// MinIO Object Storage (c) 2022 MinIO, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package madmin

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/minio/minio-go/v7/pkg/set"
)

// Top level configuration key constants.
const (
	CredentialsSubSys    = "credentials"
	PolicyOPASubSys      = "policy_opa"
	PolicyPluginSubSys   = "policy_plugin"
	IdentityOpenIDSubSys = "identity_openid"
	IdentityLDAPSubSys   = "identity_ldap"
	IdentityTLSSubSys    = "identity_tls"
	IdentityPluginSubSys = "identity_plugin"
	CacheSubSys          = "cache"
	SiteSubSys           = "site"
	RegionSubSys         = "region"
	EtcdSubSys           = "etcd"
	StorageClassSubSys   = "storage_class"
	APISubSys            = "api"
	CompressionSubSys    = "compression"
	LoggerWebhookSubSys  = "logger_webhook"
	AuditWebhookSubSys   = "audit_webhook"
	AuditKafkaSubSys     = "audit_kafka"
	HealSubSys           = "heal"
	ScannerSubSys        = "scanner"
	CrawlerSubSys        = "crawler"
	SubnetSubSys         = "subnet"
	CallhomeSubSys       = "callhome"

	NotifyKafkaSubSys    = "notify_kafka"
	NotifyMQTTSubSys     = "notify_mqtt"
	NotifyMySQLSubSys    = "notify_mysql"
	NotifyNATSSubSys     = "notify_nats"
	NotifyNSQSubSys      = "notify_nsq"
	NotifyESSubSys       = "notify_elasticsearch"
	NotifyAMQPSubSys     = "notify_amqp"
	NotifyPostgresSubSys = "notify_postgres"
	NotifyRedisSubSys    = "notify_redis"
	NotifyWebhookSubSys  = "notify_webhook"
)

// SubSystems - list of all subsystems in MinIO
var SubSystems = set.CreateStringSet(
	CredentialsSubSys,
	PolicyOPASubSys,
	PolicyPluginSubSys,
	IdentityOpenIDSubSys,
	IdentityLDAPSubSys,
	IdentityTLSSubSys,
	IdentityPluginSubSys,
	CacheSubSys,
	SiteSubSys,
	RegionSubSys,
	EtcdSubSys,
	StorageClassSubSys,
	APISubSys,
	CompressionSubSys,
	LoggerWebhookSubSys,
	AuditWebhookSubSys,
	AuditKafkaSubSys,
	HealSubSys,
	ScannerSubSys,
	CrawlerSubSys,
	SubnetSubSys,
	CallhomeSubSys,
	NotifyKafkaSubSys,
	NotifyMQTTSubSys,
	NotifyMySQLSubSys,
	NotifyNATSSubSys,
	NotifyNSQSubSys,
	NotifyESSubSys,
	NotifyAMQPSubSys,
	NotifyPostgresSubSys,
	NotifyRedisSubSys,
	NotifyWebhookSubSys,
)

// Standard config keys and values.
const (
	EnableKey  = "enable"
	CommentKey = "comment"

	// Enable values
	EnableOn  = "on"
	EnableOff = "off"
)

// HasSpace - returns if given string has space.
func HasSpace(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

// Constant separators
const (
	SubSystemSeparator = `:`
	KvSeparator        = `=`
	KvComment          = `#`
	KvSpaceSeparator   = ` `
	KvNewline          = "\n"
	KvDoubleQuote      = `"`
	KvSingleQuote      = `'`

	Default = `_`

	EnvPrefix        = "MINIO_"
	EnvWordDelimiter = `_`

	EnvLinePrefix = KvComment + KvSpaceSeparator + EnvPrefix
)

// SanitizeValue - this function is needed, to trim off single or double quotes, creeping into the values.
func SanitizeValue(v string) string {
	v = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(v), KvDoubleQuote), KvDoubleQuote)
	return strings.TrimSuffix(strings.TrimPrefix(v, KvSingleQuote), KvSingleQuote)
}

// EnvOverride contains the name of the environment variable and its value.
type EnvOverride struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ConfigKV represents a configuration key and value, along with any environment
// override if present.
type ConfigKV struct {
	Key         string       `json:"key"`
	Value       string       `json:"value"`
	EnvOverride *EnvOverride `json:"envOverride,omitempty"`
}

// SubsysConfig represents the configuration for a particular subsytem and
// target.
type SubsysConfig struct {
	SubSystem string `json:"subSystem"`
	Target    string `json:"target,omitempty"`

	// WARNING: Use AddConfigKV() to mutate this.
	KV []ConfigKV `json:"kv"`

	kvIndexMap map[string]int
}

// AddConfigKV - adds a config parameter to the subsystem.
func (c *SubsysConfig) AddConfigKV(ckv ConfigKV) {
	if c.kvIndexMap == nil {
		c.kvIndexMap = make(map[string]int)
	}
	idx, ok := c.kvIndexMap[ckv.Key]
	if ok {
		c.KV[idx] = ckv
	} else {
		c.KV = append(c.KV, ckv)
		c.kvIndexMap[ckv.Key] = len(c.KV) - 1
	}
}

// Lookup resolves the value of a config parameter. If an env variable is
// specified on the server for the parameter, it is returned.
func (c *SubsysConfig) Lookup(key string) (val string, present bool) {
	if c.kvIndexMap == nil {
		return "", false
	}

	idx, ok := c.kvIndexMap[key]
	if !ok {
		return "", false
	}
	if c.KV[idx].EnvOverride != nil {
		return c.KV[idx].EnvOverride.Value, true
	}
	return c.KV[idx].Value, true
}

var (
	ErrInvalidEnvVarLine = errors.New("expected env var line of the form `# MINIO_...=...`")
	ErrInvalidConfigKV   = errors.New("expected config value in the format `key=value`")
)

func parseEnvVarLine(s, subSystem, target string) (val ConfigKV, err error) {
	s = strings.TrimPrefix(s, KvComment+KvSpaceSeparator)
	ps := strings.SplitN(s, KvSeparator, 2)
	if len(ps) != 2 {
		err = ErrInvalidEnvVarLine
		return
	}

	val.EnvOverride = &EnvOverride{
		Name:  ps[0],
		Value: ps[1],
	}

	envVar := val.EnvOverride.Name
	envPrefix := EnvPrefix + strings.ToUpper(subSystem) + EnvWordDelimiter
	if !strings.HasPrefix(envVar, envPrefix) {
		err = fmt.Errorf("expected env %v to have prefix %v", envVar, envPrefix)
		return
	}
	configVar := strings.TrimPrefix(envVar, envPrefix)
	if target != Default {
		configVar = strings.TrimSuffix(configVar, EnvWordDelimiter+target)
	}
	val.Key = strings.ToLower(configVar)
	return
}

// Takes "k1=v1 k2=v2 ..." and returns key=k1 and rem="v1 k2=v2 ..." on success.
func parseConfigKey(text string) (key, rem string, err error) {
	// Split to first `=`
	ts := strings.SplitN(text, KvSeparator, 2)

	key = strings.TrimSpace(ts[0])
	if len(key) == 0 {
		err = ErrInvalidConfigKV
		return
	}

	if len(ts) == 1 {
		err = ErrInvalidConfigKV
		return
	}

	return key, ts[1], nil
}

func parseConfigValue(text string) (v, rem string, err error) {
	// Value may be double quoted.
	if strings.HasPrefix(text, KvDoubleQuote) {
		text = strings.TrimPrefix(text, KvDoubleQuote)
		ts := strings.SplitN(text, KvDoubleQuote, 2)
		v = ts[0]
		if len(ts) == 1 {
			err = ErrInvalidConfigKV
			return
		}
		rem = strings.TrimSpace(ts[1])
	} else {
		ts := strings.SplitN(text, KvSpaceSeparator, 2)
		v = ts[0]
		if len(ts) == 2 {
			rem = strings.TrimSpace(ts[1])
		} else {
			rem = ""
		}
	}
	return
}

func parseConfigLine(s string) (c SubsysConfig, err error) {
	ps := strings.SplitN(s, KvSpaceSeparator, 2)

	ws := strings.SplitN(ps[0], SubSystemSeparator, 2)
	c.SubSystem = ws[0]
	if len(ws) == 2 {
		c.Target = ws[1]
	}

	if len(ps) == 1 {
		// No config KVs present.
		return
	}

	// Parse keys and values
	text := strings.TrimSpace(ps[1])
	for len(text) > 0 {

		kv := ConfigKV{}
		kv.Key, text, err = parseConfigKey(text)
		if err != nil {
			return
		}

		kv.Value, text, err = parseConfigValue(text)
		if err != nil {
			return
		}

		c.AddConfigKV(kv)
	}
	return
}

func isEnvLine(s string) bool {
	return strings.HasPrefix(s, EnvLinePrefix)
}

func isCommentLine(s string) bool {
	return strings.HasPrefix(s, KvComment)
}

func getConfigLineSubSystemAndTarget(s string) (subSys, target string) {
	words := strings.SplitN(s, KvSpaceSeparator, 2)
	pieces := strings.SplitN(words[0], SubSystemSeparator, 2)
	if len(pieces) == 2 {
		return pieces[0], pieces[1]
	}
	// If no target is present, it is the default target.
	return pieces[0], Default
}

// ParseServerConfigOutput - takes a server config output and returns a slice of
// configs. Depending on the server config get API request, this may return
// configuration info for one or more configuration sub-systems.
//
// A configuration subsystem in the server may have one or more subsystem
// targets (named instances of the sub-system, for example `notify_postres`,
// `logger_webhook` or `identity_openid`). For every subsystem and target
// returned in `serverConfigOutput`, this function returns a separate
// `SubsysConfig` value in the output slice. The default target is returned as
// "" (empty string) by this function.
//
// Use the `Lookup()` function on the `SubsysConfig` type to query a
// subsystem-target pair for a configuration parameter. This returns the
// effective value (i.e. possibly overridden by an environment variable) of the
// configuration parameter on the server.
func ParseServerConfigOutput(serverConfigOutput string) ([]SubsysConfig, error) {
	lines := strings.Split(serverConfigOutput, "\n")

	// Clean up config lines
	var configLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			configLines = append(configLines, line)
		}
	}

	// Parse out config lines into groups corresponding to a single subsystem
	// and target.
	//
	// How does it work? The server output is a list of lines, where each line
	// may be one of:
	//
	//   1. A config line for a single subsystem (and optional target). For
	//   example, "site region=us-east-1" or "identity_openid:okta k1=v1 k2=v2".
	//
	//   2. A comment line showing an environment variable set on the server.
	//   For example "# MINIO_SITE_NAME=my-cluster".
	//
	//   3. Comment lines with other content. These will not start with `#
	//   MINIO_`.
	//
	// For the structured JSON representation, only lines of type 1 and 2 are
	// required as they correspond to configuration specified by an
	// administrator.
	//
	// Additionally, after ignoring lines of type 3 above:
	//
	//   1. environment variable lines for a subsystem (and target if present)
	//   appear consecutively.
	//
	//   2. exactly one config line for a subsystem and target immediately
	//   follows the env var lines for the same subsystem and target.
	//
	// The parsing logic below classifies each line and groups them by
	// subsystem and target.
	var configGroups [][]string
	var subSystems []string
	var targets []string
	var currGroup []string
	for _, line := range configLines {
		if isEnvLine(line) {
			currGroup = append(currGroup, line)
		} else if isCommentLine(line) {
			continue
		} else {
			subSys, target := getConfigLineSubSystemAndTarget(line)
			currGroup = append(currGroup, line)
			configGroups = append(configGroups, currGroup)
			subSystems = append(subSystems, subSys)
			targets = append(targets, target)

			// Reset currGroup to collect lines for the next group.
			currGroup = nil
		}
	}

	res := make([]SubsysConfig, 0, len(configGroups))
	for i, group := range configGroups {
		sc := SubsysConfig{
			SubSystem: subSystems[i],
		}
		if targets[i] != Default {
			sc.Target = targets[i]
		}

		for _, line := range group {
			if isEnvLine(line) {
				ckv, err := parseEnvVarLine(line, subSystems[i], targets[i])
				if err != nil {
					return nil, err
				}
				// Since all env lines have distinct env vars, we can append
				// here without risk of introducing any duplicates.
				sc.AddConfigKV(ckv)
				continue
			}

			// At this point all env vars for this subsys and target are already
			// in `sc.KV`, so we fill in values if a ConfigKV entry for the
			// config parameter is already present.
			lineCfg, err := parseConfigLine(line)
			if err != nil {
				return nil, err
			}
			for _, kv := range lineCfg.KV {
				idx, ok := sc.kvIndexMap[kv.Key]
				if ok {
					sc.KV[idx].Value = kv.Value
				} else {
					sc.AddConfigKV(kv)
				}
			}
		}

		res = append(res, sc)
	}

	return res, nil
}
