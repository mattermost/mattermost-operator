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
	"math"
	"sort"
	"time"
)

// Timings captures all latency metrics
type Timings struct {
	Avg     time.Duration `json:"avg"`   // Average duration per sample
	P50     time.Duration `json:"p50"`   // 50th %ile of all the sample durations
	P75     time.Duration `json:"p75"`   // 75th %ile of all the sample durations
	P95     time.Duration `json:"p95"`   // 95th %ile of all the sample durations
	P99     time.Duration `json:"p99"`   // 99th %ile of all the sample durations
	P999    time.Duration `json:"p999"`  // 99.9th %ile of all the sample durations
	Long5p  time.Duration `json:"l5p"`   // Average duration of the longest 5%
	Short5p time.Duration `json:"s5p"`   // Average duration of the shortest 5%
	Max     time.Duration `json:"max"`   // Max duration
	Min     time.Duration `json:"min"`   // Min duration
	StdDev  time.Duration `json:"sdev"`  // Standard deviation among all the sample durations
	Range   time.Duration `json:"range"` // Delta between Max and Min
}

// Measure - calculate all the latency measurements
func (ts TimeDurations) Measure() Timings {
	if len(ts) == 0 {
		return Timings{
			Avg:     0,
			P50:     0,
			P75:     0,
			P95:     0,
			P99:     0,
			P999:    0,
			Long5p:  0,
			Short5p: 0,
			Min:     0,
			Max:     0,
			Range:   0,
			StdDev:  0,
		}
	}
	sort.Slice(ts, func(i, j int) bool {
		return int64(ts[i]) < int64(ts[j])
	})
	return Timings{
		Avg:     ts.avg(),
		P50:     ts[ts.Len()/2],
		P75:     ts.p(0.75),
		P95:     ts.p(0.95),
		P99:     ts.p(0.99),
		P999:    ts.p(0.999),
		Long5p:  ts.long5p(),
		Short5p: ts.short5p(),
		Min:     ts.min(),
		Max:     ts.max(),
		Range:   ts.srange(),
		StdDev:  ts.stdDev(),
	}
}

// TimeDurations is time.Duration segments.
type TimeDurations []time.Duration

func (ts TimeDurations) Len() int { return len(ts) }

func (ts TimeDurations) avg() time.Duration {
	var total time.Duration
	for _, t := range ts {
		total += t
	}
	return time.Duration(int(total) / ts.Len())
}

func (ts TimeDurations) p(p float64) time.Duration {
	return ts[int(float64(ts.Len())*p+0.5)-1]
}

func (ts TimeDurations) stdDev() time.Duration {
	m := ts.avg()
	s := 0.00

	for _, t := range ts {
		s += math.Pow(float64(m-t), 2)
	}

	msq := s / float64(ts.Len())

	return time.Duration(math.Sqrt(msq))
}

func (ts TimeDurations) long5p() time.Duration {
	set := ts[int(float64(ts.Len())*0.95+0.5):]

	if len(set) <= 1 {
		return ts[ts.Len()-1]
	}

	var t time.Duration
	var i int
	for _, n := range set {
		t += n
		i++
	}

	return time.Duration(int(t) / i)
}

func (ts TimeDurations) short5p() time.Duration {
	set := ts[:int(float64(ts.Len())*0.05+0.5)]

	if len(set) <= 1 {
		return ts[0]
	}

	var t time.Duration
	var i int
	for _, n := range set {
		t += n
		i++
	}

	return time.Duration(int(t) / i)
}

func (ts TimeDurations) min() time.Duration {
	return ts[0]
}

func (ts TimeDurations) max() time.Duration {
	return ts[ts.Len()-1]
}

func (ts TimeDurations) srange() time.Duration {
	return ts.max() - ts.min()
}
