package mattermost

const (
	// sizeMB is the number of bytes that make a megabyte
	sizeMB = 1048576
	// sizeGB is the number of bytes that make a gigabyte
	sizeGB = 1048576000
	// defaultMaxFileSize is the default maximum file size configuration value
	// that will be used unless nginx annotation is set
	defaultMaxFileSize = 1000

	// defaultRevHistoryLimit is the default RevisionHistoryLimit - number of
	// possible roll-back points.
	// More details:
	// https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#rolling-back-a-deployment
	defaultRevHistoryLimit = 1
	// defaultMaxUnavailable is the default max number of unavailable pods out
	// of specified `Replicas` during rolling update.
	// More details:
	// https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#max-unavailable
	// Recommended to be as low as possible in order to have number of available
	// pods as close to `Replicas` as possible.
	defaultMaxUnavailable = 0
	// defaultMaxSurge is the default max number of extra pods over specified
	// `Replicas` during rolling update.
	// More details:
	// https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#max-surge
	// Recommended not to be too high in order to have not too many extra pods
	// over requested `Replicas` number.
	defaultMaxSurge = 1
)
